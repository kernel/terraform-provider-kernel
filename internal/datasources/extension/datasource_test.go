package extension

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionClient = kernelclient.Clients{}

type fakeExtensionClient struct {
	defaultProjectID string
	get              func(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

func (f fakeExtensionClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeExtensionClient) GetExtension(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
	if f.get == nil {
		return nil, errors.New("unexpected GetExtension call")
	}
	return f.get(ctx, projectID, idOrName)
}

func TestReadResolvesProjectScope(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configProjectID  types.String
		defaultProjectID string
		wantProjectID    string
	}{
		"explicit attribute wins over provider default": {
			configProjectID:  types.StringValue("proj_attr"),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_attr",
		},
		"unset attribute inherits provider default": {
			configProjectID:  types.StringNull(),
			defaultProjectID: "proj_default",
			wantProjectID:    "proj_default",
		},
		"nothing set stays unscoped": {
			configProjectID:  types.StringNull(),
			defaultProjectID: "",
			wantProjectID:    "",
		},
	}

	t.Run("unknown attribute errors instead of reading the default", func(t *testing.T) {
		t.Parallel()

		ds := newDataSourceWithClient(fakeExtensionClient{defaultProjectID: "proj_default"})
		_, diags := ds.read(context.Background(), extensionModel{
			ID:        types.StringValue("extension-1"),
			ProjectID: types.StringUnknown(),
		})
		if !diags.HasError() {
			t.Fatal("expected diagnostics for unknown project_id")
		}
	})

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var gotProjectID string
			ds := newDataSourceWithClient(fakeExtensionClient{
				defaultProjectID: test.defaultProjectID,
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
					gotProjectID = projectID
					return extensionForTest("extension-1", "Extension"), nil
				},
			})

			state, diags := ds.read(context.Background(), extensionModel{
				ID:        types.StringValue("extension-1"),
				ProjectID: test.configProjectID,
			})
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("GetExtension project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if !state.ProjectID.Equal(test.configProjectID) {
				t.Fatalf("state project_id = %v, want configured value %v echoed", state.ProjectID, test.configProjectID)
			}
		})
	}
}

func TestDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()

	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_extension" {
		t.Fatalf("TypeName = %q, want kernel_extension", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "created_at", "size_bytes", "last_used_at"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s attribute", name)
		}
	}
	for _, name := range []string{"download_url", "zip_file"} {
		if _, ok := schema.Schema.Attributes[name]; ok {
			t.Fatalf("schema should not include %s", name)
		}
	}
}

func TestReadSetsTerraformState(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			return extensionForTest("extension-target", "Target"), nil
		},
	})

	var schemaResp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	req := datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    extensionConfigValue(tftypes.NewValue(tftypes.String, nil), tftypes.NewValue(tftypes.String, "Target")),
		},
	}
	resp := datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state extensionModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected state diagnostics: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "extension-target" {
		t.Fatalf("state id = %q, want extension-target", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Target" {
		t.Fatalf("state name = %q, want Target", state.Name.ValueString())
	}
	if state.SizeBytes.ValueInt64() != 1234 {
		t.Fatalf("state size_bytes = %d, want 1234", state.SizeBytes.ValueInt64())
	}
	if state.LastUsedAt.ValueString() != "2026-06-05T12:00:00Z" {
		t.Fatalf("state last_used_at = %q, want 2026-06-05T12:00:00Z", state.LastUsedAt.ValueString())
	}
}

