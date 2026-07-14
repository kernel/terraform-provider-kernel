package project

import (
	"context"
	"errors"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFrameworkUpdateWritesValidatedState(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
			project := projectForTest(t, `{"id":"project_123","name":"Renamed"}`)
			return &project, nil
		},
	})
	state := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")}
	plan := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Renamed")}
	req, resp := frameworkUpdateRequest(t, plan, state)

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	got := frameworkUpdateState(t, resp)
	if !got.ID.Equal(state.ID) || !got.Name.Equal(plan.Name) {
		t.Fatalf("state = %#v, want id %v and name %v", got, state.ID, plan.Name)
	}
}

func TestFrameworkUpdateSkipsNoOpAndPreservesState(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
			called = true
			return nil, errors.New("unexpected update")
		},
	})
	state := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Project")}
	req, resp := frameworkUpdateRequest(t, state, state)

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if called {
		t.Fatal("UpdateProject was called for a no-op plan")
	}
	if got := frameworkUpdateState(t, resp); !got.ID.Equal(state.ID) || !got.Name.Equal(state.Name) {
		t.Fatalf("state = %#v, want preserved %#v", got, state)
	}
}

func TestFrameworkUpdatePreservesStateOnDiagnostic(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
			return nil, errors.New("connection reset")
		},
	})
	state := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")}
	plan := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Renamed")}
	req, resp := frameworkUpdateRequest(t, plan, state)

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected update diagnostic")
	}
	if got := frameworkUpdateState(t, resp); !got.ID.Equal(state.ID) || !got.Name.Equal(state.Name) {
		t.Fatalf("state = %#v, want preserved %#v", got, state)
	}
}

func TestFrameworkUpdateRejectsMalformedInputBeforeClientCall(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"plan", "state"} {
		t.Run("malformed "+target, func(t *testing.T) {
			t.Parallel()
			called := false
			r := newResourceWithClient(fakeProjectClient{
				update: func(ctx context.Context, id string, params kernel.ProjectUpdateParams) (*kernel.Project, error) {
					called = true
					return nil, nil
				},
			})
			state := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Original")}
			plan := projectModel{ID: types.StringValue("project_123"), Name: types.StringValue("Renamed")}
			req, resp := frameworkUpdateRequest(t, plan, state)
			malformed := tftypes.NewValue(tftypes.String, "not a project value")
			if target == "plan" {
				req.Plan.Raw = malformed
			} else {
				req.State.Raw = malformed
			}

			r.Update(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected malformed-input diagnostic")
			}
			if called {
				t.Fatal("UpdateProject was called for malformed input")
			}
			if got := frameworkUpdateState(t, resp); !got.ID.Equal(state.ID) || !got.Name.Equal(state.Name) {
				t.Fatalf("state = %#v, want preserved %#v", got, state)
			}
		})
	}
}

func frameworkUpdateRequest(t *testing.T, plan, state projectModel) (tfresource.UpdateRequest, *tfresource.UpdateResponse) {
	t.Helper()
	ctx := context.Background()

	var req tfresource.UpdateRequest
	req.Plan.Schema = projectSchema()
	if diags := req.Plan.Set(ctx, plan); diags.HasError() {
		t.Fatalf("set update plan: %v", diags)
	}
	req.State.Schema = projectSchema()
	if diags := req.State.Set(ctx, state); diags.HasError() {
		t.Fatalf("set update request state: %v", diags)
	}

	resp := &tfresource.UpdateResponse{}
	resp.State.Schema = projectSchema()
	if diags := resp.State.Set(ctx, state); diags.HasError() {
		t.Fatalf("set update response state: %v", diags)
	}
	return req, resp
}

func frameworkUpdateState(t *testing.T, resp *tfresource.UpdateResponse) projectModel {
	t.Helper()
	var state projectModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get update state: %v", diags)
	}
	return state
}
