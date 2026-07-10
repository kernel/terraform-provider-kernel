package project

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func expandProjectCreate(model projectModel) (kernel.ProjectNewParams, diag.Diagnostics) {
	var diags diag.Diagnostics
	if model.Name.IsNull() || model.Name.IsUnknown() {
		diags.AddAttributeError(
			path.Root("name"),
			"Invalid Project Name",
			"name must be known before creating a Kernel project.",
		)
		return kernel.ProjectNewParams{}, diags
	}

	return kernel.ProjectNewParams{
		CreateProjectRequest: kernel.CreateProjectRequestParam{
			Name: model.Name.ValueString(),
		},
	}, diags
}

func flattenProject(project kernel.Project) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !validResponseString(project.JSON.ID.Raw(), project.JSON.ID.Valid(), project.ID) {
		addInvalidResponseDiagnostic(&diags, "id")
	}
	if !validResponseString(project.JSON.Name.Raw(), project.JSON.Name.Valid(), project.Name) {
		addInvalidResponseDiagnostic(&diags, "name")
	}
	if diags.HasError() {
		return projectModel{}, diags
	}

	return projectModel{
		ID:   types.StringValue(project.ID),
		Name: types.StringValue(project.Name),
	}, diags
}

func validResponseString(raw string, valid bool, value string) bool {
	if raw == "" || !valid || value == "" {
		return false
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func addInvalidResponseDiagnostic(diags *diag.Diagnostics, field string) {
	diags.AddError(
		"Invalid Kernel Project Response",
		"Kernel returned a project with missing or invalid field "+field+".",
	)
}
