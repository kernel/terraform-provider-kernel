package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ apiKeyClient = kernelclient.Clients{}

type fakeAPIKeyClient struct {
	get  func(context.Context, string) (*kernel.APIKey, error)
	list func(context.Context, string, int64) (kernelclient.APIKeyPage, error)
}

func (f fakeAPIKeyClient) GetAPIKey(ctx context.Context, id string) (*kernel.APIKey, error) {
	if f.get == nil {
		return nil, errors.New("unexpected API key get")
	}
	return f.get(ctx, id)
}

func (f fakeAPIKeyClient) ListAPIKeyPage(ctx context.Context, query string, offset int64) (kernelclient.APIKeyPage, error) {
	if f.list == nil {
		return kernelclient.APIKeyPage{}, errors.New("unexpected API key list")
	}
	return f.list(ctx, query, offset)
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()
	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_api_key" {
		t.Fatalf("TypeName = %q, want kernel_api_key", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "masked_key", "project_id", "project_name", "created_at", "expires_at", "created_by_id", "created_by_email", "created_by_name"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s", name)
		}
	}
	for _, name := range []string{"key", "deleted_at", "rotation_keeper", "days_to_expire"} {
		if _, ok := schema.Schema.Attributes[name]; ok {
			t.Fatalf("schema must not expose %s", name)
		}
	}
	masked, ok := schema.Schema.Attributes["masked_key"].(dschema.StringAttribute)
	if !ok || !masked.Sensitive || !masked.Computed {
		t.Fatalf("masked_key schema = %#v, want sensitive computed string", schema.Schema.Attributes["masked_key"])
	}
}

func TestReadByIDReturnsMaskedMetadata(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeAPIKeyClient{
		get: func(ctx context.Context, id string) (*kernel.APIKey, error) {
			key := apiKeyForTest(t, id, "deploy")
			return &key, nil
		},
	})
	state, diags := ds.read(context.Background(), apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringNull()})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "key-1" || state.Name.ValueString() != "deploy" {
		t.Fatalf("identity = %q/%q", state.ID.ValueString(), state.Name.ValueString())
	}
	if state.MaskedKey.ValueString() != "kern****test" {
		t.Fatalf("masked_key = %q", state.MaskedKey.ValueString())
	}
	if state.ProjectID.ValueString() != "project-1" || state.ProjectName.ValueString() != "Project" {
		t.Fatalf("project = %q/%q", state.ProjectID.ValueString(), state.ProjectName.ValueString())
	}
	if state.CreatedByID.ValueString() != "user-1" || state.CreatedByEmail.ValueString() != "user@example.com" {
		t.Fatalf("creator = %q/%q", state.CreatedByID.ValueString(), state.CreatedByEmail.ValueString())
	}
}

func TestReadByNameScansPagesAndDeduplicatesByID(t *testing.T) {
	t.Parallel()

	target := apiKeyForTest(t, "key-target", "deploy")
	ds := newDataSourceWithClient(fakeAPIKeyClient{
		list: listAPIKeyPages(t, "deploy", map[int64]kernelclient.APIKeyPage{
			0: {
				Items:       []kernel.APIKey{apiKeyForTest(t, "fuzzy", "deploy-old"), target},
				NextOffset:  100,
				HasNextPage: true,
			},
			100: {Items: []kernel.APIKey{target}},
		}),
	})
	state, diags := ds.read(context.Background(), apiKeyModel{ID: types.StringNull(), Name: types.StringValue("deploy")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "key-target" {
		t.Fatalf("id = %q, want key-target", state.ID.ValueString())
	}
}

func TestReadByNameDiagnosesMissingAndAmbiguousMatches(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		page kernelclient.APIKeyPage
		want string
	}{
		"missing": {
			page: kernelclient.APIKeyPage{Items: []kernel.APIKey{apiKeyForTest(t, "fuzzy", "deploy-old")}},
			want: "No non-deleted Kernel API key",
		},
		"ambiguous": {
			page: kernelclient.APIKeyPage{Items: []kernel.APIKey{
				apiKeyForTest(t, "key-1", "deploy"),
				apiKeyForTest(t, "key-2", "deploy"),
			}},
			want: "multiple non-deleted Kernel API keys",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(fakeAPIKeyClient{
				list: listAPIKeyPages(t, "deploy", map[int64]kernelclient.APIKeyPage{0: test.page}),
			})
			_, diags := ds.read(context.Background(), apiKeyModel{ID: types.StringNull(), Name: types.StringValue("deploy")})
			if !diags.HasError() || !diagnosticsContain(diags, test.want) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.want)
			}
		})
	}
}

func TestReadRejectsSelectorsAndClientFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		client apiKeyClient
		config apiKeyModel
		want   string
	}{
		"missing client": {
			config: apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringNull()},
			want:   "Missing Kernel Client",
		},
		"missing selector": {
			client: fakeAPIKeyClient{},
			config: apiKeyModel{ID: types.StringNull(), Name: types.StringNull()},
			want:   "Missing API Key Selector",
		},
		"conflicting selector": {
			client: fakeAPIKeyClient{},
			config: apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringValue("deploy")},
			want:   "Conflicting API Key Selectors",
		},
		"get error": {
			client: fakeAPIKeyClient{get: func(context.Context, string) (*kernel.APIKey, error) {
				return nil, errors.New("read failed")
			}},
			config: apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringNull()},
			want:   "read failed",
		},
		"empty response": {
			client: fakeAPIKeyClient{get: func(context.Context, string) (*kernel.APIKey, error) { return nil, nil }},
			config: apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringNull()},
			want:   "empty API key response",
		},
		"id mismatch": {
			client: fakeAPIKeyClient{get: func(context.Context, string) (*kernel.APIKey, error) {
				key := apiKeyForTest(t, "key-other", "deploy")
				return &key, nil
			}},
			config: apiKeyModel{ID: types.StringValue("key-1"), Name: types.StringNull()},
			want:   "ID Mismatch",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(test.client)
			_, diags := ds.read(context.Background(), test.config)
			if !diags.HasError() || !diagnosticsContain(diags, test.want) {
				t.Fatalf("diagnostics = %v, want %q", diags, test.want)
			}
		})
	}
}

func TestFlattenAPIKeyRejectsMalformedOrDeletedMetadata(t *testing.T) {
	t.Parallel()

	valid := apiKeyPayload("key-1", "deploy")
	for _, field := range []string{"id", "name", "masked_key", "created_at", "created_by", "deleted_at", "expires_at", "project_id", "project_name"} {
		t.Run("missing "+field, func(t *testing.T) {
			t.Parallel()
			payload := cloneMap(valid)
			delete(payload, field)
			_, diags := flattenAPIKey(apiKeyFromPayload(t, payload))
			if !diags.HasError() {
				t.Fatalf("expected diagnostics for missing %s", field)
			}
		})
	}

	t.Run("deleted key", func(t *testing.T) {
		t.Parallel()
		payload := cloneMap(valid)
		payload["deleted_at"] = "2026-07-11T13:00:00Z"
		_, diags := flattenAPIKey(apiKeyFromPayload(t, payload))
		if !diags.HasError() || !diagnosticsContain(diags, "deleted_at") {
			t.Fatalf("diagnostics = %v, want deleted_at", diags)
		}
	})

	t.Run("invalid creator email", func(t *testing.T) {
		t.Parallel()
		key := apiKeyFromPayload(t, valid)
		key.CreatedBy.Email = ""
		key.CreatedBy.JSON.Email = respjson.NewInvalidField("123")
		_, diags := flattenAPIKey(key)
		if !diags.HasError() || !diagnosticsContain(diags, "created_by.email") {
			t.Fatalf("diagnostics = %v, want created_by.email", diags)
		}
	})
}

func TestFlattenAPIKeyNormalizesNullableMetadata(t *testing.T) {
	t.Parallel()

	payload := apiKeyPayload("key-1", "org-wide")
	payload["expires_at"] = nil
	payload["project_id"] = nil
	payload["project_name"] = nil
	payload["created_by"] = map[string]any{"id": "user-1", "email": "user@example.com", "name": nil}
	state, diags := flattenAPIKey(apiKeyFromPayload(t, payload))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.ExpiresAt.IsNull() || !state.ProjectID.IsNull() || !state.ProjectName.IsNull() || !state.CreatedByName.IsNull() {
		t.Fatalf("nullable metadata = %v/%v/%v/%v", state.ExpiresAt, state.ProjectID, state.ProjectName, state.CreatedByName)
	}
}

func listAPIKeyPages(t *testing.T, query string, pages map[int64]kernelclient.APIKeyPage) func(context.Context, string, int64) (kernelclient.APIKeyPage, error) {
	t.Helper()
	return func(ctx context.Context, gotQuery string, offset int64) (kernelclient.APIKeyPage, error) {
		if gotQuery != query {
			t.Fatalf("query = %q, want %q", gotQuery, query)
		}
		page, ok := pages[offset]
		if !ok {
			t.Fatalf("unexpected offset %d", offset)
		}
		return page, nil
	}
}

func apiKeyForTest(t *testing.T, id, name string) kernel.APIKey {
	t.Helper()
	return apiKeyFromPayload(t, apiKeyPayload(id, name))
}

func apiKeyPayload(id, name string) map[string]any {
	return map[string]any{
		"id":           id,
		"name":         name,
		"masked_key":   "kern****test",
		"created_at":   "2026-07-11T12:00:00Z",
		"created_by":   map[string]any{"id": "user-1", "email": "user@example.com", "name": "User"},
		"deleted_at":   nil,
		"expires_at":   "2027-07-11T12:00:00Z",
		"project_id":   "project-1",
		"project_name": "Project",
	}
}

func apiKeyFromPayload(t *testing.T, payload map[string]any) kernel.APIKey {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal API key: %v", err)
	}
	var key kernel.APIKey
	if err := json.Unmarshal(raw, &key); err != nil {
		t.Fatalf("unmarshal API key: %v", err)
	}
	return key
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func diagnosticsContain(diags diag.Diagnostics, want string) bool {
	for _, diagnostic := range diags {
		if strings.Contains(diagnostic.Summary(), want) || strings.Contains(diagnostic.Detail(), want) {
			return true
		}
	}
	return false
}
