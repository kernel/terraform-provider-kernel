package extension

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestFrameworkDeleteExtensionDelegatesStateIdentityAndScope(t *testing.T) {
	t.Parallel()

	var gotProjectID, gotID string
	client := fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			gotProjectID, gotID = projectID, id
			return nil
		},
	}
	req := frameworkExtensionDeleteRequest(t, extensionModel{
		ID:           types.StringValue("extension_123"),
		Name:         types.StringNull(),
		ProjectID:    types.StringValue("project_123"),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringUnknown(),
	})
	var resp resource.DeleteResponse

	deleteExtensionResource(context.Background(), client, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if gotProjectID != "project_123" || gotID != "extension_123" {
		t.Fatalf("DeleteExtension scope/id = %q/%q, want project_123/extension_123", gotProjectID, gotID)
	}
}

func TestFrameworkDeleteExtensionReturnsDependencyDiagnostic(t *testing.T) {
	t.Parallel()

	client := fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			return extensionAPIErrorForTest(t, http.StatusBadRequest, `{"code":"resource_in_use"}`)
		},
	}
	req := frameworkExtensionDeleteRequest(t, extensionDeleteStateForTest())
	var resp resource.DeleteResponse

	deleteExtensionResource(context.Background(), client, req, &resp)

	if !extensionDiagnosticContains(resp.Diagnostics, "Delete Kernel Extension", "browser pool configurations") {
		t.Fatalf("diagnostics = %v, want durable dependency guidance", resp.Diagnostics)
	}
}

func TestFrameworkDeleteExtensionRejectsMalformedStateBeforeCall(t *testing.T) {
	t.Parallel()

	called := false
	client := fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			called = true
			return nil
		},
	}
	var req resource.DeleteRequest
	req.State.Schema = extensionSchema()
	req.State.Raw = tftypes.NewValue(tftypes.String, "not extension state")
	var resp resource.DeleteResponse

	deleteExtensionResource(context.Background(), client, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-state diagnostic")
	}
	if called {
		t.Fatal("DeleteExtension called for malformed state")
	}
}

func frameworkExtensionDeleteRequest(t *testing.T, state extensionModel) resource.DeleteRequest {
	t.Helper()

	var req resource.DeleteRequest
	req.State.Schema = extensionSchema()
	if diags := req.State.Set(context.Background(), state); diags.HasError() {
		t.Fatalf("set extension delete state: %v", diags)
	}
	return req
}

func extensionDeleteStateForTest() extensionModel {
	return extensionModel{
		ID:           types.StringValue("extension_123"),
		Name:         types.StringNull(),
		ProjectID:    types.StringNull(),
		SourcePath:   types.StringNull(),
		SourceSHA256: types.StringUnknown(),
	}
}
