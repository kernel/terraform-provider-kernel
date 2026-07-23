package project

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ tfresource.Resource                = (*projectResource)(nil)
	_ tfresource.ResourceWithConfigure   = (*projectResource)(nil)
	_ tfresource.ResourceWithImportState = (*projectResource)(nil)
)

type projectCreateResult struct {
	State        projectModel
	UncertainErr error
}

type projectClient interface {
	CreateProject(context.Context, kernel.ProjectNewParams) (*kernel.Project, error)
	GetProject(context.Context, string) (*kernel.Project, error)
	UpdateProject(context.Context, string, kernel.ProjectUpdateParams) (*kernel.Project, error)
	DeleteProject(context.Context, string) error
}

type projectResource struct {
	client projectClient
}

func newResourceWithClient(client projectClient) *projectResource {
	return &projectResource{client: client}
}

func NewResource() tfresource.Resource {
	return &projectResource{}
}

func (r *projectResource) Metadata(ctx context.Context, req tfresource.MetadataRequest, resp *tfresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(ctx context.Context, req tfresource.SchemaRequest, resp *tfresource.SchemaResponse) {
	resp.Schema = projectSchema()
}

func (r *projectResource) Configure(ctx context.Context, req tfresource.ConfigureRequest, resp *tfresource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(projectClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the project durable client contract.",
		)
		return
	}

	r.client = client
}

func projectImportState(id string) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if id == "" {
		diags.AddError(
			"Invalid Kernel Project Import ID",
			"Import a Kernel project using its canonical project ID.",
		)
		return projectModel{}, diags
	}

	return projectModel{
		ID:   types.StringValue(id),
		Name: types.StringUnknown(),
	}, diags
}

func (r *projectResource) Create(ctx context.Context, req tfresource.CreateRequest, resp *tfresource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, createDiags := r.create(ctx, plan)
	resp.Diagnostics.Append(createDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if result.UncertainErr == nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, result.State)...)
		return
	}

	projectID := result.State.ID.ValueString()
	if projectID != "" {
		stateDiags := resp.State.Set(ctx, result.State)
		resp.Diagnostics.Append(stateDiags...)
		if stateDiags.HasError() {
			projectID = ""
		}
	}
	addUncertainProjectCreateDiagnostic(&resp.Diagnostics, plan.Name.ValueString(), projectID, result.UncertainErr.Error())
}

func (r *projectResource) Read(ctx context.Context, req tfresource.ReadRequest, resp *tfresource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nextState, removed, readDiags := r.read(ctx, state)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if removed {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, nextState)...)
}

func (r *projectResource) Update(ctx context.Context, req tfresource.UpdateRequest, resp *tfresource.UpdateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nextState, updateDiags := r.update(ctx, plan, state)
	resp.Diagnostics.Append(updateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, nextState)...)
}

func (r *projectResource) Delete(ctx context.Context, req tfresource.DeleteRequest, resp *tfresource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.delete(ctx, state)...)
}

func (r *projectResource) ImportState(ctx context.Context, req tfresource.ImportStateRequest, resp *tfresource.ImportStateResponse) {
	state, importDiags := projectImportState(req.ID)
	resp.Diagnostics.Append(importDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectResource) create(ctx context.Context, plan projectModel) (projectCreateResult, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return projectCreateResult{}, diags
	}

	params, expandDiags := expandProjectCreate(plan)
	diags.Append(expandDiags...)
	if diags.HasError() {
		return projectCreateResult{}, diags
	}

	created, err := r.client.CreateProject(ctx, params)
	if err != nil {
		if projectCreateFailureIsDefinite(err) {
			diags.AddError("Create Kernel Project", err.Error())
			return projectCreateResult{}, diags
		}
		return projectCreateResult{UncertainErr: err}, diags
	}
	if created == nil {
		return projectCreateResult{UncertainErr: errors.New("Kernel returned an empty project response.")}, diags
	}

	state, flattenDiags := flattenProject(*created)
	if flattenDiags.HasError() {
		reasons := make([]string, 0, len(flattenDiags))
		for _, flattenDiag := range flattenDiags {
			reasons = append(reasons, flattenDiag.Detail())
		}
		return projectCreateResult{
			State:        partialProjectState(*created, plan),
			UncertainErr: errors.New(strings.Join(reasons, " ")),
		}, diags
	}
	if !state.Name.Equal(plan.Name) {
		return projectCreateResult{
			State:        partialProjectState(*created, plan),
			UncertainErr: errors.New("Kernel returned project name " + strconv.Quote(state.Name.ValueString()) + " instead of the requested name."),
		}, diags
	}
	return projectCreateResult{State: state}, diags
}

