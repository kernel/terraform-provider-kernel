package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
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
	detail := resp.Diagnostics[0].Detail()
	if want := `Terraform saved project ID "project_123" in state and will plan to replace this resource.`; !strings.Contains(detail, want) {
		t.Fatalf("diagnostic detail = %q, want persisted-state guidance %q", detail, want)
	}
	if strings.Contains(detail, "import its canonical project ID") {
		t.Fatalf("diagnostic detail = %q, must not recommend import for persisted state", detail)
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

	tests := map[string]struct {
		create     func(*testing.T) (*kernel.Project, error)
		wantDetail string
	}{
		"uncertain empty response": {
			create: func(t *testing.T) (*kernel.Project, error) {
				return nil, nil
			},
			wantDetail: "If it exists, import its canonical project ID before applying again. If it does not exist, retry the apply.",
		},
		"uncertain error without detail": {
			create: func(t *testing.T) (*kernel.Project, error) {
				return nil, errors.New("   ")
			},
			wantDetail: "Kernel returned an error without details.",
		},
		"definite API failure": {
			create: func(t *testing.T) (*kernel.Project, error) {
				return nil, projectAPIError(t, http.StatusConflict, `{"code":"conflict"}`)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newResourceWithClient(fakeProjectClient{
				create: func(ctx context.Context, params kernel.ProjectNewParams) (*kernel.Project, error) {
					return test.create(t)
				},
			})
			req, resp := frameworkCreateRequest(t, projectModel{Name: types.StringValue("Project")})

			r.Create(context.Background(), req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected create diagnostic")
			}
			if test.wantDetail != "" && !strings.Contains(resp.Diagnostics[0].Detail(), test.wantDetail) {
				t.Fatalf("diagnostic detail = %q, want recovery guidance %q", resp.Diagnostics[0].Detail(), test.wantDetail)
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
