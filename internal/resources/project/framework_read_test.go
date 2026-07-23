package project

import (
	"context"
	"errors"
	"net/http"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFrameworkReadRefreshesState(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			project := projectForTest(t, `{"id":"project_123","name":"Renamed"}`)
			return &project, nil
		},
	})
	req, resp := frameworkReadRequest(t, projectModel{
		ID:   types.StringValue("project_123"),
		Name: types.StringValue("Original"),
	})

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	state := frameworkReadState(t, resp)
	if got := state.ID.ValueString(); got != "project_123" {
		t.Fatalf("state id = %q, want project_123", got)
	}
	if got := state.Name.ValueString(); got != "Renamed" {
		t.Fatalf("state name = %q, want Renamed", got)
	}
}

func TestFrameworkReadRemovesConfirmedMissingProject(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			return nil, projectAPIError(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	})
	req, resp := frameworkReadRequest(t, projectModel{
		ID:   types.StringValue("project_123"),
		Name: types.StringValue("Project"),
	})

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want removed state", resp.State.Raw)
	}
}

func TestFrameworkReadPreservesStateOnDiagnostic(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			return nil, errors.New("connection reset")
		},
	})
	want := projectModel{
		ID:   types.StringValue("project_123"),
		Name: types.StringValue("Project"),
	}
	req, resp := frameworkReadRequest(t, want)

	r.Read(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read diagnostic")
	}
	if got := frameworkReadState(t, resp); !got.ID.Equal(want.ID) || !got.Name.Equal(want.Name) {
		t.Fatalf("state = %#v, want preserved %#v", got, want)
	}
}

func TestFrameworkReadRejectsMalformedStateBeforeClientCall(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		get: func(ctx context.Context, id string) (*kernel.Project, error) {
			called = true
			return nil, nil
		},
	})
	var req tfresource.ReadRequest
	req.State.Schema = projectSchema()
	req.State.Raw = tftypes.NewValue(tftypes.String, "not project state")
	resp := &tfresource.ReadResponse{}
	resp.State.Schema = projectSchema()
	resp.State.Raw = req.State.Raw.Copy()

	r.Read(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-state diagnostic")
	}
	if called {
		t.Fatal("GetProject was called for malformed state")
	}
	if !resp.State.Raw.Equal(req.State.Raw) {
		t.Fatalf("state = %v, want preserved malformed state %v", resp.State.Raw, req.State.Raw)
	}
}

func frameworkReadRequest(t *testing.T, state projectModel) (tfresource.ReadRequest, *tfresource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	var req tfresource.ReadRequest
	req.State.Schema = projectSchema()
	if diags := req.State.Set(ctx, state); diags.HasError() {
		t.Fatalf("set read request state: %v", diags)
	}

	resp := &tfresource.ReadResponse{}
	resp.State.Schema = projectSchema()
	if diags := resp.State.Set(ctx, state); diags.HasError() {
		t.Fatalf("set read response state: %v", diags)
	}
	return req, resp
}

func frameworkReadState(t *testing.T, resp *tfresource.ReadResponse) projectModel {
	t.Helper()
	var state projectModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get read state: %v", diags)
	}
	return state
}
