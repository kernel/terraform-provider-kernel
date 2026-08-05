package browserpool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ resource.Resource                = (*browserPoolResource)(nil)
	_ resource.ResourceWithConfigure   = (*browserPoolResource)(nil)
	_ resource.ResourceWithImportState = (*browserPoolResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*browserPoolResource)(nil)
)

type browserPoolClient interface {
	DefaultProjectID() string
	CreateBrowserPool(context.Context, string, kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error)
	GetBrowserPool(context.Context, string, string) (*kernel.BrowserPool, error)
	UpdateBrowserPool(context.Context, string, string, kernel.BrowserPoolUpdateParams) (*kernel.BrowserPool, error)
	DeleteBrowserPool(context.Context, string, string) error
}

type browserPoolResource struct {
	client browserPoolClient
}

func newResourceWithClient(client browserPoolClient) *browserPoolResource {
	return &browserPoolResource{client: client}
}

func NewResource() resource.Resource {
	return &browserPoolResource{}
}

func (r *browserPoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_browser_pool"
}

func (r *browserPoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = BrowserPoolSchema()
}

func (r *browserPoolResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan browserPoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state browserPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	var config browserPoolModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if browserPoolReplacementRequired(plan, state) {
		resp.Diagnostics.AddWarning(
			"Browser Pool Will Be Replaced",
			"Applying this plan will replace the browser pool. Completing the replacement deletes the existing pool and all browsers in it. Kernel blocks this provider's non-forceful deletion while any browser is leased; release leased browsers before applying.",
		)
	}
	if !idleBrowserRebuildWarningRequired(plan, state, config) {
		return
	}

	resp.Diagnostics.AddAttributeWarning(
		path.Root("rebuild_idle_browsers_on_update"),
		"Idle Browser Rebuild May Be Applied",
		"Applying this plan may discard browsers that are currently idle so Kernel can replace them with the planned browser launch configuration. This occurs only if rebuild_idle_browsers_on_update resolves to true and a launch setting changes. Browsers that are warming or currently leased are not affected. Ready capacity may be reduced while the pool refills.",
	)
}

func browserPoolReplacementRequired(plan, state browserPoolModel) bool {
	// Keep these conditions aligned with the schema's replacement plan modifiers.
	projectChanges := !plan.ProjectID.IsUnknown() && !plan.ProjectID.Equal(state.ProjectID)
	clearsViewport := plan.Viewport.IsNull() && !state.Viewport.IsNull() && !state.Viewport.IsUnknown()
	return projectChanges || clearsString(plan.Name, state.Name) || clearsViewport
}

func idleBrowserRebuildWarningRequired(plan, state, config browserPoolModel) bool {
	rebuildPossible := isKnownBool(plan.RebuildIdle) && plan.RebuildIdle.ValueBool()
	rebuildPossible = rebuildPossible || config.RebuildIdle.IsUnknown() ||
		(isKnownBool(config.RebuildIdle) && config.RebuildIdle.ValueBool())
	if !rebuildPossible {
		return false
	}
	if !plan.ProjectID.IsUnknown() && !plan.ProjectID.Equal(state.ProjectID) {
		return false
	}

	var diags diag.Diagnostics
	validateSupportedUpdateClears(&diags, plan, state)
	if diags.HasError() {
		return false
	}

	return browserLaunchConfigurationMayChange(plan, state, config)
}

func (r *browserPoolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(browserPoolClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the browser pool durable client contract.",
		)
		return
	}

	r.client = client
}

func (r *browserPoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan browserPoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, diags := r.create(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *browserPoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state browserPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nextState, removed, diags := r.read(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if removed {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, nextState)...)
}

func (r *browserPoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan browserPoolModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state browserPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nextState, diags := r.update(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, nextState)...)
}

func (r *browserPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state browserPoolModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.delete(ctx, state)...)
}

func (r *browserPoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.client == nil {
		addMissingClientDiagnostic(&resp.Diagnostics)
		return
	}

	projectID, poolID, ok := parseImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid Kernel Browser Pool Import ID",
			"Cannot import \""+req.ID+"\": import a browser pool as \"<pool-id>\" or \"<project-id>/<pool-id>\". "+
				"The bare form resolves the project the same way create does: the provider default, "+
				"else the API key's binding. Use the project form to import from a different project.",
		)
		return
	}

	if projectID == "" {
		projectID = r.client.DefaultProjectID()
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectscope.StateValue(projectID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("extension_ids"), types.ListValueMust(types.StringType, nil))...)
}

func parseImportID(id string) (projectID, poolID string, ok bool) {
	before, after, found := strings.Cut(id, "/")
	if !found {
		return "", id, id != ""
	}
	if strings.Contains(after, "/") {
		return "", "", false
	}
	return before, after, before != "" && after != ""
}

