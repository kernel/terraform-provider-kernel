package extension

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFlattenExtensionUploadMapsDurableState(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	state, diags := flattenExtensionUpload(
		extensionUploadResponseForTest(t, `{
			"id":"extension_123",
			"name":"Extension",
			"checksum":"`+checksum+`",
			"created_at":"2026-01-01T00:00:00Z",
			"size_bytes":123,
			"last_used_at":"2026-01-02T00:00:00Z"
		}`),
		extensionModel{
			Name:         types.StringValue("Extension"),
			SourceSHA256: types.StringValue(checksum),
		},
		"project_123",
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got, want := state.ID.ValueString(), "extension_123"; got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if got, want := state.Name.ValueString(), "Extension"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := state.ProjectID.ValueString(), "project_123"; got != want {
		t.Fatalf("project_id = %q, want %q", got, want)
	}
	if got, want := state.SourceSHA256.ValueString(), checksum; got != want {
		t.Fatalf("source_sha256 = %q, want %q", got, want)
	}
	if !state.SourcePath.IsNull() {
		t.Fatalf("source_path = %v, want null write-only state", state.SourcePath)
	}
}

func TestFlattenExtensionUploadMapsNullableNameAndUnscopedProject(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	state, diags := flattenExtensionUpload(
		extensionUploadResponseForTest(t, `{
			"id":"extension_123",
			"name":null,
			"checksum":"`+checksum+`",
			"created_at":"2026-01-01T00:00:00Z",
			"size_bytes":123,
			"last_used_at":null
		}`),
		extensionModel{
			Name:         types.StringNull(),
			SourceSHA256: types.StringValue(checksum),
		},
		"",
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Name.IsNull() {
		t.Fatalf("name = %v, want null", state.Name)
	}
	if !state.ProjectID.IsNull() {
		t.Fatalf("project_id = %v, want null", state.ProjectID)
	}
}

func TestFlattenExtensionUploadRejectsInvalidRequiredFields(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	tests := map[string]struct {
		body  string
		field string
	}{
		"missing id":       {body: `{"checksum":"` + checksum + `"}`, field: "id"},
		"empty id":         {body: `{"id":"","checksum":"` + checksum + `"}`, field: "id"},
		"missing checksum": {body: `{"id":"extension_123"}`, field: "checksum"},
		"null checksum":    {body: `{"id":"extension_123","checksum":null}`, field: "checksum"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenExtensionUpload(
				extensionUploadResponseForTest(t, test.body),
				extensionModel{Name: types.StringNull(), SourceSHA256: types.StringValue(checksum)},
				"project_123",
			)
			if !extensionDiagnosticContains(diags, "Invalid Kernel Extension Response", "field "+test.field) {
				t.Fatalf("diagnostics = %v, want invalid %s", diags, test.field)
			}
		})
	}
}

func TestFlattenExtensionUploadRejectsResponseDrift(t *testing.T) {
	t.Parallel()

	configuredChecksum := strings.Repeat("a", 64)
	responseChecksum := strings.Repeat("b", 64)
	tests := map[string]struct {
		body   string
		config extensionModel
		want   string
	}{
		"name": {
			body: `{"id":"extension_123","name":"Different","checksum":"` + configuredChecksum + `"}`,
			config: extensionModel{
				Name:         types.StringValue("Extension"),
				SourceSHA256: types.StringValue(configuredChecksum),
			},
			want: "Unexpected Kernel Extension Name",
		},
		"checksum": {
			body: `{"id":"extension_123","checksum":"` + responseChecksum + `"}`,
			config: extensionModel{
				Name:         types.StringNull(),
				SourceSHA256: types.StringValue(configuredChecksum),
			},
			want: "Unexpected Kernel Extension Checksum",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenExtensionUpload(extensionUploadResponseForTest(t, test.body), test.config, "project_123")
			if !extensionDiagnosticContains(diags, test.want, "") {
				t.Fatalf("diagnostics = %v, want %q", diags, test.want)
			}
		})
	}
}

func extensionUploadResponseForTest(t *testing.T, body string) kernel.ExtensionUploadResponse {
	t.Helper()

	var response kernel.ExtensionUploadResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("unmarshal extension upload response: %v", err)
	}
	return response
}

func extensionDiagnosticContains(diags diag.Diagnostics, summary, detail string) bool {
	for _, diagnostic := range diags {
		if diagnostic.Summary() == summary && (detail == "" || strings.Contains(diagnostic.Detail(), detail)) {
			return true
		}
	}
	return false
}
