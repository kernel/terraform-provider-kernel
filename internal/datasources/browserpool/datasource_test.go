package browserpool

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ browserPoolClient = kernelclient.Clients{}

type fakeBrowserPoolClient struct {
	defaultProjectID string
	get              func(context.Context, string, string) (*kernel.BrowserPool, error)
}

func (f fakeBrowserPoolClient) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeBrowserPoolClient) GetBrowserPool(ctx context.Context, projectID, idOrName string) (*kernel.BrowserPool, error) {
	if f.get == nil {
		return nil, errors.New("unexpected GetBrowserPool call")
	}
	return f.get(ctx, projectID, idOrName)
}

func TestDataSourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()
	var metadata datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_browser_pool" {
		t.Fatalf("type name = %q, want kernel_browser_pool", metadata.TypeName)
	}

	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "project_id", "size", "profile_id", "extension_ids"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("schema missing %s", name)
		}
	}
	for _, runtimeField := range []string{"acquired_count", "available_count", "standby", "sessions"} {
		if _, ok := schema.Schema.Attributes[runtimeField]; ok {
			t.Fatalf("schema includes runtime field %s", runtimeField)
		}
	}

	configured := &browserPoolDataSource{}
	var configure datasource.ConfigureResponse
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: kernelclient.Clients{}}, &configure)
	if configure.Diagnostics.HasError() || configured.client == nil {
		t.Fatalf("configure diagnostics/client = %v/%v", configure.Diagnostics, configured.client)
	}

	var invalid datasource.ConfigureResponse
	configured.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "not a client"}, &invalid)
	if len(invalid.Diagnostics) != 1 || invalid.Diagnostics[0].Summary() != "Unexpected Kernel Client Type" {
		t.Fatalf("invalid configure diagnostics = %v", invalid.Diagnostics)
	}
}

func TestDataSourceSchemaSemantics(t *testing.T) {
	t.Parallel()

	ds := NewDataSource()
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	assertAttributeMode(t, resp.Schema, "id", true, true)
	assertAttributeMode(t, resp.Schema, "name", true, true)
	assertAttributeMode(t, resp.Schema, "project_id", true, false)
	assertAttributeMode(t, resp.Schema, "size", false, true)
	assertAttributeMode(t, resp.Schema, "profile_id", false, true)
	assertAttributeMode(t, resp.Schema, "extension_ids", false, true)

	projectID := resp.Schema.Attributes["project_id"].(dschema.StringAttribute)
	if !validateProjectID(projectID.Validators, "").HasError() {
		t.Fatal("project_id accepted an empty string")
	}
	if diags := validateProjectID(projectID.Validators, "project-1"); diags.HasError() {
		t.Fatalf("project_id rejected a non-empty string: %v", diags)
	}
}

func assertAttributeMode(t *testing.T, schema dschema.Schema, name string, optional, computed bool) {
	t.Helper()
	attribute, ok := schema.Attributes[name]
	if !ok {
		t.Fatalf("schema missing %s", name)
	}
	if attribute.IsOptional() != optional || attribute.IsComputed() != computed || attribute.IsRequired() {
		t.Fatalf("%s has unexpected schema mode: %#v", name, attribute)
	}
}

func validateProjectID(validators []validator.String, value string) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, candidate := range validators {
		req := validator.StringRequest{ConfigValue: types.StringValue(value)}
		var resp validator.StringResponse
		candidate.ValidateString(context.Background(), req, &resp)
		diags.Append(resp.Diagnostics...)
	}
	return diags
}

func TestReadBrowserPoolByIDOrName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		config           browserPoolModel
		defaultProjectID string
		wantProjectID    string
		wantSelector     string
	}{
		"id": {
			config:           browserPoolModel{ID: types.StringValue("pool-1"), ProjectID: types.StringValue("project-explicit")},
			defaultProjectID: "project-default",
			wantProjectID:    "project-explicit",
			wantSelector:     "pool-1",
		},
		"name uses provider default": {
			config:           browserPoolModel{Name: types.StringValue("Pool"), ProjectID: types.StringNull()},
			defaultProjectID: "project-default",
			wantProjectID:    "project-default",
			wantSelector:     "Pool",
		},
		"unscoped": {
			config:       browserPoolModel{ID: types.StringValue("pool-1"), ProjectID: types.StringNull()},
			wantSelector: "pool-1",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var gotProjectID, gotSelector string
			ds := newDataSourceWithClient(fakeBrowserPoolClient{
				defaultProjectID: test.defaultProjectID,
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.BrowserPool, error) {
					gotProjectID, gotSelector = projectID, idOrName
					return browserPoolForTest("pool-1", "Pool", 2), nil
				},
			})

			state, diags := ds.read(context.Background(), test.config)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if gotSelector != test.wantSelector {
				t.Fatalf("selector = %q, want %q", gotSelector, test.wantSelector)
			}
			if gotProjectID != test.wantProjectID {
				t.Fatalf("project = %q, want %q", gotProjectID, test.wantProjectID)
			}
			if state.ID.ValueString() != "pool-1" || state.Name.ValueString() != "Pool" || state.Size.ValueInt64() != 2 {
				t.Fatalf("state = %#v", state)
			}
			if !state.ProjectID.Equal(test.config.ProjectID) {
				t.Fatalf("state project_id = %v, want %v", state.ProjectID, test.config.ProjectID)
			}
		})
	}
}

