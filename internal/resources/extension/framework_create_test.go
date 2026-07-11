package extension

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFrameworkCreateExtensionPersistsSuccessfulState(t *testing.T) {
	t.Parallel()

	config := extensionFrameworkConfigForTest(t)
	var gotProjectID string
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			gotProjectID = projectID
			response := extensionUploadResponseForTest(t, `{"id":"extension_123","checksum":"`+config.SourceSHA256.ValueString()+`"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionCreateRequest(t, config)

	createExtensionResource(context.Background(), client, "project_default", req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := gotProjectID, "project_default"; got != want {
		t.Fatalf("project id = %q, want %q", got, want)
	}
	state := frameworkExtensionCreateState(t, resp)
	if got, want := state.ID.ValueString(), "extension_123"; got != want {
		t.Fatalf("state id = %q, want %q", got, want)
	}
	if !state.ProjectID.Equal(types.StringValue("project_default")) {
		t.Fatalf("project_id = %v, want project_default", state.ProjectID)
	}
	if !state.SourcePath.IsNull() {
		t.Fatalf("source_path = %v, want null write-only state", state.SourcePath)
	}
}

func TestFrameworkCreateExtensionExplicitProjectOverridesDefault(t *testing.T) {
	t.Parallel()

	config := extensionFrameworkConfigForTest(t)
	config.ProjectID = types.StringValue("project_explicit")
	var gotProjectID string
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			gotProjectID = projectID
			response := extensionUploadResponseForTest(t, `{"id":"extension_123","checksum":"`+config.SourceSHA256.ValueString()+`"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionCreateRequest(t, config)

	createExtensionResource(context.Background(), client, "project_default", req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := gotProjectID, "project_explicit"; got != want {
		t.Fatalf("project id = %q, want %q", got, want)
	}
}

func TestFrameworkCreateExtensionUsesKnownPlannedProject(t *testing.T) {
	t.Parallel()

	config := extensionFrameworkConfigForTest(t)
	config.ProjectID = types.StringUnknown()
	var gotProjectID string
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			gotProjectID = projectID
			response := extensionUploadResponseForTest(t, `{"id":"extension_123","checksum":"`+config.SourceSHA256.ValueString()+`"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionCreateRequest(t, config)
	plan := config
	plan.ProjectID = types.StringValue("project_planned")
	setFrameworkExtensionCreatePlan(t, &req, plan)

	createExtensionResource(context.Background(), client, "project_default", req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if got, want := gotProjectID, "project_planned"; got != want {
		t.Fatalf("project id = %q, want %q", got, want)
	}
}

func TestFrameworkCreateExtensionUsesKnownPlannedUploadMetadata(t *testing.T) {
	t.Parallel()

	config := extensionFrameworkConfigForTest(t)
	config.Name = types.StringUnknown()
	config.SourceSHA256 = types.StringUnknown()
	plan := config
	plan.Name = types.StringValue("Extension")
	plan.SourceSHA256 = types.StringValue("9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc")
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			if !params.Name.Valid() || params.Name.Value != "Extension" {
				t.Fatalf("name = %#v, want planned Extension", params.Name)
			}
			response := extensionUploadResponseForTest(t, `{"id":"extension_123","name":"Extension","checksum":"`+plan.SourceSHA256.ValueString()+`"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionCreateRequest(t, config)
	setFrameworkExtensionCreatePlan(t, &req, plan)

	createExtensionResource(context.Background(), client, "", req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	state := frameworkExtensionCreateState(t, resp)
	if !state.Name.Equal(plan.Name) {
		t.Fatalf("name = %v, want %v", state.Name, plan.Name)
	}
	if !state.SourceSHA256.Equal(plan.SourceSHA256) {
		t.Fatalf("source_sha256 = %v, want %v", state.SourceSHA256, plan.SourceSHA256)
	}
}

func TestFrameworkCreateExtensionRejectsUnknownProjectBeforeUpload(t *testing.T) {
	t.Parallel()

	called := false
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			called = true
			return nil, nil
		},
	}
	config := extensionFrameworkConfigForTest(t)
	config.ProjectID = types.StringUnknown()
	req, resp := frameworkExtensionCreateRequest(t, config)

	createExtensionResource(context.Background(), client, "project_default", req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unknown project diagnostic")
	}
	if called {
		t.Fatal("UploadExtension was called with unknown project scope")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want absent state", resp.State.Raw)
	}
}

func TestFrameworkCreateExtensionPersistsRecoverableIdentityBeforeDiagnostic(t *testing.T) {
	t.Parallel()

	config := extensionFrameworkConfigForTest(t)
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			response := extensionUploadResponseForTest(t, `{"id":"extension_123"}`)
			return &response, nil
		},
	}
	req, resp := frameworkExtensionCreateRequest(t, config)

	createExtensionResource(context.Background(), client, "project_default", req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected uncertain-create diagnostic")
	}
	state := frameworkExtensionCreateState(t, resp)
	if got, want := state.ID.ValueString(), "extension_123"; got != want {
		t.Fatalf("state id = %q, want recoverable %q", got, want)
	}
	if !state.SourceSHA256.Equal(config.SourceSHA256) {
		t.Fatalf("source_sha256 = %v, want %v", state.SourceSHA256, config.SourceSHA256)
	}
}

func TestFrameworkCreateExtensionLeavesNoStateWithoutRecoverableIdentity(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T) (*kernel.ExtensionUploadResponse, error){
		"definite rejection": func(t *testing.T) (*kernel.ExtensionUploadResponse, error) {
			return nil, extensionAPIErrorForTest(t, http.StatusConflict, `{}`)
		},
		"uncertain transport failure": func(t *testing.T) (*kernel.ExtensionUploadResponse, error) {
			return nil, errors.New("response timeout")
		},
	}

	for name, upload := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := fakeExtensionUploader{
				upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
					return upload(t)
				},
			}
			req, resp := frameworkExtensionCreateRequest(t, extensionFrameworkConfigForTest(t))

			createExtensionResource(context.Background(), client, "", req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected create diagnostic")
			}
			if !resp.State.Raw.IsNull() {
				t.Fatalf("state = %v, want absent state", resp.State.Raw)
			}
		})
	}
}

