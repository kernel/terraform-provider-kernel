package extension

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFrameworkReadExtensionSetsRefreshedState(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	prior := extensionModel{
		ID:           types.StringValue("extension_123"),
		Name:         types.StringValue("Old"),
		ProjectID:    types.StringValue("project_123"),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringValue(checksum),
	}
	client := fakeExtensionReader{
		get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
			response := extensionGetResponseForTest(t, `{"id":"extension_123","name":"Current","checksum":"`+checksum+`"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionReadRequest(t, prior)

	readExtensionResource(context.Background(), client, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	state := frameworkExtensionReadState(t, resp)
	if got, want := state.Name.ValueString(), "Current"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if !state.ProjectID.Equal(prior.ProjectID) {
		t.Fatalf("project_id = %v, want %v", state.ProjectID, prior.ProjectID)
	}
}

func TestFrameworkReadExtensionRemovesCodedNotFound(t *testing.T) {
	t.Parallel()

	client := fakeExtensionReader{
		get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
			return nil, extensionAPIErrorForTest(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	}
	req, resp := frameworkExtensionReadRequest(t, extensionReadStateForTest())

	readExtensionResource(context.Background(), client, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want removed resource", resp.State.Raw)
	}
}

func TestFrameworkReadExtensionPreservesStateOnDiagnostic(t *testing.T) {
	t.Parallel()

	client := fakeExtensionReader{
		get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
			return nil, nil
		},
	}
	req, resp := frameworkExtensionReadRequest(t, extensionReadStateForTest())
	before := resp.State.Raw.Copy()

	readExtensionResource(context.Background(), client, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read diagnostic")
	}
	if !resp.State.Raw.Equal(before) {
		t.Fatalf("state = %v, want preserved %v", resp.State.Raw, before)
	}
}

func TestFrameworkReadExtensionRejectsMalformedStateBeforeGet(t *testing.T) {
	t.Parallel()

	called := false
	client := fakeExtensionReader{
		get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
			called = true
			return nil, nil
		},
	}
	var req resource.ReadRequest
	req.State.Schema = extensionSchema()
	req.State.Raw = tftypes.NewValue(tftypes.String, "not extension state")
	resp := &resource.ReadResponse{}
	resp.State.Schema = extensionSchema()
	resp.State.Raw = req.State.Raw.Copy()

	readExtensionResource(context.Background(), client, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-state diagnostic")
	}
	if called {
		t.Fatal("GetExtension called for malformed state")
	}
	if !resp.State.Raw.Equal(req.State.Raw) {
		t.Fatalf("state = %v, want preserved malformed state %v", resp.State.Raw, req.State.Raw)
	}
}

func frameworkExtensionReadRequest(t *testing.T, state extensionModel) (resource.ReadRequest, *resource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	schema := extensionSchema()

	var req resource.ReadRequest
	req.State.Schema = schema
	if diags := req.State.Set(ctx, state); diags.HasError() {
		t.Fatalf("set extension read state: %v", diags)
	}

	resp := &resource.ReadResponse{}
	resp.State.Schema = schema
	resp.State.Raw = req.State.Raw.Copy()
	return req, resp
}

func frameworkExtensionReadState(t *testing.T, resp *resource.ReadResponse) extensionModel {
	t.Helper()
	var state extensionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get extension read state: %v", diags)
	}
	return state
}

func extensionReadStateForTest() extensionModel {
	return extensionModel{
		ID:           types.StringValue("extension_123"),
		Name:         types.StringNull(),
		ProjectID:    types.StringNull(),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringUnknown(),
	}
}
