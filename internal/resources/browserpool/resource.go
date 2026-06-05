package browserpool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ resource.Resource              = (*browserPoolResource)(nil)
	_ resource.ResourceWithConfigure = (*browserPoolResource)(nil)
)

type browserPoolClient interface {
	DefaultProjectID() string
	CreateBrowserPool(context.Context, string, kernel.BrowserPoolNewParams) (*kernel.BrowserPool, error)
	GetBrowserPool(context.Context, string, string) (*kernel.BrowserPool, error)
}

type browserPoolResource struct {
	client browserPoolClient
}

func newResourceWithClient(client browserPoolClient) *browserPoolResource {
	return &browserPoolResource{client: client}
}

func (r *browserPoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_browser_pool"
}

func (r *browserPoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = BrowserPoolSchema()
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
	resp.Diagnostics.AddError(
		"Kernel Browser Pool Update Not Implemented",
		"Update support is intentionally left for the browser pool update/delete slice.",
	)
}

func (r *browserPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError(
		"Kernel Browser Pool Delete Not Implemented",
		"Delete support is intentionally left for the browser pool update/delete slice.",
	)
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

func (r *browserPoolResource) read(ctx context.Context, state browserPoolModel) (browserPoolModel, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return browserPoolModel{}, false, diags
	}
	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("id"),
			"Missing Kernel Browser Pool ID",
			"Cannot read a Kernel browser pool without a known id in Terraform state.",
		)
		return browserPoolModel{}, false, diags
	}

	projectID := state.ProjectID.ValueString()
	pool, err := r.client.GetBrowserPool(ctx, projectID, state.ID.ValueString())
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
