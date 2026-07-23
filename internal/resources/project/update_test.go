package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestUpdateProjectUsesCanonicalIDAndFlattensResponse(t *testing.T) {
	t.Parallel()

	var gotID string
	var gotParams kernel.ProjectUpdateParams
	r := newResourceWithClient(fakeProjectClient{
		update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
			gotID = id
			gotParams = params
			project := projectForTest(t, `{"id":"project_123","name":"Renamed"}`)
			return &project, nil
		},
	})

	nextState, diags := r.update(
		context.Background(),
		projectModel{ID: types.StringValue("ignored-plan-id"), Name: types.StringValue("Renamed")},
		projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotID != "project_123" {
		t.Fatalf("UpdateProject id = %q, want canonical state id", gotID)
	}
	if got, want := gotParams.UpdateProjectRequest.Name.Value, "Renamed"; got != want {
		t.Fatalf("update name = %q, want %q", got, want)
	}
	if got := nextState.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want project_123", got)
	}
	if got := nextState.Name.ValueString(); got != "Renamed" {
		t.Fatalf("state name = %q, want Renamed", got)
	}
}

func TestUpdateProjectSkipsUnchangedName(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
			called = true
			return nil, errors.New("unexpected update")
		},
	})
	state := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Project")}

	nextState, diags := r.update(context.Background(), state, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if called {
		t.Fatal("UpdateProject was called for an unchanged name")
	}
	if !nextState.ID.Equal(state.ID) || !nextState.Name.Equal(state.Name) {
		t.Fatalf("state = %#v, want unchanged %#v", nextState, state)
	}
}

func TestUpdateProjectReturnsDiagnostics(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		state       projectModel
		response    string
		err         func(*testing.T) error
		wantSummary string
		wantDetail  string
	}{
		"missing state id": {
			state:       projectModel{ID: types.StringNull(), Name: types.StringValue("Original")},
			wantSummary: "Missing Kernel Project ID",
			wantDetail:  "without a known id",
		},
		"client error": {
			state: projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
			err: func(t *testing.T) error {
				return projectAPIError(t, http.StatusConflict, `{"code":"conflict"}`)
			},
			wantSummary: "Update Kernel Project",
			wantDetail:  "409 Conflict",
		},
		"empty response": {
			state:       projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
			wantSummary: "Update Kernel Project",
			wantDetail:  "empty response",
		},
		"malformed response": {
			state:       projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
			response:    `{"id":"project_123"}`,
			wantSummary: "Invalid Kernel Project Response",
			wantDetail:  "missing or invalid field name",
		},
		"mismatched response id": {
			state:       projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
			response:    `{"id":"project_other","name":"Renamed"}`,
			wantSummary: "Invalid Kernel Project Response",
			wantDetail:  `returned project id "project_other" while updating "project_123"`,
		},
		"mismatched response name": {
			state:       projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
			response:    `{"id":"project_123","name":"Different"}`,
			wantSummary: "Invalid Kernel Project Response",
			wantDetail:  `returned project name "Different" after updating to "Renamed"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var response *kernel.Project
			if test.response != "" {
				project := projectForTest(t, test.response)
				response = &project
			}
			var updateErr error
			if test.err != nil {
				updateErr = test.err(t)
			}
			called := false
			r := newResourceWithClient(fakeProjectClient{
				update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
					called = true
					return response, updateErr
				},
			})

			_, diags := r.update(
				context.Background(),
				projectModel{Name: types.StringValue("Renamed")},
				test.state,
			)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if got := diags[0].Summary(); got != test.wantSummary {
				t.Fatalf("diagnostic summary = %q, want %q", got, test.wantSummary)
			}
			if detail := diags[0].Detail(); !strings.Contains(detail, test.wantDetail) {
				t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, test.wantDetail)
			}
			if test.state.ID.IsNull() && called {
				t.Fatal("UpdateProject was called without a state id")
			}
		})
	}
}

func TestUpdateProjectRequiresConfiguredClient(t *testing.T) {
	t.Parallel()

	_, diags := (&projectResource{}).update(
		context.Background(),
		projectModel{Name: types.StringValue("Renamed")},
		projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")},
	)
	if !diags.HasError() || diags[0].Summary() != "Missing Kernel Client" {
		t.Fatalf("diagnostics = %v, want missing-client error", diags)
	}
}
