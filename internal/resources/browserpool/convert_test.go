package browserpool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestExpandCreateParamsMapsDurableConfigToSDK(t *testing.T) {
	model := BrowserPoolModel{
		Name:               types.StringValue("pool-a"),
		Size:               types.Int64Value(5),
		ProfileID:          types.StringValue("profile-1"),
		ProfileSaveChanges: types.BoolValue(true),
		ProxyID:            types.StringValue("proxy-1"),
		ExtensionIDs:       extensionIDsSet("ext-b", "ext-a", "ext-a"),
		ChromePolicy:       types.StringValue(`{"HomepageLocation":"https://example.com"}`),
		Viewport:           viewportValue(1280, 800, 60),
		Headless:           types.BoolValue(true),
		KioskMode:          types.BoolValue(true),
		Stealth:            types.BoolValue(false),
		StartURL:           types.StringValue("https://start.example"),
		TimeoutSeconds:     types.Int64Value(90),
		FillRatePerMinute:  types.Int64Value(20),
	}

	params, diags := ExpandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"name":                 "pool-a",
		"size":                 float64(5),
		"profile":              map[string]any{"id": "profile-1", "save_changes": true},
		"proxy_id":             "proxy-1",
		"extensions":           []any{map[string]any{"id": "ext-a"}, map[string]any{"id": "ext-b"}},
		"chrome_policy":        map[string]any{"HomepageLocation": "https://example.com"},
		"viewport":             map[string]any{"width": float64(1280), "height": float64(800), "refresh_rate": float64(60)},
		"headless":             true,
		"kiosk_mode":           true,
		"stealth":              false,
		"start_url":            "https://start.example",
		"timeout_seconds":      float64(90),
		"fill_rate_per_minute": float64(20),
	}

	if !jsonEqual(body, want) {
		t.Fatalf("expanded SDK JSON mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
}

func TestExpandCreateParamsRejectsProfileSaveChangesWithoutProfileID(t *testing.T) {
	model := BrowserPoolModel{
		Size:               types.Int64Value(1),
		ProfileSaveChanges: types.BoolValue(true),
	}

	_, diags := ExpandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics when profile_save_changes is set without profile_id")
	}
}

func TestExpandCreateParamsIgnoresFalseProfileSaveChangesWithoutProfileID(t *testing.T) {
	model := BrowserPoolModel{
		Size:               types.Int64Value(1),
		ProfileSaveChanges: types.BoolValue(false),
	}

	params, diags := ExpandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	if _, ok := body["profile"]; ok {
		t.Fatalf("profile = %#v, want omitted", body["profile"])
	}
}

func TestExpandCreateParamsRejectsEmptyExtensionID(t *testing.T) {
	model := BrowserPoolModel{
		Size: types.Int64Value(1),
		ExtensionIDs: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue(""),
		}),
	}

	_, diags := ExpandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for empty extension id")
	}
}

func TestExpandCreateParamsDecodesChromePolicyOnlyForSDKBoundary(t *testing.T) {
	model := BrowserPoolModel{
		Size:         types.Int64Value(1),
		ChromePolicy: types.StringValue(`{"HomepageLocation":"https://example.com"}`),
	}

	params, diags := ExpandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := model.ChromePolicy.ValueString(); got != `{"HomepageLocation":"https://example.com"}` {
		t.Fatalf("Terraform model chrome_policy changed to %q", got)
	}
	if params.ChromePolicy["HomepageLocation"] != "https://example.com" {
		t.Fatalf("chrome_policy was not decoded into SDK params: %#v", params.ChromePolicy)
	}
}