func (r *browserPoolResource) create(ctx context.Context, plan browserPoolModel) (browserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return browserPoolModel{}, diags
	}

	params, expandDiags := expandCreateParams(ctx, plan)
	diags.Append(expandDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	projectID := projectscope.Resolve(plan.ProjectID, r.client.DefaultProjectID())
	pool, err := r.client.CreateBrowserPool(ctx, projectID, params)
	if err != nil {
		projectscope.AddError(&diags, "Create Kernel Browser Pool", projectID, err)
		return browserPoolModel{}, diags
	}
	if pool == nil {
		addNilResponseDiagnostic(&diags, "create")
		return browserPoolModel{}, diags
	}

	state, flattenDiags := flattenBrowserPool(*pool, plan)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	// The API response carries no project; record the one the pool was created in.
	state.ProjectID = projectscope.StateValue(projectID)
	return state, diags
}

func (r *browserPoolResource) update(ctx context.Context, plan, state browserPoolModel) (browserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return browserPoolModel{}, diags
	}

	id, ok := stateBrowserPoolID(state, "update", &diags)
	if !ok {
		return browserPoolModel{}, diags
	}

	params, hasPatch, expandDiags := expandUpdateParams(ctx, plan, state)
	diags.Append(expandDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}
	if !hasPatch {
		state.RebuildIdle = plan.RebuildIdle
		return state, diags
	}

	// Project changes replace the pool, so plan and state agree on the project here.
	projectID := state.ProjectID.ValueString()
	if _, err := r.client.UpdateBrowserPool(ctx, projectID, id, params); err != nil {
		projectscope.AddError(&diags, "Update Kernel Browser Pool", projectID, err)
		return browserPoolModel{}, diags
	}

	readBase := plan
	readBase.ID = state.ID
	readBase.ProjectID = state.ProjectID
	nextState, removed, readDiags := r.read(ctx, readBase)
	diags.Append(readDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}
	if removed {
		diags.AddError(
			"Read Kernel Browser Pool After Update",
			"Kernel browser pool "+id+" was not found after update.",
		)
		return browserPoolModel{}, diags
	}

	return nextState, diags
}

func (r *browserPoolResource) delete(ctx context.Context, state browserPoolModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return diags
	}

	id, ok := stateBrowserPoolID(state, "delete", &diags)
	if !ok {
		return diags
	}

	projectID := state.ProjectID.ValueString()
	if err := r.client.DeleteBrowserPool(ctx, projectID, id); err != nil {
		if projectscope.IsNotFound(err) {
			return diags
		}
		if browserPoolDeleteConflict(err) {
			diags.AddError(
				"Delete Kernel Browser Pool",
				"Kernel refused to delete browser pool "+id+" because one or more browsers are currently leased. Terraform will not force-delete leased browsers. Release active browsers and retry.",
			)
			return diags
		}
		projectscope.AddError(&diags, "Delete Kernel Browser Pool", projectID, err)
	}

	return diags
}

func (r *browserPoolResource) read(ctx context.Context, state browserPoolModel) (browserPoolModel, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return browserPoolModel{}, false, diags
	}
	id, ok := stateBrowserPoolID(state, "read", &diags)
	if !ok {
		return browserPoolModel{}, false, diags
	}

	projectID := state.ProjectID.ValueString()
	pool, err := r.client.GetBrowserPool(ctx, projectID, id)
	if err != nil {
		if projectscope.IsNotFound(err) {
			return browserPoolModel{}, true, diags
		}
		projectscope.AddError(&diags, "Read Kernel Browser Pool", projectID, err)
		return browserPoolModel{}, false, diags
	}
	if pool == nil {
		addNilResponseDiagnostic(&diags, "read")
		return browserPoolModel{}, false, diags
	}

	nextState, flattenDiags := flattenBrowserPool(*pool, state)
	diags.Append(flattenDiags...)
	nextState.ProjectID = state.ProjectID
	return nextState, false, diags
}

func stateBrowserPoolID(state browserPoolModel, operation string, diags *diag.Diagnostics) (string, bool) {
	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("id"),
			"Missing Kernel Browser Pool ID",
			"Cannot "+operation+" a Kernel browser pool without a known id in Terraform state.",
		)
		return "", false
	}

	return state.ID.ValueString(), true
}

func browserPoolDeleteConflict(err error) bool {
	var apiError *kernel.Error
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusBadRequest {
		return false
	}

	var body struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiError.RawJSON()), &body) != nil {
		return false
	}

	return body.Code == "pool_in_use" || body.Error.Code == "pool_in_use"
}

func addMissingClientDiagnostic(diags *diag.Diagnostics) {
	diags.AddError(
		"Missing Kernel Client",
		"The Kernel provider was not configured before using the browser pool resource.",
	)
}

func addNilResponseDiagnostic(diags *diag.Diagnostics, operation string) {
	diags.AddError(
		"Invalid Kernel Browser Pool Response",
		"Kernel returned an empty browser pool response during "+operation+".",
	)
}
