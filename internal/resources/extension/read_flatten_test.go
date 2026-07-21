package extension

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFlattenExtensionReadMapsDurableState(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	state, diags := flattenExtensionRead(
		extensionGetResponseForTest(t, `{
			"id":"extension_123",
			"name":"Extension",
			"checksum":"`+checksum+`",
			"created_at":"2026-01-01T00:00:00Z",
			"size_bytes":123,
			"last_used_at":"2026-01-02T00:00:00Z"
		}`),
		extensionModel{
			ID:           types.StringValue("extension_123"),
			ProjectID:    types.StringValue("project_123"),
			SourceSHA256: types.StringValue(checksum),
		},
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
	if !state.ProjectID.Equal(types.StringValue("project_123")) {
		t.Fatalf("project_id = %v, want project_123", state.ProjectID)
	}
	if !state.SourcePath.IsNull() {
		t.Fatalf("source_path = %v, want null write-only state", state.SourcePath)
	}
	if got, want := state.SourceSHA256.ValueString(), checksum; got != want {
		t.Fatalf("source_sha256 = %q, want %q", got, want)
	}
}

func TestFlattenExtensionReadAllowsLegacyNullableMetadata(t *testing.T) {
	t.Parallel()

	state, diags := flattenExtensionRead(
		extensionGetResponseForTest(t, `{"id":"extension_123","name":null}`),
		extensionModel{
			ID:           types.StringValue("extension_123"),
			ProjectID:    types.StringNull(),
			SourceSHA256: types.StringUnknown(),
		},
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
	if !state.SourceSHA256.IsNull() {
		t.Fatalf("source_sha256 = %v, want null", state.SourceSHA256)
	}
}

func TestFlattenExtensionReadReportsLostManagedChecksum(t *testing.T) {
	t.Parallel()

	_, diags := flattenExtensionRead(
		extensionGetResponseForTest(t, `{"id":"extension_123","checksum":null}`),
		extensionModel{
			ID:           types.StringValue("extension_123"),
			SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
		},
	)
	if !extensionDiagnosticContains(diags, "Missing Kernel Extension Checksum", "cannot verify the managed archive content") {
		t.Fatalf("diagnostics = %v, want missing managed checksum", diags)
	}
}

func TestFlattenExtensionReadSurfacesRemoteChecksumDrift(t *testing.T) {
	t.Parallel()

	remoteChecksum := strings.Repeat("b", 64)
	state, diags := flattenExtensionRead(
		extensionGetResponseForTest(t, `{"id":"extension_123","checksum":"`+remoteChecksum+`"}`),
		extensionModel{
			ID:           types.StringValue("extension_123"),
			SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
		},
	)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := state.SourceSHA256.ValueString(); got != remoteChecksum {
		t.Fatalf("source_sha256 = %q, want remote checksum %q", got, remoteChecksum)
	}
}

func TestFlattenExtensionReadRejectsInvalidResponse(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	tests := map[string]struct {
		body    string
		priorID string
		summary string
		detail  string
	}{
		"missing id": {
			body:    `{"checksum":"` + checksum + `"}`,
			priorID: "extension_123",
			summary: "Invalid Kernel Extension Response",
			detail:  "field id",
		},
		"unexpected id": {
			body:    `{"id":"extension_other","checksum":"` + checksum + `"}`,
			priorID: "extension_123",
			summary: "Unexpected Kernel Extension ID",
			detail:  "extension_other",
		},
		"invalid name": {
			body:    `{"id":"extension_123","name":123,"checksum":"` + checksum + `"}`,
			priorID: "extension_123",
			summary: "Invalid Kernel Extension Response",
			detail:  "field name",
		},
		"invalid checksum": {
			body:    `{"id":"extension_123","checksum":"bad"}`,
			priorID: "extension_123",
			summary: "Invalid Kernel Extension Response",
			detail:  "field checksum",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenExtensionRead(
				extensionGetResponseForTest(t, test.body),
				extensionModel{ID: types.StringValue(test.priorID), SourceSHA256: types.StringNull()},
			)
			if !extensionDiagnosticContains(diags, test.summary, test.detail) {
				t.Fatalf("diagnostics = %v, want %q containing %q", diags, test.summary, test.detail)
			}
		})
	}
}

func extensionGetResponseForTest(t *testing.T, body string) kernel.ExtensionGetResponse {
	t.Helper()

	var response kernel.ExtensionGetResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("unmarshal extension response: %v", err)
	}
	return response
}
