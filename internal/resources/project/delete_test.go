package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDeleteProjectUsesCanonicalStateID(t *testing.T) {
	t.Parallel()

	var gotID string
	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			gotID = id
			return nil
		},
	})

	diags := r.delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotID != "project_123" {
		t.Fatalf("DeleteProject id = %q, want canonical state id", gotID)
	}
}

func TestDeleteProjectTreatsCodedNotFoundAsSuccess(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			return projectAPIError(t, http.StatusNotFound, `{"code":"not_found","message":"project not found"}`)
		},
	})

	diags := r.delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestDeleteProjectExplainsLifecycleConflictsWithoutForcingChildren(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		code    string
		message string
	}{
		"last active project": {
			code:    "last_active_project",
			message: "organization must have at least one project",
		},
		"project not empty": {
			code:    "project_not_empty",
			message: "project still has active resources",
		},
		"legacy conflict": {
			code:    "conflict",
			message: "project cannot be deleted",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			deleteErr := projectAPIError(t, http.StatusConflict, `{"code":"`+test.code+`","message":"`+test.message+`"}`)
			r := newResourceWithClient(fakeProjectClient{
				delete: func(ctx context.Context, id string) error {
					return deleteErr
				},
			})

			diags := r.delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if got := diags[0].Summary(); got != "Delete Kernel Project" {
				t.Fatalf("diagnostic summary = %q, want Delete Kernel Project", got)
			}
			detail := diags[0].Detail()
			for _, want := range []string{
				"must have no active resources",
				"retain at least one active project",
				"will not delete child resources implicitly",
				"409 Conflict",
			} {
				if !strings.Contains(detail, want) {
					t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, want)
				}
			}
		})
	}
}

func TestDeleteProjectDoesNotTreatProjectsDisabledAsSuccess(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			return projectAPIError(t, http.StatusNotFound, `{"code":"projects_disabled","message":"projects are disabled for this organization"}`)
		},
	})

	diags := r.delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one error", diags)
	}
	if got := diags[0].Summary(); got != "Delete Kernel Project" {
		t.Fatalf("diagnostic summary = %q, want Delete Kernel Project", got)
	}
	if detail := diags[0].Detail(); !strings.Contains(detail, "404 Not Found") {
		t.Fatalf("diagnostic detail = %q, want it to contain 404 Not Found", detail)
	}
}

func TestDeleteProjectReturnsOtherErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        func(*testing.T) error
		wantDetail string
	}{
		"transport error": {
			err: func(t *testing.T) error {
				return errors.New("connection reset")
			},
			wantDetail: "connection reset",
		},
		"forbidden": {
			err: func(t *testing.T) error {
				return projectAPIError(t, http.StatusForbidden, `{"code":"forbidden","message":"project-scoped API keys cannot delete projects"}`)
			},
			wantDetail: "403 Forbidden",
		},
		"server error": {
			err: func(t *testing.T) error {
				return projectAPIError(t, http.StatusInternalServerError, `{"code":"db_error","message":"failed to delete project"}`)
			},
			wantDetail: "500 Internal Server Error",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			deleteErr := test.err(t)
			r := newResourceWithClient(fakeProjectClient{
				delete: func(ctx context.Context, id string) error {
					return deleteErr
				},
			})

			diags := r.delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if got := diags[0].Summary(); got != "Delete Kernel Project" {
				t.Fatalf("diagnostic summary = %q, want Delete Kernel Project", got)
			}
			if detail := diags[0].Detail(); !strings.Contains(detail, test.wantDetail) {
				t.Fatalf("diagnostic detail = %q, want it to contain %q", detail, test.wantDetail)
			}
		})
	}
}

func TestDeleteProjectRejectsMissingStateID(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			called = true
			return nil
		},
	})

	diags := r.delete(context.Background(), projectModel{ID: types.StringNull()})
	if !diags.HasError() || diags[0].Summary() != "Missing Kernel Project ID" {
		t.Fatalf("diagnostics = %v, want missing-id error", diags)
	}
	if called {
		t.Fatal("DeleteProject was called without a state id")
	}
}

func TestDeleteProjectRequiresConfiguredClient(t *testing.T) {
	t.Parallel()

	diags := (&projectResource{}).delete(context.Background(), projectModel{ID: types.StringValue("project_123")})
	if !diags.HasError() || diags[0].Summary() != "Missing Kernel Client" {
		t.Fatalf("diagnostics = %v, want missing-client error", diags)
	}
}
