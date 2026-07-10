package project

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestExpandProjectCreate(t *testing.T) {
	t.Parallel()

	params, diags := expandProjectCreate(projectModel{Name: types.StringValue("Project")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := params.CreateProjectRequest.Name, "Project"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}

func TestExpandProjectCreateRejectsUnknownOrNullName(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := expandProjectCreate(projectModel{Name: value})
			assertProjectNameDiagnostic(t, diags)
		})
	}
}

func TestFlattenProjectMapsDurableState(t *testing.T) {
	t.Parallel()

	state, diags := flattenProject(projectForTest(t, `{"id":"project_123","name":"Project"}`))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := state.ID.ValueString(), "project_123"; got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if got, want := state.Name.ValueString(), "Project"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}

func TestFlattenProjectRejectsInvalidDurableFields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body  string
		field string
	}{
		"missing id":   {body: `{"name":"Project"}`, field: "id"},
		"numeric id":   {body: `{"id":123,"name":"Project"}`, field: "id"},
		"missing name": {body: `{"id":"project_123"}`, field: "name"},
		"numeric name": {body: `{"id":"project_123","name":123}`, field: "name"},
		"empty name":   {body: `{"id":"project_123","name":""}`, field: "name"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenProject(projectForTest(t, test.body))
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if got, want := diags[0].Summary(), "Invalid Kernel Project Response"; got != want {
				t.Fatalf("diagnostic summary = %q, want %q", got, want)
			}
			if got, want := diags[0].Detail(), "Kernel returned a project with missing or invalid field "+test.field+"."; got != want {
				t.Fatalf("diagnostic detail = %q, want %q", got, want)
			}
		})
	}
}

func assertProjectNameDiagnostic(t *testing.T, diags diag.Diagnostics) {
	t.Helper()
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one error", diags)
	}
	diagnostic := diags[0]
	if got, want := diagnostic.Summary(), "Invalid Project Name"; got != want {
		t.Fatalf("diagnostic summary = %q, want %q", got, want)
	}
	if got, want := diagnostic.Detail(), "name must be known before creating a Kernel project."; got != want {
		t.Fatalf("diagnostic detail = %q, want %q", got, want)
	}
	withPath, ok := diagnostic.(diag.DiagnosticWithPath)
	if !ok || !withPath.Path().Equal(path.Root("name")) {
		t.Fatalf("diagnostic path = %v, want name", withPath)
	}
}

func projectForTest(t *testing.T, body string) kernel.Project {
	t.Helper()
	var project kernel.Project
	if err := json.Unmarshal([]byte(body), &project); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}
	return project
}
