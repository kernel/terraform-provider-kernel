package extension

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionDeleter = kernelclient.Clients{}

type fakeExtensionDeleter struct {
	delete func(context.Context, string, string) error
}

func (f fakeExtensionDeleter) DeleteExtension(ctx context.Context, projectID, id string) error {
	if f.delete == nil {
		return errors.New("unexpected delete")
	}
	return f.delete(ctx, projectID, id)
}

func TestDeleteExtensionUsesStateIdentityAndScope(t *testing.T) {
	t.Parallel()

	var gotProjectID, gotID string
	diags := deleteExtension(context.Background(), fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			gotProjectID, gotID = projectID, id
			return nil
		},
	}, extensionModel{ID: types.StringValue("extension_123"), ProjectID: types.StringValue("project_123")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotProjectID != "project_123" || gotID != "extension_123" {
		t.Fatalf("DeleteExtension scope/id = %q/%q, want project_123/extension_123", gotProjectID, gotID)
	}
}

func TestDeleteExtensionTreatsCodedNotFoundAsSuccess(t *testing.T) {
	t.Parallel()

	diags := deleteExtension(context.Background(), fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			return extensionAPIErrorForTest(t, http.StatusNotFound, `{"code":"not_found"}`)
		},
	}, extensionModel{ID: types.StringValue("extension_123"), ProjectID: types.StringNull()})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestDeleteExtensionExplainsDurablePoolDependency(t *testing.T) {
	t.Parallel()

	diags := deleteExtension(context.Background(), fakeExtensionDeleter{
		delete: func(ctx context.Context, projectID, id string) error {
			return extensionAPIErrorForTest(t, http.StatusBadRequest, `{"code":"resource_in_use"}`)
		},
	}, extensionModel{ID: types.StringValue("extension_123"), ProjectID: types.StringNull()})
	if !extensionDiagnosticContains(diags, "Delete Kernel Extension", "Remove the extension from those durable browser pool configurations") {
		t.Fatalf("diagnostics = %v, want browser pool dependency guidance", diags)
	}
}

func TestDeleteExtensionDoesNotMisclassifyGenericErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]error{
		"generic bad request": extensionAPIErrorForTest(t, http.StatusBadRequest, `{}`),
		"generic not found":   extensionAPIErrorForTest(t, http.StatusNotFound, `{}`),
		"transport failure":   errors.New("connection reset"),
	}
	for name, err := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			diags := deleteExtension(context.Background(), fakeExtensionDeleter{
				delete: func(ctx context.Context, projectID, id string) error { return err },
			}, extensionModel{ID: types.StringValue("extension_123"), ProjectID: types.StringNull()})
			if !extensionDiagnosticContains(diags, "Delete Kernel Extension", "extension_123") {
				t.Fatalf("diagnostics = %v, want generic delete error with ID", diags)
			}
		})
	}
}

func TestDeleteExtensionRejectsInvalidStateBeforeCall(t *testing.T) {
	t.Parallel()

	for name, state := range map[string]extensionModel{
		"missing id":      {ID: types.StringNull(), ProjectID: types.StringNull()},
		"unknown project": {ID: types.StringValue("extension_123"), ProjectID: types.StringUnknown()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			called := false
			diags := deleteExtension(context.Background(), fakeExtensionDeleter{
				delete: func(ctx context.Context, projectID, id string) error { called = true; return nil },
			}, state)
			if !diags.HasError() {
				t.Fatal("expected invalid state diagnostic")
			}
			if called {
				t.Fatal("DeleteExtension called for invalid state")
			}
		})
	}
}