func TestReadSetsTerraformState(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeBrowserPoolClient{
		get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
			return browserPoolForTest("pool-1", "Pool", 2), nil
		},
	})
	var schema datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &schema)
	req := datasource.ReadRequest{Config: tfsdk.Config{
		Schema: schema.Schema,
		Raw: browserPoolConfigValue(
			tftypes.NewValue(tftypes.String, nil),
			tftypes.NewValue(tftypes.String, "Pool"),
			tftypes.NewValue(tftypes.String, nil),
		),
	}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}

	ds.Read(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var state browserPoolModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read state: %v", resp.Diagnostics)
	}
	if state.ID.ValueString() != "pool-1" || state.Name.ValueString() != "Pool" || state.Size.ValueInt64() != 2 {
		t.Fatalf("state = %#v", state)
	}
	if !state.ProfileID.IsNull() {
		t.Fatalf("profile_id = %v, want null", state.ProfileID)
	}
	assertBrowserPoolStringList(t, state.ExtensionIDs, nil)
}

func TestFlattenBrowserPoolResolvedReferences(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body             string
		wantProfileID    string
		wantProfileNull  bool
		wantExtensionIDs []string
	}{
		"authoritative fields": {
			body: `{
				"id":"pool-1",
				"profile_id":"profile-resolved",
				"extension_ids":["extension-b","extension-a"],
				"browser_pool_config":{
					"size":1,
					"profile":{"name":"profile-selector"},
					"extensions":[{"name":"extension-selector-b"},{"name":"extension-selector-a"}]
				}
			}`,
			wantProfileID:    "profile-resolved",
			wantExtensionIDs: []string{"extension-b", "extension-a"},
		},
		"legacy ID selectors": {
			body: `{
				"id":"pool-1",
				"browser_pool_config":{
					"size":1,
					"profile":{"id":"profile-legacy"},
					"extensions":[{"id":"extension-b"},{"id":"extension-a"}]
				}
			}`,
			wantProfileID:    "profile-legacy",
			wantExtensionIDs: []string{"extension-b", "extension-a"},
		},
		"no references": {
			body:             `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1}}`,
			wantProfileNull:  true,
			wantExtensionIDs: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, diags := flattenBrowserPool(*browserPoolFromJSON(test.body))
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if test.wantProfileNull {
				if !state.ProfileID.IsNull() {
					t.Fatalf("profile_id = %v, want null", state.ProfileID)
				}
			} else if state.ProfileID.ValueString() != test.wantProfileID {
				t.Fatalf("profile_id = %q, want %q", state.ProfileID.ValueString(), test.wantProfileID)
			}
			assertBrowserPoolStringList(t, state.ExtensionIDs, test.wantExtensionIDs)
		})
	}
}

