package extension

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestExtensionUploadFailureIsDefinite(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want bool
	}{
		"bad request":         {err: &kernel.Error{StatusCode: http.StatusBadRequest}, want: true},
		"conflict":            {err: &kernel.Error{StatusCode: http.StatusConflict}, want: true},
		"unprocessable":       {err: &kernel.Error{StatusCode: http.StatusUnprocessableEntity}, want: true},
		"server error":        {err: &kernel.Error{StatusCode: http.StatusInternalServerError}, want: false},
		"service unavailable": {err: &kernel.Error{StatusCode: http.StatusServiceUnavailable}, want: false},
		"transport error":     {err: errors.New("response timeout"), want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := extensionUploadFailureIsDefinite(test.err); got != test.want {
				t.Fatalf("extensionUploadFailureIsDefinite() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestPartialExtensionStatePreservesVerifiedIdentityAndConfiguration(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	config := extensionModel{
		Name:         types.StringValue("Extension"),
		SourcePath:   types.StringValue("extension.zip"),
		SourceSHA256: types.StringValue(checksum),
	}
	state := partialExtensionState(
		extensionUploadResponseForTest(t, `{"id":"extension_123"}`),
		config,
		"project_123",
	)

	if got, want := state.ID.ValueString(), "extension_123"; got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if !state.Name.Equal(config.Name) {
		t.Fatalf("name = %v, want %v", state.Name, config.Name)
	}
	if got, want := state.ProjectID.ValueString(), "project_123"; got != want {
		t.Fatalf("project_id = %q, want %q", got, want)
	}
	if !state.SourcePath.IsNull() {
		t.Fatalf("source_path = %v, want null write-only state", state.SourcePath)
	}
	if !state.SourceSHA256.Equal(config.SourceSHA256) {
		t.Fatalf("source_sha256 = %v, want %v", state.SourceSHA256, config.SourceSHA256)
	}
}

func TestPartialExtensionStateRejectsUnverifiedIdentity(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"missing id": `{"checksum":"abc"}`,
		"empty id":   `{"id":""}`,
		"null id":    `{"id":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := partialExtensionState(extensionUploadResponseForTest(t, body), extensionModel{}, "project_123")
			if !state.ID.IsNull() {
				t.Fatalf("id = %v, want zero state", state.ID)
			}
		})
	}
}

func TestPartialExtensionStateLeavesAPIKeyBoundScopeUnset(t *testing.T) {
	t.Parallel()

	state := partialExtensionState(
		extensionUploadResponseForTest(t, `{"id":"extension_123"}`),
		extensionModel{},
		"",
	)
	if !state.ProjectID.IsNull() {
		t.Fatalf("project_id = %v, want null", state.ProjectID)
	}
}

func TestUncertainExtensionCreateDiagnosticIncludesRecoveryContext(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	tests := map[string]struct {
		projectID string
		scope     string
	}{
		"explicit project": {projectID: "project_123", scope: `project "project_123"`},
		"api key scope":    {scope: "the API-key-bound project"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var diags diag.Diagnostics
			addUncertainExtensionCreateDiagnostic(&diags, test.projectID, checksum, "response timeout")
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %v, want one error", diags)
			}
			if got, want := diags[0].Summary(), "Kernel Extension Upload Outcome Uncertain"; got != want {
				t.Fatalf("summary = %q, want %q", got, want)
			}
			detail := diags[0].Detail()
			for _, want := range []string{checksum, test.scope, "import its canonical extension ID or delete it", "If no match exists, retry the apply", "response timeout"} {
				if !strings.Contains(detail, want) {
					t.Fatalf("detail = %q, want it to contain %q", detail, want)
				}
			}
		})
	}
}
