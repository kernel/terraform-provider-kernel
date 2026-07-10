package project

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type projectCreateStatus uint8

const (
	projectCreateFailed projectCreateStatus = iota
	projectCreateSucceeded
	projectCreateUncertain
)

type projectCreateResult struct {
	State  projectModel
	Status projectCreateStatus
}

type projectClient interface {
	CreateProject(context.Context, kernel.ProjectNewParams) (*kernel.Project, error)
	GetProject(context.Context, string) (*kernel.Project, error)
}

type projectResource struct {
	client projectClient
}

func newResourceWithClient(client projectClient) *projectResource {
	return &projectResource{client: client}
}

func (r *projectResource) create(ctx context.Context, plan projectModel) (projectCreateResult, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.client == nil {
		addMissingClientDiagnostic(&diags)
		return projectCreateResult{Status: projectCreateFailed}, diags
	}

	params, expandDiags := expandProjectCreate(plan)
	diags.Append(expandDiags...)
	if diags.HasError() {
		return projectCreateResult{Status: projectCreateFailed}, diags
	}

	created, err := r.client.CreateProject(ctx, params)
	if err != nil {
		if projectCreateFailureIsDefinite(err) {
			diags.AddError("Create Kernel Project", err.Error())
			return projectCreateResult{Status: projectCreateFailed}, diags
		}
		addUncertainProjectCreateDiagnostic(&diags, plan.Name.ValueString(), err.Error())
		return projectCreateResult{Status: projectCreateUncertain}, diags
	}
	if created == nil {
		addUncertainProjectCreateDiagnostic(&diags, plan.Name.ValueString(), "Kernel returned an empty project response.")
		return projectCreateResult{Status: projectCreateUncertain}, diags
	}

	state, flattenDiags := flattenProject(*created)
	if flattenDiags.HasError() {
		reasons := make([]string, 0, len(flattenDiags))
		for _, flattenDiag := range flattenDiags {
			reasons = append(reasons, flattenDiag.Detail())
		}
		addUncertainProjectCreateDiagnostic(&diags, plan.Name.ValueString(), strings.Join(reasons, " "))
		return projectCreateResult{
			State:  partialProjectState(*created, plan),
			Status: projectCreateUncertain,
		}, diags
	}
	if !state.Name.Equal(plan.Name) {
		addUncertainProjectCreateDiagnostic(
			&diags,
			plan.Name.ValueString(),
			"Kernel returned project name "+strconv.Quote(state.Name.ValueString())+" instead of the requested name.",
		)
		return projectCreateResult{
			State:  partialProjectState(*created, plan),
			Status: projectCreateUncertain,
		}, diags
	}
	return projectCreateResult{State: state, Status: projectCreateSucceeded}, diags
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

func partialProjectState(project kernel.Project, plan projectModel) projectModel {
	if !validResponseString(project.JSON.ID.Raw(), project.JSON.ID.Valid(), project.ID) {
		return projectModel{}
	}
	return projectModel{
		ID:   types.StringValue(project.ID),
		Name: plan.Name,
	}
}

func addUncertainProjectCreateDiagnostic(diags *diag.Diagnostics, name, reason string) {
	diags.AddError(
		"Kernel Project Creation Outcome Uncertain",
		"Kernel may have created project "+strconv.Quote(name)+", but Terraform did not receive a complete confirmation. "+
			"Check Kernel for the project. If it exists and Terraform is not tracking it, import its project ID before applying again. "+
			"Reason: "+reason,
	)
}