func TestFlattenBrowserPoolRejectsInvalidResolvedReferences(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"null authoritative extensions":       `{"id":"pool-1","extension_ids":null,"browser_pool_config":{"size":1}}`,
		"non-list authoritative extensions":   `{"id":"pool-1","extension_ids":{},"browser_pool_config":{"size":1}}`,
		"empty authoritative extension ID":    `{"id":"pool-1","extension_ids":[""],"browser_pool_config":{"size":1}}`,
		"null authoritative profile ID":       `{"id":"pool-1","profile_id":null,"extension_ids":[],"browser_pool_config":{"size":1}}`,
		"non-string authoritative profile ID": `{"id":"pool-1","profile_id":1,"extension_ids":[],"browser_pool_config":{"size":1}}`,
		"empty authoritative profile ID":      `{"id":"pool-1","profile_id":"","extension_ids":[],"browser_pool_config":{"size":1}}`,
		"legacy profile name only":            `{"id":"pool-1","browser_pool_config":{"size":1,"profile":{"name":"profile-selector"}}}`,
		"legacy extension name only":          `{"id":"pool-1","browser_pool_config":{"size":1,"extensions":[{"name":"extension-selector"}]}}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenBrowserPool(*browserPoolFromJSON(body))
			if !diags.HasError() {
				t.Fatal("expected diagnostics")
			}
		})
	}
}

func TestReadBrowserPoolAllowsUnnamedPoolByID(t *testing.T) {
	t.Parallel()

	ds := newDataSourceWithClient(fakeBrowserPoolClient{
		get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
			return browserPoolForTest("pool-1", "", 1), nil
		},
	})
	state, diags := ds.read(context.Background(), browserPoolModel{ID: types.StringValue("pool-1")})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Name.IsNull() {
		t.Fatalf("name = %v, want null", state.Name)
	}
}

func TestReadBrowserPoolRejectsInvalidInputsAndResponses(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		client browserPoolClient
		config browserPoolModel
	}{
		"missing client":   {config: browserPoolModel{ID: types.StringValue("pool-1")}},
		"missing selector": {client: fakeBrowserPoolClient{}},
		"conflicting selectors": {
			client: fakeBrowserPoolClient{},
			config: browserPoolModel{ID: types.StringValue("pool-1"), Name: types.StringValue("Pool")},
		},
		"unknown project": {
			client: fakeBrowserPoolClient{},
			config: browserPoolModel{ID: types.StringValue("pool-1"), ProjectID: types.StringUnknown()},
		},
		"API error": {
			client: fakeBrowserPoolClient{get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
				return nil, errors.New("connection reset")
			}},
			config: browserPoolModel{ID: types.StringValue("pool-1")},
		},
		"empty response": {
			client: fakeBrowserPoolClient{get: func(context.Context, string, string) (*kernel.BrowserPool, error) { return nil, nil }},
			config: browserPoolModel{ID: types.StringValue("pool-1")},
		},
		"invalid response": {
			client: fakeBrowserPoolClient{get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
				return browserPoolFromJSON(`{"id":"pool-1","browser_pool_config":{"size":"2"}}`), nil
			}},
			config: browserPoolModel{ID: types.StringValue("pool-1")},
		},
		"ID mismatch": {
			client: fakeBrowserPoolClient{get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
				return browserPoolForTest("pool-other", "Pool", 1), nil
			}},
			config: browserPoolModel{ID: types.StringValue("pool-1")},
		},
		"name mismatch": {
			client: fakeBrowserPoolClient{get: func(context.Context, string, string) (*kernel.BrowserPool, error) {
				return browserPoolForTest("pool-1", "Other", 1), nil
			}},
			config: browserPoolModel{Name: types.StringValue("Pool")},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ds := newDataSourceWithClient(test.client)
			_, diags := ds.read(context.Background(), test.config)
			if !diags.HasError() {
				t.Fatal("expected diagnostics")
			}
		})
	}
}

func browserPoolForTest(id, name string, size int64) *kernel.BrowserPool {
	nameJSON := "null"
	configName := ""
	if name != "" {
		nameJSON = strconv.Quote(name)
		configName = `,"name":` + strconv.Quote(name)
	}
	return browserPoolFromJSON(`{"id":` + strconv.Quote(id) + `,"name":` + nameJSON + `,"extension_ids":[],"browser_pool_config":{"size":` + strconv.FormatInt(size, 10) + configName + `}}`)
}

func browserPoolFromJSON(body string) *kernel.BrowserPool {
	var pool kernel.BrowserPool
	if err := json.Unmarshal([]byte(body), &pool); err != nil {
		panic(err)
	}
	return &pool
}

func browserPoolConfigValue(id, name, projectID tftypes.Value) tftypes.Value {
	return tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"id":            tftypes.String,
			"name":          tftypes.String,
			"project_id":    tftypes.String,
			"size":          tftypes.Number,
			"profile_id":    tftypes.String,
			"extension_ids": tftypes.List{ElementType: tftypes.String},
		}},
		map[string]tftypes.Value{
			"id":            id,
			"name":          name,
			"project_id":    projectID,
			"size":          tftypes.NewValue(tftypes.Number, nil),
			"profile_id":    tftypes.NewValue(tftypes.String, nil),
			"extension_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		},
	)
}

func assertBrowserPoolStringList(t *testing.T, got types.List, want []string) {
	t.Helper()
	if got.IsNull() || got.IsUnknown() {
		t.Fatalf("list = %v, want %v", got, want)
	}
	elements := got.Elements()
	if len(elements) != len(want) {
		t.Fatalf("list length = %d, want %d", len(elements), len(want))
	}
	for index, element := range elements {
		value, ok := element.(types.String)
		if !ok || value.ValueString() != want[index] {
			t.Fatalf("list[%d] = %v, want %q", index, element, want[index])
		}
	}
}
