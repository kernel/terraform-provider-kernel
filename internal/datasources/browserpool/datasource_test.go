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
	for _, name := range []string{"id", "name", "project_id", "size", "profile_id", "refresh_on_profile_update", "extension_ids", "proxy_id", "headless", "kiosk_mode", "stealth", "start_url", "timeout_seconds", "fill_rate_per_minute", "viewport", "chrome_policy"} {
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
	assertAttributeMode(t, resp.Schema, "refresh_on_profile_update", false, true)
	assertAttributeMode(t, resp.Schema, "extension_ids", false, true)
	assertAttributeMode(t, resp.Schema, "proxy_id", false, true)
	assertAttributeMode(t, resp.Schema, "headless", false, true)
	assertAttributeMode(t, resp.Schema, "kiosk_mode", false, true)
	assertAttributeMode(t, resp.Schema, "stealth", false, true)
	assertAttributeMode(t, resp.Schema, "start_url", false, true)
	assertAttributeMode(t, resp.Schema, "timeout_seconds", false, true)
	assertAttributeMode(t, resp.Schema, "fill_rate_per_minute", false, true)
	assertAttributeMode(t, resp.Schema, "viewport", false, true)

	viewport := resp.Schema.Attributes["viewport"].(dschema.SingleNestedAttribute)
	for _, name := range []string{"width", "height", "refresh_rate"} {
		assertAttributeMapMode(t, viewport.Attributes, name, false, true)
	}
	assertAttributeMode(t, resp.Schema, "chrome_policy", false, true)

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
	assertAttributeMapMode(t, schema.Attributes, name, optional, computed)
}

func assertAttributeMapMode(t *testing.T, attributes map[string]dschema.Attribute, name string, optional, computed bool) {
	t.Helper()
	attribute, ok := attributes[name]
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
			return browserPoolFromJSON(`{
				"id":"pool-1",
				"name":"Pool",
				"extension_ids":[],
				"browser_pool_config":{
					"size":2,
					"proxy_id":"proxy-1",
					"headless":true,
					"kiosk_mode":false,
					"stealth":true,
					"refresh_on_profile_update":true,
					"start_url":"chrome://newtab",
					"timeout_seconds":10,
					"fill_rate_per_minute":0,
					"viewport":{"width":1280,"height":800,"refresh_rate":60},
					"chrome_policy":{"RestoreOnStartup":4,"HomepageLocation":"https://example.com?x=1&y=2"}
				}
			}`), nil
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
	if state.ProxyID.ValueString() != "proxy-1" || !state.Headless.ValueBool() || state.KioskMode.IsNull() || state.KioskMode.ValueBool() || !state.Stealth.ValueBool() {
		t.Fatalf("launch state = %#v", state)
	}
	if !state.RefreshOnProfile.ValueBool() {
		t.Fatal("refresh_on_profile_update = false, want true")
	}
	if state.StartURL.ValueString() != "chrome://newtab" || state.TimeoutSeconds.ValueInt64() != 10 || state.FillRatePerMinute.IsNull() || state.FillRatePerMinute.IsUnknown() || state.FillRatePerMinute.ValueInt64() != 0 {
		t.Fatalf("warmup state = %#v", state)
	}
	assertBrowserPoolViewport(t, state.Viewport, 1280, 800, types.Int64Value(60))
	if state.ChromePolicy.ValueString() != `{"HomepageLocation":"https://example.com?x=1&y=2","RestoreOnStartup":4}` {
		t.Fatalf("chrome_policy = %q", state.ChromePolicy.ValueString())
	}
}

func TestFlattenBrowserPoolChromePolicyNormalization(t *testing.T) {
	t.Parallel()

	omitted, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1}}`))
	if diags.HasError() {
		t.Fatalf("unexpected omitted policy diagnostics: %v", diags)
	}
	if !omitted.ChromePolicy.IsNull() {
		t.Fatalf("omitted chrome_policy = %v, want null", omitted.ChromePolicy)
	}
	explicitNull, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"chrome_policy":null}}`))
	if diags.HasError() || !explicitNull.ChromePolicy.IsNull() {
		t.Fatalf("explicit-null chrome_policy = %v, diagnostics = %v", explicitNull.ChromePolicy, diags)
	}

	state, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"chrome_policy":{"Tag":"<b>","Number":1.0,"Nested":{"enabled":true}}}}`))
	if diags.HasError() {
		t.Fatalf("unexpected policy diagnostics: %v", diags)
	}
	want := `{"Nested":{"enabled":true},"Number":1,"Tag":"<b>"}`
	if state.ChromePolicy.ValueString() != want {
		t.Fatalf("chrome_policy = %q, want %q", state.ChromePolicy.ValueString(), want)
	}
}

func TestFlattenBrowserPoolRejectsInvalidChromePolicy(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"array":  `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"chrome_policy":[]}}`,
		"string": `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"chrome_policy":"policy"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, diags := flattenBrowserPool(*browserPoolFromJSON(body))
			if !diags.HasError() {
				t.Fatal("expected diagnostics")
			}
		})
	}
}