func TestFlattenBrowserPoolPreservesPriorOptionalNulls(t *testing.T) {
	prior := BrowserPoolModel{
		Viewport:          types.ObjectNull(viewportAttrTypes()),
		ChromePolicy:      types.StringNull(),
		ExtensionIDs:      types.SetNull(types.StringType),
		TimeoutSeconds:    types.Int64Null(),
		FillRatePerMinute: types.Int64Null(),
	}

	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 5,
		"available_count": 1,
		"browser_pool_config": {
			"size": 3,
			"viewport": {
				"width": 1920,
				"height": 1080,
				"refresh_rate": 25
			},
			"chrome_policy": {},
			"extensions": [],
			"timeout_seconds": 600,
			"fill_rate_per_minute": 10
		}
	}`)

	got, diags := FlattenBrowserPool(api, prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.ID.ValueString() != "pool-1" {
		t.Fatalf("id = %q, want pool-1", got.ID.ValueString())
	}
	if got.Size.ValueInt64() != 3 {
		t.Fatalf("size = %d, want 3", got.Size.ValueInt64())
	}
	if !got.Viewport.IsNull() {
		t.Fatalf("viewport = %#v, want prior null preserved", got.Viewport)
	}
	if !got.ChromePolicy.IsNull() {
		t.Fatalf("chrome_policy = %#v, want prior null preserved", got.ChromePolicy)
	}
	if !got.ExtensionIDs.IsNull() {
		t.Fatalf("extension_ids = %#v, want prior null preserved", got.ExtensionIDs)
	}
	if !got.TimeoutSeconds.IsNull() {
		t.Fatalf("timeout_seconds = %#v, want prior null preserved", got.TimeoutSeconds)
	}
	if !got.FillRatePerMinute.IsNull() {
		t.Fatalf("fill_rate_per_minute = %#v, want prior null preserved", got.FillRatePerMinute)
	}
}

func TestFlattenBrowserPoolUsesTopLevelName(t *testing.T) {
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"name": "top-level-name",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"name": "config-name"
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.Name.ValueString() != "top-level-name" {
		t.Fatalf("name = %q, want top-level-name", got.Name.ValueString())
	}
}

func TestFlattenBrowserPoolPreservesSemanticChromePolicyPrior(t *testing.T) {
	priorPolicy := "{\n  \"HomepageLocation\": \"https://example.com\",\n  \"Another\": true\n}"
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"chrome_policy": {
				"Another": true,
				"HomepageLocation": "https://example.com"
			}
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{
		ChromePolicy: types.StringValue(priorPolicy),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.ChromePolicy.ValueString() != priorPolicy {
		t.Fatalf("chrome_policy = %q, want prior representation preserved", got.ChromePolicy.ValueString())
	}
}

func TestFlattenBrowserPoolPreservesLargeChromePolicyNumberPrior(t *testing.T) {
	priorPolicy := `{"LargeInteger":9007199254740993}`
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"chrome_policy": {
				"LargeInteger": 9007199254740993
			}
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{
		ChromePolicy: types.StringValue(priorPolicy),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.ChromePolicy.ValueString() != priorPolicy {
		t.Fatalf("chrome_policy = %q, want exact prior number preserved", got.ChromePolicy.ValueString())
	}
}

func TestFlattenBrowserPoolPreservesPriorNullViewportRefreshRate(t *testing.T) {
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"viewport": {
				"width": 1280,
				"height": 800,
				"refresh_rate": 25
			}
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{
		Viewport: viewportValue(1280, 800, 0),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	refreshRate, ok := got.Viewport.Attributes()["refresh_rate"].(types.Int64)
	if !ok {
		t.Fatalf("refresh_rate has type %T, want Int64", got.Viewport.Attributes()["refresh_rate"])
	}
	if !refreshRate.IsNull() {
		t.Fatalf("refresh_rate = %#v, want prior null preserved", refreshRate)
	}
}

func TestFlattenBrowserPoolPreservesPriorRefreshRateWhenAPIOmitsIt(t *testing.T) {
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"viewport": {
				"width": 1280,
				"height": 800
			}
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{
		Viewport: viewportValue(1280, 800, 0),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	refreshRate, ok := got.Viewport.Attributes()["refresh_rate"].(types.Int64)
	if !ok {
		t.Fatalf("refresh_rate has type %T, want Int64", got.Viewport.Attributes()["refresh_rate"])
	}
	if !refreshRate.IsNull() {
		t.Fatalf("refresh_rate = %#v, want prior null preserved", refreshRate)
	}
}

func TestFlattenBrowserPoolPreservesPriorChromePolicyWhenAbsent(t *testing.T) {
	api := kernel.BrowserPool{
		ID: "pool-1",
		BrowserPoolConfig: kernel.BrowserPoolBrowserPoolConfig{
			Size: 3,
		},
	}

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{
		ChromePolicy: types.StringValue(`{"HomepageLocation":"https://example.com"}`),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.ChromePolicy.ValueString() != `{"HomepageLocation":"https://example.com"}` {
		t.Fatalf("chrome_policy = %q, want prior value preserved", got.ChromePolicy.ValueString())
	}
}

func TestFlattenBrowserPoolSortsExtensionIDs(t *testing.T) {
	api := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"created_at": "2026-01-01T00:00:00Z",
		"acquired_count": 0,
		"available_count": 0,
		"browser_pool_config": {
			"size": 3,
			"extensions": [
				{"id": "ext-b"},
				{"id": "ext-a"},
				{"id": "ext-a"}
			]
		}
	}`)

	got, diags := FlattenBrowserPool(api, BrowserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var ids []string
	diags = got.ExtensionIDs.ElementsAs(t.Context(), &ids, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading extension ids: %v", diags)
	}

	if len(ids) != 2 || ids[0] != "ext-a" || ids[1] != "ext-b" {
		t.Fatalf("extension ids = %#v, want stable sorted set", ids)
	}
}

func marshalSDKParams(t *testing.T, params kernel.BrowserPoolNewParams) map[string]any {
	t.Helper()

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal SDK params: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal SDK params: %v", err)
	}
	return got
}

func jsonEqual(got, want any) bool {
	gotData, _ := json.Marshal(got)
	wantData, _ := json.Marshal(want)
	return string(gotData) == string(wantData)
}

func unmarshalBrowserPool(t *testing.T, data string) kernel.BrowserPool {
	t.Helper()

	var pool kernel.BrowserPool
	if err := json.Unmarshal([]byte(data), &pool); err != nil {
		t.Fatalf("unmarshal browser pool: %v", err)
	}
	return pool
}