func (r *projectResource) read(ctx context.Context, state projectModel) (projectModel, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return projectModel{}, false, diags
	}

	id, ok := stateProjectID(state, "read", &diags)
	if !ok {
		return projectModel{}, false, diags
	}

	remote, err := r.client.GetProject(ctx, id)
	if err != nil {
		if projectscope.IsNotFound(err) {
			return projectModel{}, true, diags
		}
		diags.AddError("Read Kernel Project", err.Error())
		return projectModel{}, false, diags
	}
	if remote == nil {
		diags.AddError(
			"Read Kernel Project",
			"Kernel returned an empty response while reading project "+strconv.Quote(id)+".",
		)
		return projectModel{}, false, diags
	}

	nextState, flattenDiags := flattenProject(*remote)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return projectModel{}, false, diags
	}
	if nextState.ID.ValueString() != id {
		diags.AddError(
			"Invalid Kernel Project Response",
			"Kernel returned project id "+strconv.Quote(nextState.ID.ValueString())+" while reading "+strconv.Quote(id)+".",
		)
		return projectModel{}, false, diags
	}
	return nextState, false, diags
}

func (r *projectResource) update(ctx context.Context, plan, state projectModel) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return projectModel{}, diags
	}

	id, ok := stateProjectID(state, "update", &diags)
	if !ok {
		return projectModel{}, diags
	}

	params, changed, expandDiags := expandProjectUpdate(plan, state)
	diags.Append(expandDiags...)
	if diags.HasError() {
		return projectModel{}, diags
	}
	if !changed {
		return state, diags
	}

	remote, err := r.client.UpdateProject(ctx, id, params)
	if err != nil {
		diags.AddError("Update Kernel Project", err.Error())
		return projectModel{}, diags
	}
	if remote == nil {
		diags.AddError(
			"Update Kernel Project",
			"Kernel returned an empty response while updating project "+strconv.Quote(id)+".",
		)
		return projectModel{}, diags
	}

	nextState, flattenDiags := flattenProject(*remote)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return projectModel{}, diags
	}
	if nextState.ID.ValueString() != id {
		diags.AddError(
			"Invalid Kernel Project Response",
			"Kernel returned project id "+strconv.Quote(nextState.ID.ValueString())+" while updating "+strconv.Quote(id)+".",
		)
		return projectModel{}, diags
	}
	if !nextState.Name.Equal(plan.Name) {
		diags.AddError(
			"Invalid Kernel Project Response",
			"Kernel returned project name "+strconv.Quote(nextState.Name.ValueString())+" after updating to "+strconv.Quote(plan.Name.ValueString())+".",
		)
		return projectModel{}, diags
	}

	return nextState, diags
}

func (r *projectResource) delete(ctx context.Context, state projectModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return diags
	}

	id, ok := stateProjectID(state, "delete", &diags)
	if !ok {
		return diags
	}

	if err := r.client.DeleteProject(ctx, id); err != nil {
		// Kernel also uses not_found when projects are disabled. Terraform still
		// treats it as absence because the API provides no distinguishable signal.
		if projectscope.IsNotFound(err) {
			return diags
		}
		if projectDeleteConflict(err) {
			diags.AddError(
				"Delete Kernel Project",
				"Kernel refused to delete project "+strconv.Quote(id)+". A project must have no active resources, and its organization must retain at least one active project. Terraform will not delete child resources implicitly. Underlying error: "+err.Error(),
			)
			return diags
		}
		diags.AddError("Delete Kernel Project", err.Error())
	}

	return diags
}

func addMissingClientDiagnostic(diags *diag.Diagnostics) {
	diags.AddError(
		"Missing Kernel Client",
		"The Kernel provider was not configured before using the project resource.",
	)
}

func stateProjectID(state projectModel, operation string, diags *diag.Diagnostics) (string, bool) {
	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("id"),
			"Missing Kernel Project ID",
			"Cannot "+operation+" a Kernel project without a known id in Terraform state.",
		)
		return "", false
	}

	return state.ID.ValueString(), true
}

func projectCreateFailureIsDefinite(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) && apiError.StatusCode >= http.StatusBadRequest && apiError.StatusCode < http.StatusInternalServerError
}

func projectDeleteConflict(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) && apiError.StatusCode == http.StatusConflict
}

func partialProjectState(project kernel.Project, plan projectModel) projectModel {
	if !validResponseString(project.JSON.ID.Raw(), project.JSON.ID.Valid(), project.ID) {
		return projectModel{}
	}
	return projectModel{
		ID:   types.StringValue(project.ID),
		Name: plan.Name,
	}
}

func addUncertainProjectCreateDiagnostic(diags *diag.Diagnostics, name, projectID, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Kernel returned an error without details."
	}

	recovery := "Check Kernel for the project. If it exists, import its canonical project ID before applying again. If it does not exist, retry the apply."
	if projectID != "" {
		recovery = "Terraform saved project ID " + strconv.Quote(projectID) + " in state and will plan to replace this resource. Check the project in Kernel before applying again."
	}

	diags.AddError(
		"Kernel Project Creation Outcome Uncertain",
		"Kernel may have created project "+strconv.Quote(name)+", but Terraform did not receive a complete confirmation. "+
			recovery+" "+
			"Reason: "+reason,
	)
}