func TestFlattenBrowserPoolViewportOptionalFields(t *testing.T) {
	t.Parallel()

	omitted, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1}}`))
	if diags.HasError() {
		t.Fatalf("unexpected omitted viewport diagnostics: %v", diags)
	}
	if !omitted.Viewport.IsNull() || len(omitted.Viewport.AttributeTypes(t.Context())) != 3 {
		t.Fatalf("omitted viewport = %#v, want typed null", omitted.Viewport)
	}

	withoutRefreshRate, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1280,"height":800}}}`))
	if diags.HasError() {
		t.Fatalf("unexpected viewport diagnostics: %v", diags)
	}
	assertBrowserPoolViewport(t, withoutRefreshRate.Viewport, 1280, 800, types.Int64Null())
}

func TestFlattenBrowserPoolAcceptsMinimumViewport(t *testing.T) {
	t.Parallel()

	state, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1,"height":1,"refresh_rate":1}}}`))
	if diags.HasError() {
		t.Fatalf("unexpected minimum viewport diagnostics: %v", diags)
	}
	assertBrowserPoolViewport(t, state.Viewport, 1, 1, types.Int64Value(1))
}

func TestFlattenBrowserPoolRejectsInvalidViewport(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"null viewport":        `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":null}}`,
		"non-object viewport":  `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":[]}}`,
		"missing width":        `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"height":800}}}`,
		"missing height":       `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1280}}}`,
		"zero width":           `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":0,"height":800}}}`,
		"negative height":      `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1280,"height":-1}}}`,
		"null refresh rate":    `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1280,"height":800,"refresh_rate":null}}}`,
		"zero refresh rate":    `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":1280,"height":800,"refresh_rate":0}}}`,
		"non-number dimension": `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"viewport":{"width":"1280","height":800}}}`,
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

func TestFlattenBrowserPoolWarmupConfigurationBoundaries(t *testing.T) {
	t.Parallel()

	omitted, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1}}`))
	if diags.HasError() {
		t.Fatalf("unexpected omitted-field diagnostics: %v", diags)
	}
	if !omitted.StartURL.IsNull() || !omitted.TimeoutSeconds.IsNull() || !omitted.FillRatePerMinute.IsNull() {
		t.Fatalf("omitted warmup state = %#v, want null values", omitted)
	}

	boundary, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"timeout_seconds":259200,"fill_rate_per_minute":0}}`))
	if diags.HasError() {
		t.Fatalf("unexpected boundary diagnostics: %v", diags)
	}
	if boundary.TimeoutSeconds.ValueInt64() != 259200 || boundary.FillRatePerMinute.IsNull() || boundary.FillRatePerMinute.IsUnknown() || boundary.FillRatePerMinute.ValueInt64() != 0 {
		t.Fatalf("boundary warmup state = %#v", boundary)
	}
}

