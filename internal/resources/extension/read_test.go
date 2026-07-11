package extension

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionReader = kernelclient.Clients{}

type fakeExtensionReader struct {
	get func(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

func (f fakeExtensionReader) GetExtension(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
	if f.get == nil {
		return nil, errors.New("unexpected get")
	}
	return f.get(ctx, projectID, id)
}

func TestReadExtensionGetsAndFlattensDurableState(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	prior := extensionModel{
		ID:           types.StringValue("extension_123"),
		ProjectID:    types.StringValue("project_123"),
		SourceSHA256: types.StringValue(checksum),
	}
	var gotProjectID, gotID string
	next, removed, diags := readExtension(context.Background(), fakeExtensionReader{
		get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
			gotProjectID, gotID = projectID, id
			response := extensionGetResponseForTest(t, `{"id":"extension_123","name":"Extension","checksum":"`+checksum+`"}`)
			return &response, nil
		},
	}, prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if removed {
		t.Fatal("removed = true, want false")
	}
	if gotProjectID != "project_123" || gotID != "extension_123" {
		t.Fatalf("GetExtension scope/id = %q/%q, want project_123/extension_123", gotProjectID, gotID)
	}
	if got, want := next.Name.ValueString(), "Extension"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if !next.ProjectID.Equal(prior.ProjectID) {
		t.Fatalf("project_id = %v, want %v", next.ProjectID, prior.ProjectID)
	}
}

func TestReadExtensionRemovesOnlyCodedNotFound(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		projectID   types.String
		err         error
		wantRemoved bool
		wantSummary string
		wantDetail  string
	}{
		"extension not found": {
			err:         extensionAPIErrorForTest(t, http.StatusNotFound, `{"code":"not_found"}`),
			wantRemoved: true,
		},
		"project not found": {
			projectID:   types.StringValue("project_123"),
			err:         extensionAPIErrorForTest(t, http.StatusNotFound, `{"code":"project_not_found"}`),
			wantSummary: "Kernel Project Not Found",
			wantDetail:  `extension_123`,
		},
		"uncoded not found": {
			err:         extensionAPIErrorForTest(t, http.StatusNotFound, `{}`),
			wantSummary: "Read Kernel Extension",
			wantDetail:  "the API-key-bound project",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, removed, diags := readExtension(context.Background(), fakeExtensionReader{
				get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
					return nil, test.err
				},
			}, extensionModel{ID: types.StringValue("extension_123"), ProjectID: test.projectID})
			if removed != test.wantRemoved {
				t.Fatalf("removed = %t, want %t", removed, test.wantRemoved)
			}
			if test.wantSummary == "" {
				if diags.HasError() {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}
			} else if !extensionDiagnosticContains(diags, test.wantSummary, test.wantDetail) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.wantSummary)
			}
		})
	}
}

func TestReadExtensionRejectsInvalidStateBeforeGet(t *testing.T) {
	t.Parallel()

	tests := map[string]extensionModel{
		"missing id":      {ID: types.StringNull(), ProjectID: types.StringNull()},
		"unknown id":      {ID: types.StringUnknown(), ProjectID: types.StringNull()},
		"empty id":        {ID: types.StringValue(""), ProjectID: types.StringNull()},
		"unknown project": {ID: types.StringValue("extension_123"), ProjectID: types.StringUnknown()},
	}

	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			called := false
			_, removed, diags := readExtension(context.Background(), fakeExtensionReader{
				get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
					called = true
					return nil, nil
				},
			}, state)
			if !diags.HasError() {
				t.Fatal("expected invalid-state diagnostic")
			}
			if removed {
				t.Fatal("removed = true, want false")
			}
			if called {
				t.Fatal("GetExtension called for invalid state")
			}
		})
	}
}

func TestReadExtensionReportsMissingClientAndEmptyResponse(t *testing.T) {
	t.Parallel()

	state := extensionModel{ID: types.StringValue("extension_123"), ProjectID: types.StringNull()}
	tests := map[string]struct {
		client  extensionReader
		summary string
		detail  string
	}{
		"missing client": {
			summary: "Missing Kernel Client",
		},
		"empty response": {
			client: fakeExtensionReader{
				get: func(ctx context.Context, projectID, id string) (*kernel.ExtensionGetResponse, error) {
					return nil, nil
				},
			},
			summary: "Read Kernel Extension",
			detail:  `extension_123`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, removed, diags := readExtension(context.Background(), test.client, state)
			if removed {
				t.Fatal("removed = true, want false")
			}
			if !extensionDiagnosticContains(diags, test.summary, test.detail) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.summary)
			}
		})
	}
}
