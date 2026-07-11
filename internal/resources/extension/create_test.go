package extension

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionUploader = kernelclient.Clients{}

type fakeExtensionUploader struct {
	upload func(context.Context, string, kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error)
}

func (f fakeExtensionUploader) UploadExtension(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
	if f.upload == nil {
		return nil, errors.New("unexpected upload")
	}
	return f.upload(ctx, projectID, params)
}

func TestCreateExtensionUploadsAndFlattensDurableState(t *testing.T) {
	t.Parallel()

	data := []byte("extension archive")
	checksum := "9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"
	config := extensionModel{
		Name:         types.StringValue("Extension"),
		SourcePath:   types.StringValue(writeExtensionArchiveForTest(t, data)),
		SourceSHA256: types.StringValue(checksum),
	}
	var gotProjectID string
	result, diags := createExtension(context.Background(), fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			gotProjectID = projectID
			if !params.Name.Valid() || params.Name.Value != "Extension" {
				t.Fatalf("name = %#v, want Extension", params.Name)
			}
			upload, err := io.ReadAll(params.File)
			if err != nil {
				t.Fatalf("read upload: %v", err)
			}
			if got, want := string(upload), string(data); got != want {
				t.Fatalf("upload data = %q, want %q", got, want)
			}
			response := extensionUploadResponseForTest(t, `{"id":"extension_123","name":"Extension","checksum":"`+checksum+`"}`)
			return &response, nil
		},
	}, config, "project_123")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result.Status != extensionCreateSucceeded {
		t.Fatalf("status = %v, want succeeded", result.Status)
	}
	if got, want := gotProjectID, "project_123"; got != want {
		t.Fatalf("project id = %q, want %q", got, want)
	}
	if got, want := result.State.ID.ValueString(), "extension_123"; got != want {
		t.Fatalf("state id = %q, want %q", got, want)
	}
	if !result.State.SourcePath.IsNull() {
		t.Fatalf("source_path = %v, want null write-only state", result.State.SourcePath)
	}
}

func TestCreateExtensionStopsBeforeUploadOnPreflightFailure(t *testing.T) {
	t.Parallel()

	called := false
	result, diags := createExtension(context.Background(), fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			called = true
			return nil, nil
		},
	}, extensionModel{
		SourcePath:   types.StringUnknown(),
		SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
	}, "project_123")
	if !diags.HasError() {
		t.Fatal("expected preflight diagnostic")
	}
	if result.Status != extensionCreateFailed {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	if called {
		t.Fatal("upload called after preflight failure")
	}
}

func TestCreateExtensionClassifiesDefiniteAPIRejection(t *testing.T) {
	t.Parallel()

	config := extensionConfigForCreateTest(t)
	result, diags := createExtension(context.Background(), fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			return nil, extensionAPIErrorForTest(t, http.StatusConflict, `{}`)
		},
	}, config, "")
	if result.Status != extensionCreateFailed {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	if !extensionDiagnosticContains(diags, "Create Kernel Extension", "409") {
		t.Fatalf("diagnostics = %v, want definite create failure", diags)
	}
}

func TestCreateExtensionClassifiesUncertainOutcomes(t *testing.T) {
	t.Parallel()

	checksum := "9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"
	tests := map[string]struct {
		response *kernel.ExtensionUploadResponse
		err      error
		reason   string
		wantID   string
	}{
		"transport failure": {
			err:    errors.New("response timeout"),
			reason: "response timeout",
		},
		"server failure": {
			err:    extensionAPIErrorForTest(t, http.StatusInternalServerError, `{}`),
			reason: "500",
		},
		"empty success response": {
			reason: "empty extension upload response",
		},
		"malformed success preserves valid id": {
			response: extensionUploadResponsePointerForTest(t, `{"id":"extension_123"}`),
			reason:   "missing or invalid field checksum",
			wantID:   "extension_123",
		},
		"malformed success rejects invalid id": {
			response: extensionUploadResponsePointerForTest(t, `{"checksum":"`+checksum+`"}`),
			reason:   "missing or invalid field id",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, diags := createExtension(context.Background(), fakeExtensionUploader{
				upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
					return test.response, test.err
				},
			}, extensionConfigForCreateTest(t), "project_123")
			if result.Status != extensionCreateUncertain {
				t.Fatalf("status = %v, want uncertain", result.Status)
			}
			if !extensionDiagnosticContains(diags, "Kernel Extension Upload Outcome Uncertain", test.reason) {
				t.Fatalf("diagnostics = %v, want uncertain reason %q", diags, test.reason)
			}
			if got := result.State.ID.ValueString(); got != test.wantID {
				t.Fatalf("state id = %q, want %q", got, test.wantID)
			}
		})
	}
}

func extensionConfigForCreateTest(t *testing.T) extensionModel {
	t.Helper()
	return extensionModel{
		Name:         types.StringNull(),
		SourcePath:   types.StringValue(writeExtensionArchiveForTest(t, []byte("extension archive"))),
		SourceSHA256: types.StringValue("9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"),
	}
}

func extensionUploadResponsePointerForTest(t *testing.T, body string) *kernel.ExtensionUploadResponse {
	t.Helper()
	response := extensionUploadResponseForTest(t, body)
	return &response
}

func extensionAPIErrorForTest(t *testing.T, status int, body string) *kernel.Error {
	t.Helper()

	var apiError kernel.Error
	if err := json.Unmarshal([]byte(body), &apiError); err != nil {
		t.Fatalf("unmarshal API error: %v", err)
	}
	apiError.StatusCode = status
	apiError.Request = &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Scheme: "https", Host: "api.example", Path: "/extensions"},
	}
	apiError.Response = &http.Response{StatusCode: status, Status: http.StatusText(status)}
	return &apiError
}