func TestReadExtensionByID(t *testing.T) {
	t.Parallel()

	var gotIDOrName string
	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			gotIDOrName = idOrName
			return extensionForTest("extension-target", "Target"), nil
		},
	})

	state, diags := ds.read(context.Background(), extensionModel{ID: types.StringValue("extension-target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotIDOrName != "extension-target" {
		t.Fatalf("GetExtension idOrName = %q, want extension-target", gotIDOrName)
	}
	if state.Name.ValueString() != "Target" {
		t.Fatalf("state name = %q, want Target", state.Name.ValueString())
	}
}

func TestReadExtensionByName(t *testing.T) {
	t.Parallel()

	var gotIDOrName string
	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			gotIDOrName = idOrName
			return extensionForTest("extension-target", "Target"), nil
		},
	})

	state, diags := ds.read(context.Background(), extensionModel{Name: types.StringValue("Target")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if gotIDOrName != "Target" {
		t.Fatalf("GetExtension idOrName = %q, want Target (server resolves name)", gotIDOrName)
	}
	if state.ID.ValueString() != "extension-target" {
		t.Fatalf("state id = %q, want extension-target", state.ID.ValueString())
	}
}

func TestReadExtensionAllowsNullableName(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			return extensionWithoutNameForTest("extension-1"), nil
		},
	})

	state, diags := ds.read(context.Background(), extensionModel{ID: types.StringValue("extension-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Name.IsNull() {
		t.Fatalf("state name = %q, want null", state.Name.ValueString())
	}
}

func TestReadExtensionAllowsNullableLastUsedAt(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			return extensionWithoutLastUsedForTest("extension-1", "Extension"), nil
		},
	})

	state, diags := ds.read(context.Background(), extensionModel{ID: types.StringValue("extension-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.LastUsedAt.IsNull() {
		t.Fatalf("state last_used_at = %q, want null", state.LastUsedAt.ValueString())
	}
}

func TestReadRejectsExtensionNotFound(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			return nil, errors.New("404 Not Found: extension not found")
		},
	})

	_, diags := ds.read(context.Background(), extensionModel{Name: types.StringValue("Missing")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for a not-found extension")
	}
}

func TestReadRejectsInvalidExtensionResponseField(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{
		get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
			return invalidExtension(), nil
		},
	})

	_, diags := ds.read(context.Background(), extensionModel{ID: types.StringValue("extension-1")})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid extension response field")
	}
}

func TestReadRejectsMissingExtensionSelector(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeExtensionClient{})

	_, diags := ds.read(context.Background(), extensionModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for missing extension selector")
	}
}

func TestReadRejectsEmptyExtensionSelectors(t *testing.T) {
	t.Parallel()

	tests := map[string]extensionModel{
		"empty id": {
			ID: types.StringValue(""),
		},
		"empty name": {
			Name: types.StringValue(""),
		},
	}

	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			called := false
			ds := newDataSourceWithClient(fakeExtensionClient{
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
					called = true
					return nil, errors.New("should not call GetExtension")
				},
			})

			_, diags := ds.read(context.Background(), config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics for empty selector")
			}
			if called {
				t.Fatal("GetExtension was called for empty selector")
			}
		})
	}
}

func extensionForTest(id, name string) *kernel.ExtensionGetResponse {
	return extensionFromJSON(`{"id":"` + id + `","name":"` + name + `","created_at":"2026-06-05T12:00:00Z","size_bytes":1234,"last_used_at":"2026-06-05T12:00:00Z"}`)
}

func extensionWithoutNameForTest(id string) *kernel.ExtensionGetResponse {
	return extensionFromJSON(`{"id":"` + id + `","name":null,"created_at":"2026-06-05T12:00:00Z","size_bytes":1234,"last_used_at":"2026-06-05T12:00:00Z"}`)
}

func extensionWithoutLastUsedForTest(id, name string) *kernel.ExtensionGetResponse {
	return extensionFromJSON(`{"id":"` + id + `","name":"` + name + `","created_at":"2026-06-05T12:00:00Z","size_bytes":1234,"last_used_at":null}`)
}

func invalidExtension() *kernel.ExtensionGetResponse {
	extension := extensionForTest("extension-1", "Extension")
	extension.CreatedAt = time.Time{}
	extension.JSON.CreatedAt = respjson.NewInvalidField("123")
	return extension
}

func extensionFromJSON(body string) *kernel.ExtensionGetResponse {
	var extension kernel.ExtensionGetResponse
	if err := json.Unmarshal([]byte(body), &extension); err != nil {
		panic(err)
	}
	return &extension
}

func extensionConfigValue(id, name tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"id":           tftypes.String,
				"name":         tftypes.String,
				"project_id":   tftypes.String,
				"created_at":   tftypes.String,
				"size_bytes":   tftypes.Number,
				"last_used_at": tftypes.String,
			},
		},
		map[string]tftypes.Value{
			"id":           id,
			"name":         name,
			"project_id":   tftypes.NewValue(tftypes.String, nil),
			"created_at":   tftypes.NewValue(tftypes.String, nil),
			"size_bytes":   tftypes.NewValue(tftypes.Number, nil),
			"last_used_at": tftypes.NewValue(tftypes.String, nil),
		},
	)
}
