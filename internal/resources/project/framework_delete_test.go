package project

import (
	"context"
	"errors"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestFrameworkDeleteCallsCoreWithStateID(t *testing.T) {
	t.Parallel()

	var gotID string
	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			gotID = id
			return nil
		},
	})
	req := frameworkDeleteRequest(t, projectModel{ID: types.StringValue("project_123")})
	var resp tfresource.DeleteResponse

	r.Delete(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotID != "project_123" {
		t.Fatalf("DeleteProject id = %q, want project_123", gotID)
	}
}

func TestFrameworkDeletePropagatesCoreDiagnostic(t *testing.T) {
	t.Parallel()

	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			return errors.New("connection reset")
		},
	})
	req := frameworkDeleteRequest(t, projectModel{ID: types.StringValue("project_123")})
	var resp tfresource.DeleteResponse

	r.Delete(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected delete diagnostic")
	}
}

func TestFrameworkDeleteRejectsMalformedStateBeforeClientCall(t *testing.T) {
	t.Parallel()

	called := false
	r := newResourceWithClient(fakeProjectClient{
		delete: func(ctx context.Context, id string) error {
			called = true
			return nil
		},
	})
	var req tfresource.DeleteRequest
	req.State.Schema = projectSchema()
	req.State.Raw = tftypes.NewValue(tftypes.String, "not project state")
	var resp tfresource.DeleteResponse

	r.Delete(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-state diagnostic")
	}
	if called {
		t.Fatal("DeleteProject was called for malformed state")
	}
}

func frameworkDeleteRequest(t *testing.T, state projectModel) tfresource.DeleteRequest {
	t.Helper()
	var req tfresource.DeleteRequest
	req.State.Schema = projectSchema()
	if diags := req.State.Set(context.Background(), state); diags.HasError() {
		t.Fatalf("set delete state: %v", diags)
	}
	return req
}
