package project

import (
	"context"
	"net/http"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFrameworkCreatePersistsSuccessfulState(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
			project := projectForTest(t, `{"id":"project_123","name":"Project"}`)
			return &project, nil
		},
	})
	req, resp := frameworkCreateRequest(t, projectModel{Name: types.StringValue("Project")})

	r.Create(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	state := frameworkCreateState(t, resp)
	if got := state.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want project_123", got)
	}
	if got := state.Name.ValueString(); got != "Project" {
		t.Fatalf("state name = %q, want Project", got)
	}
}

func TestFrameworkCreatePersistsRecoverableIdentityBeforeUncertainDiagnostic(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
			project := projectForTest(t, `{"id":"project_123"}`)
			return &project, nil
		},
	})
	req, resp := frameworkCreateRequest(t, projectModel{Name: types.StringValue("Project")})

	r.Create(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected uncertain-create diagnostic")
	}
	state := frameworkCreateState(t, resp)
	if got := state.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want recoverable project_123", got)
	}
	if got := state.Name.ValueString(); got != "Project" {
		t.Fatalf("state name = %q, want planned Project", got)
	}
}

func TestFrameworkCreateLeavesNoStateWithoutRecoverableIdentity(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T) (*kernel.Project, error){
		"uncertain empty response": func(t *testing.T) (*kernel.Project, error) {
			return nil, nil
		},
		"definite API failure": func(t *testing.T) (*kernel.Project, error) {
			return nil, projectAPIError(t, http.StatusConflict, `{"code":"conflict"}`)
		},
	}

	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return create(t)
				},
			})
			req, resp := frameworkCreateRequest(t, projectModel{Name: types.StringValue("Project")})

			r.Create(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected create diagnostic")
			}
			if !resp.State.Raw.IsNull() {
				t.Fatalf("state = %v, want absent state", resp.State.Raw)
			}
		})
	}
}

func TestFrameworkCreateRejectsMalformedPlanBeforeClientCall(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
			called = true
			return nil, nil
		},
	})
	var req tfresource.CreateRequest
	req.Plan.Schema = projectSchema()
	req.Plan.Raw = tftypes.NewValue(tftypes.String, "not a project plan")
	resp := &tfresource.CreateResponse{}
	resp.State.Schema = projectSchema()
	resp.State.RemoveResource(context.Background())

	r.Create(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-plan diagnostic")
	}
	if called {
		t.Fatal("CreateProject was called for a malformed plan")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want absent state", resp.State.Raw)
	}
}

func frameworkCreateRequest(t *testing.T, plan projectModel) (tfresource.CreateRequest, *tfresource.CreateResponse) {
	t.Helper()
	ctx := context.Background()

	var req tfresource.CreateRequest
	req.Plan.Schema = projectSchema()
	if diags := req.Plan.Set(ctx, plan); diags.HasError() {
		t.Fatalf("set create plan: %v", diags)
	}

	resp := &tfresource.CreateResponse{}
	resp.State.Schema = projectSchema()
	resp.State.RemoveResource(ctx)
	return req, resp
}

func frameworkCreateState(t *testing.T, resp *tfresource.CreateResponse) projectModel {
	t.Helper()
	var state projectModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get create state: %v", diags)
	}
	return state
}
