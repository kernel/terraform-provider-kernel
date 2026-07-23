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
			assertProjectNameDiagnostic(t, diags, "name must be known before creating a Kernel project.")
		})
	}
}

func TestExpandProjectUpdateBuildsNameOnlyPatch(t *testing.T) {
	t.Parallel()

	params, changed, diags := expandProjectUpdate(
		projectModel{Name: types.StringValue("Renamed")},
		projectModel{Name: types.StringValue("Original")},
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if got, want := params.UpdateProjectRequest.Name.Value, "Renamed"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if !params.UpdateProjectRequest.Name.Valid() {
		t.Fatal("name was omitted from update params")
	}

	body, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal update params: %v", err)
	}
	if got, want := string(body), `{"name":"Renamed"}`; got != want {
		t.Fatalf("update body = %s, want %s", got, want)
	}
}

func TestExpandProjectUpdateOmitsUnchangedName(t *testing.T) {
	t.Parallel()

	params, changed, diags := expandProjectUpdate(
		projectModel{Name: types.StringValue("Project")},
		projectModel{Name: types.StringValue("Project")},
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if changed {
		t.Fatal("changed = true, want false")
	}
	if params.UpdateProjectRequest.Name.Valid() {
		t.Fatal("unchanged name was included in update params")
	}
}

func TestExpandProjectUpdateRejectsUnknownOrNullName(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, changed, diags := expandProjectUpdate(
				projectModel{Name: value},
				projectModel{Name: types.StringValue("Project")},
			)
			if changed {
				t.Fatal("changed = true, want false")
			}
			assertProjectNameDiagnostic(t, diags, "name must be known before updating a Kernel project.")
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

func assertProjectNameDiagnostic(t *testing.T, diags diag.Diagnostics, wantDetail string) {
	t.Helper()
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one error", diags)
	}
	diagnostic := diags[0]
	if got, want := diagnostic.Summary(), "Invalid Project Name"; got != want {
		t.Fatalf("diagnostic summary = %q, want %q", got, want)
	}
	if got := diagnostic.Detail(); got != wantDetail {
		t.Fatalf("diagnostic detail = %q, want %q", got, wantDetail)
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
