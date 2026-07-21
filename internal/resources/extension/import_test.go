package extension

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionImporter = kernelclient.Clients{}

type fakeExtensionImporter struct {
	defaultProjectID string
}

func (f fakeExtensionImporter) DefaultProjectID() string {
	return f.defaultProjectID
}

func TestParseExtensionImportID(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input       string
		projectID   string
		extensionID string
		ok          bool
	}{
		"bare id":         {input: "extension_123", extensionID: "extension_123", ok: true},
		"qualified id":    {input: "project_123/extension_123", projectID: "project_123", extensionID: "extension_123", ok: true},
		"empty":           {input: ""},
		"empty project":   {input: "/extension_123"},
		"empty extension": {input: "project_123/"},
		"extra separator": {input: "project_123/extension_123/extra"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			projectID, extensionID, ok := parseExtensionImportID(test.input)
			if projectID != test.projectID || extensionID != test.extensionID || ok != test.ok {
				t.Fatalf("parseExtensionImportID(%q) = %q, %q, %t; want %q, %q, %t", test.input, projectID, extensionID, ok, test.projectID, test.extensionID, test.ok)
			}
		})
	}
}

func TestFrameworkImportExtensionSetsRecoverableState(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		importID         string
		defaultProjectID string
		wantProjectID    types.String
	}{
		"bare id inherits provider default": {
			importID:         "extension_123",
			defaultProjectID: "project_default",
			wantProjectID:    types.StringValue("project_default"),
		},
		"bare id remains api key scoped": {
			importID:      "extension_123",
			wantProjectID: types.StringNull(),
		},
		"qualified id overrides provider default": {
			importID:         "project_explicit/extension_123",
			defaultProjectID: "project_default",
			wantProjectID:    types.StringValue("project_explicit"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := frameworkExtensionImportResponse()
			importExtensionResource(context.Background(), fakeExtensionImporter{defaultProjectID: test.defaultProjectID}, resource.ImportStateRequest{ID: test.importID}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			state := frameworkExtensionImportState(t, resp)
			if got, want := state.ID.ValueString(), "extension_123"; got != want {
				t.Fatalf("id = %q, want %q", got, want)
			}
			if !state.ProjectID.Equal(test.wantProjectID) {
				t.Fatalf("project_id = %v, want %v", state.ProjectID, test.wantProjectID)
			}
			if !state.Name.IsUnknown() || !state.SourceSHA256.IsUnknown() {
				t.Fatalf("name/source_sha256 = %v/%v, want unknown until Read", state.Name, state.SourceSHA256)
			}
			if !state.SourcePath.IsNull() {
				t.Fatalf("source_path = %v, want null write-only state", state.SourcePath)
			}
		})
	}
}

func TestFrameworkImportExtensionRejectsInvalidIDAndMissingClient(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		client   extensionImporter
		importID string
		summary  string
	}{
		"missing client": {importID: "extension_123", summary: "Missing Kernel Client"},
		"invalid id":     {client: fakeExtensionImporter{}, importID: "project/extension/extra", summary: "Invalid Kernel Extension Import ID"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := frameworkExtensionImportResponse()
			importExtensionResource(context.Background(), test.client, resource.ImportStateRequest{ID: test.importID}, resp)
			if !extensionDiagnosticContains(resp.Diagnostics, test.summary, "") {
				t.Fatalf("diagnostics = %v, want %q", resp.Diagnostics, test.summary)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatalf("state = %v, want absent state", resp.State.Raw)
			}
		})
	}
}

func frameworkExtensionImportResponse() *resource.ImportStateResponse {
	resp := &resource.ImportStateResponse{}
	resp.State.Schema = extensionSchema()
	resp.State.RemoveResource(context.Background())
	return resp
}

func frameworkExtensionImportState(t *testing.T, resp *resource.ImportStateResponse) extensionModel {
	t.Helper()
	var state extensionModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("get extension import state: %v", diags)
	}
	return state
}