func TestFlattenBrowserPoolRejectsInvalidWarmupConfiguration(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty start URL":    `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"start_url":""}}`,
		"null start URL":     `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"start_url":null}}`,
		"non-string URL":     `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"start_url":1}}`,
		"null timeout":       `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"timeout_seconds":null}}`,
		"non-number timeout": `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"timeout_seconds":"10"}}`,
		"timeout too low":    `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"timeout_seconds":9}}`,
		"timeout too high":   `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"timeout_seconds":259201}}`,
		"null fill rate":     `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"fill_rate_per_minute":null}}`,
		"non-number rate":    `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"fill_rate_per_minute":"0"}}`,
		"negative fill rate": `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"fill_rate_per_minute":-1}}`,
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

func TestFlattenBrowserPoolLaunchConfiguration(t *testing.T) {
	t.Parallel()

	omitted, diags := flattenBrowserPool(*browserPoolFromJSON(`{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1}}`))
	if diags.HasError() || !omitted.RefreshOnProfile.IsNull() {
		t.Fatalf("omitted refresh_on_profile_update = %v, diagnostics = %v", omitted.RefreshOnProfile, diags)
	}

	state, diags := flattenBrowserPool(*browserPoolFromJSON(`{
		"id":"pool-1",
		"extension_ids":[],
		"browser_pool_config":{
			"size":1,
			"proxy_id":"proxy-1",
			"headless":true,
			"kiosk_mode":false,
			"stealth":true,
			"refresh_on_profile_update":false
		}
	}`))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ProxyID.ValueString() != "proxy-1" {
		t.Fatalf("proxy_id = %q, want proxy-1", state.ProxyID.ValueString())
	}
	if !state.Headless.ValueBool() {
		t.Fatal("headless = false, want true")
	}
	if state.KioskMode.IsNull() || state.KioskMode.ValueBool() {
		t.Fatalf("kiosk_mode = %v, want known false", state.KioskMode)
	}
	if !state.Stealth.ValueBool() {
		t.Fatal("stealth = false, want true")
	}
	if state.RefreshOnProfile.IsNull() || state.RefreshOnProfile.ValueBool() {
		t.Fatalf("refresh_on_profile_update = %v, want known false", state.RefreshOnProfile)
	}
}

func TestFlattenBrowserPoolRejectsInvalidLaunchConfiguration(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty proxy ID":           `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"proxy_id":""}}`,
		"null proxy ID":            `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"proxy_id":null}}`,
		"non-string proxy":         `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"proxy_id":1}}`,
		"null headless":            `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"headless":null}}`,
		"non-bool headless":        `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"headless":"true"}}`,
		"null kiosk":               `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"kiosk_mode":null}}`,
		"non-bool kiosk":           `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"kiosk_mode":1}}`,
		"null stealth":             `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"stealth":null}}`,
		"non-bool stealth":         `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"stealth":{}}}`,
		"null profile refresh":     `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"refresh_on_profile_update":null}}`,
		"non-bool profile refresh": `{"id":"pool-1","extension_ids":[],"browser_pool_config":{"size":1,"refresh_on_profile_update":"false"}}`,
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
	viewportType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"width":        tftypes.Number,
		"height":       tftypes.Number,
		"refresh_rate": tftypes.Number,
	}}
	return tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"id":                        tftypes.String,
			"name":                      tftypes.String,
			"project_id":                tftypes.String,
			"size":                      tftypes.Number,
			"profile_id":                tftypes.String,
			"refresh_on_profile_update": tftypes.Bool,
			"extension_ids":             tftypes.List{ElementType: tftypes.String},
			"proxy_id":                  tftypes.String,
			"headless":                  tftypes.Bool,
			"kiosk_mode":                tftypes.Bool,
			"stealth":                   tftypes.Bool,
			"start_url":                 tftypes.String,
			"timeout_seconds":           tftypes.Number,
			"fill_rate_per_minute":      tftypes.Number,
			"viewport":                  viewportType,
			"chrome_policy":             tftypes.String,
		}},
		map[string]tftypes.Value{
			"id":                        id,
			"name":                      name,
			"project_id":                projectID,
			"size":                      tftypes.NewValue(tftypes.Number, nil),
			"profile_id":                tftypes.NewValue(tftypes.String, nil),
			"refresh_on_profile_update": tftypes.NewValue(tftypes.Bool, nil),
			"extension_ids":             tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
			"proxy_id":                  tftypes.NewValue(tftypes.String, nil),
			"headless":                  tftypes.NewValue(tftypes.Bool, nil),
			"kiosk_mode":                tftypes.NewValue(tftypes.Bool, nil),
			"stealth":                   tftypes.NewValue(tftypes.Bool, nil),
			"start_url":                 tftypes.NewValue(tftypes.String, nil),
			"timeout_seconds":           tftypes.NewValue(tftypes.Number, nil),
			"fill_rate_per_minute":      tftypes.NewValue(tftypes.Number, nil),
			"viewport":                  tftypes.NewValue(viewportType, nil),
			"chrome_policy":             tftypes.NewValue(tftypes.String, nil),
		},
	)
}

func assertBrowserPoolViewport(t *testing.T, viewport types.Object, width, height int64, refreshRate types.Int64) {
	t.Helper()
	if viewport.IsNull() || viewport.IsUnknown() {
		t.Fatalf("viewport = %#v, want known object", viewport)
	}
	attributes := viewport.Attributes()
	if !attributes["width"].(types.Int64).Equal(types.Int64Value(width)) || !attributes["height"].(types.Int64).Equal(types.Int64Value(height)) || !attributes["refresh_rate"].(types.Int64).Equal(refreshRate) {
		t.Fatalf("viewport = %#v, want %dx%d refresh %v", viewport, width, height, refreshRate)
	}
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