func TestFrameworkCreateExtensionRejectsMalformedConfigBeforeUpload(t *testing.T) {
	t.Parallel()

	called := false
	client := fakeExtensionUploader{
		upload: func(ctx context.Context, projectID string, params kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error) {
			called = true
			return nil, nil
		},
	}
	var req resource.CreateRequest
	req.Config.Schema = extensionSchema()
	req.Config.Raw = tftypes.NewValue(tftypes.String, "not an extension config")
	resp := frameworkExtensionCreateResponse()

	createExtensionResource(context.Background(), client, "", req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected malformed-config diagnostic")
	}
	if called {
		t.Fatal("UploadExtension was called for malformed configuration")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want absent state", resp.State.Raw)
	}
}

func TestFrameworkCreateExtensionRejectsMissingClient(t *testing.T) {
	t.Parallel()

	req, resp := frameworkExtensionCreateRequest(t, extensionFrameworkConfigForTest(t))
	createExtensionResource(context.Background(), nil, "", req, resp)

	if !extensionDiagnosticContains(resp.Diagnostics, "Missing Kernel Client", "provider was not configured") {
		t.Fatalf("diagnostics = %v, want missing client error", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatalf("state = %v, want absent state", resp.State.Raw)
	}
}

func frameworkExtensionCreateRequest(t *testing.T, config extensionModel) (resource.CreateRequest, *resource.CreateResponse) {
	t.Helper()
	ctx := context.Background()
	schema := extensionSchema()

	var encoded tfsdk.Plan
	encoded.Schema = schema
	if diags := encoded.Set(ctx, config); diags.HasError() {
		t.Fatalf("encode extension config: %v", diags)
	}

	var req resource.CreateRequest
	req.Config.Schema = schema
	req.Config.Raw = encoded.Raw
	setFrameworkExtensionCreatePlan(t, &req, config)
	return req, frameworkExtensionCreateResponse()
}

func setFrameworkExtensionCreatePlan(t *testing.T, req *resource.CreateRequest, plan extensionModel) {
	t.Helper()
	plan.SourcePath = types.StringNull()
	req.Plan.Schema = extensionSchema()
	if diags := req.Plan.Set(context.Background(), plan); diags.HasError() {
		t.Fatalf("set extension plan: %v", diags)
	}
}

func frameworkExtensionCreateResponse() *resource.CreateResponse {
	resp := &resource.CreateResponse{}
	resp.State.Schema = extensionSchema()
	resp.State.RemoveResource(context.Background())
	return resp
}

func frameworkExtensionCreateState(t *testing.T, resp *resource.CreateResponse) extensionModel {
	t.Helper()
	var state extensionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get extension state: %v", diags)
	}
	return state
}

func extensionFrameworkConfigForTest(t *testing.T) extensionModel {
	t.Helper()
	return extensionModel{
		ID:           types.StringUnknown(),
		Name:         types.StringNull(),
		ProjectID:    types.StringNull(),
		SourcePath:   types.StringValue(writeExtensionArchiveForTest(t, []byte("extension archive"))),
		SourceSHA256: types.StringValue("9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"),
	}
}
