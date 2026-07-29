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

func TestReadProjectGetsCanonicalIDAndFlattensState(t *testing.T) {
	t.Parallel()

	var gotID string
	r := newResourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			gotID = id
			project := projectForTest(t, `{"id":"project_123","name":"Renamed"}`)
			return &project, nil
		},
	})

	state, removed, diags := r.read(context.Background(), projectModel{
		ID:   types.StringValue("project_123"),
		Name: types.StringValue("Original"),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if removed {
		t.Fatal("removed = true, want false")
	}
	if gotID != "project_123" {
		t.Fatalf("GetProject id = %q, want canonical state id", gotID)
	}
	if got := state.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want project_123", got)
	}
	if got := state.Name.ValueString(); got != "Renamed" {
		t.Fatalf("state name = %q, want remote durable name", got)
	}
}

func TestReadProjectRemovesStateOnlyForCodedNotFound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err         error
		wantRemoved bool
		wantError   bool
	}{
		"coded not found": {
			err:         projectAPIError(t, http.StatusNotFound, `{"code":"not_found","message":"project not found"}`),
			wantRemoved: true,
		},
		"uncoded not found": {
			err:       projectAPIError(t, http.StatusNotFound, `{}`),
			wantError: true,
		},
		"transport error": {
			err:       errors.New("connection reset"),
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newResourceWithClient(fakeProjectClient{
				get: func(ctx context.Context, id string) (*kernel.Project, error) {
					return nil, test.err
				},
			})

			_, removed, diags := r.read(context.Background(), projectModel{ID: types.StringValue("project_123")})
			if removed != test.wantRemoved {
				t.Fatalf("removed = %t, want %t", removed, test.wantRemoved)
			}
			if diags.HasError() != test.wantError {
				t.Fatalf("has error = %t, want %t; diagnostics: %v", diags.HasError(), test.wantError, diags)
			}
		})
	}
}

func TestReadProjectRejectsMissingStateID(t *testing.T) {
	t.Parallel()

	tests := map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
		"empty":   types.StringValue(""),
	}

	for name, id := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			called := false
			r := newResourceWithClient(fakeProjectClient{
				get: func(ctx context.Context, id string) (*kernel.Project, error) {
					called = true
					return nil, errors.New("unexpected get")
				},
			})

			_, removed, diags := r.read(context.Background(), projectModel{ID: id})
			if removed {
				t.Fatal("removed = true, want false")
			}
			if !diags.HasError() {
				t.Fatal("expected missing-id diagnostic")
			}
			if called {
				t.Fatal("GetProject was called without a usable state id")
			}
		})
	}
}

func TestReadProjectRejectsInvalidResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		get        func(*testing.T, context.Context, string) (*kernel.Project, error)
		stateID    string
		wantDetail string
	}{
		"empty response": {
			get: func(t *testing.T, ctx context.Context, id string) (*kernel.Project, error) {
				return nil, nil
			},
			wantDetail: "empty response",
		},
		"missing durable name": {
			get: func(t *testing.T, ctx context.Context, id string) (*kernel.Project, error) {
				project := projectForTest(t, `{"id":"project_123"}`)
				return &project, nil
			},
			wantDetail: "missing or invalid field name",
		},
		"mismatched id": {
			get: func(t *testing.T, ctx context.Context, id string) (*kernel.Project, error) {
				if id != "Production" {
					t.Fatalf("GetProject id = %q, want imported name", id)
				}
				project := projectForTest(t, `{"id":"project_other","name":"Project"}`)
				return &project, nil
			},
			stateID: "Production",
			wantDetail: `returned canonical project ID "project_other" while reading Terraform state ID "Production". ` +
				`If this project was imported by name, remove its existing Terraform state entry, then import it again using canonical project ID "project_other". ` +
				`Terraform preserved the existing state.`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stateID := test.stateID
			if stateID == "" {
				stateID = "project_123"
			}
			r := newResourceWithClient(fakeProjectClient{
				get: func(ctx context.Context, id string) (*kernel.Project, error) {
					return test.get(t, ctx, id)
				},
			})
			_, removed, diags := r.read(context.Background(), projectModel{ID: types.StringValue(stateID)})
			if removed {
				t.Fatal("removed = true, want false")
			}
			if !diags.HasError() {
				t.Fatal("expected invalid-response diagnostic")
			}
			if !strings.Contains(diags[0].Detail(), test.wantDetail) {
				t.Fatalf("diagnostic detail = %q, want it to contain %q", diags[0].Detail(), test.wantDetail)
			}
		})
	}
}

func TestReadProjectRequiresConfiguredClient(t *testing.T) {
	t.Parallel()

	_, removed, diags := (&projectResource{}).read(context.Background(), projectModel{ID: types.StringValue("project_123")})
	if removed {
		t.Fatal("removed = true, want false")
	}
	if !diags.HasError() || diags[0].Summary() != "Missing Kernel Client" {
		t.Fatalf("diagnostics = %v, want missing-client error", diags)
	}
}
