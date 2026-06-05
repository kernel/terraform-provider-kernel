package browserpool

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestExpandCreateParamsMapsDurableConfigToSDK(t *testing.T) {
	model := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(5),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringValue("proxy-1"),
		ExtensionIDs:      stringListForTest("ext-b", "ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
		Viewport:          viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Value(60)),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(true),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringValue("https://start.example"),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(20),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"name":                 "pool-a",
		"size":                 float64(5),
		"profile":              map[string]any{"id": "profile-1"},
		"proxy_id":             "proxy-1",
		"extensions":           []any{map[string]any{"id": "ext-b"}, map[string]any{"id": "ext-a"}},
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

func TestExpandCreateParamsOmitsUnknownServerDefaults(t *testing.T) {
	model := browserPoolModel{
		Size:              types.Int64Value(1),
		Headless:          types.BoolUnknown(),
		KioskMode:         types.BoolUnknown(),
		Stealth:           types.BoolUnknown(),
		TimeoutSeconds:    types.Int64Unknown(),
		FillRatePerMinute: types.Int64Unknown(),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"size": float64(1),
	}
	if !jsonEqual(body, want) {
		t.Fatalf("expanded SDK JSON mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
}

func TestExpandCreateParamsRejectsUnknownSize(t *testing.T) {
	model := browserPoolModel{
		Size: types.Int64Unknown(),
	}

	_, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for unknown size")
	}
}

func TestExpandCreateParamsRejectsUnknownNonComputedOptionalValue(t *testing.T) {
	model := browserPoolModel{
		Size:    types.Int64Value(1),
		ProxyID: types.StringUnknown(),
	}

	_, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for unknown proxy_id")
	}
}

func TestExpandCreateParamsRejectsUnknownViewportDimensions(t *testing.T) {
	model := browserPoolModel{
		Size:     types.Int64Value(1),
		Viewport: viewportObjectForTest(types.Int64Unknown(), types.Int64Value(800), types.Int64Null()),
	}

	_, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for unknown viewport.width")
	}
}

func TestExpandCreateParamsOmitsUnknownViewportRefreshRate(t *testing.T) {
	model := browserPoolModel{
		Size:     types.Int64Value(1),
		Viewport: viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Unknown()),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"size":     float64(1),
		"viewport": map[string]any{"width": float64(1280), "height": float64(800)},
	}
	if !jsonEqual(body, want) {
		t.Fatalf("expanded SDK JSON mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
}

func TestExpandCreateParamsDecodesChromePolicyOnlyForSDKBoundary(t *testing.T) {
	model := browserPoolModel{
		Size:         types.Int64Value(1),
		ChromePolicy: chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
	}

	params, diags := expandCreateParams(context.Background(), model)
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

func TestExpandCreateParamsRejectsInvalidChromePolicy(t *testing.T) {
	model := browserPoolModel{
		Name:         types.StringValue("partial-would-be-set"),
		Size:         types.Int64Value(1),
		ChromePolicy: chromePolicyValueForTest(`[]`),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid chrome_policy")
	}
	if !hasDiagnosticPath(diags, path.Root("chrome_policy")) {
		t.Fatalf("expected diagnostic at chrome_policy, got %v", diags)
	}
	assertEmptySDKParams(t, params)
}

func TestExpandCreateParamsReturnsEmptyParamsOnViewportError(t *testing.T) {
	model := browserPoolModel{
		Name:     types.StringValue("partial-would-be-set"),
		Size:     types.Int64Value(1),
		Viewport: viewportObjectForTest(types.Int64Unknown(), types.Int64Value(800), types.Int64Null()),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid viewport")
	}
	assertEmptySDKParams(t, params)
}

func TestExpandUpdateParamsMapsChangedDurableConfigToSDKPatch(t *testing.T) {
	plan := browserPoolModel{
		Name:              types.StringValue("pool-b"),
		Size:              types.Int64Value(3),
		ProfileID:         types.StringValue("profile-2"),
		ProxyID:           types.StringValue("proxy-2"),
		ExtensionIDs:      stringListForTest("ext-b", "ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://new.example"}`),
		Viewport:          viewportObjectForTest(types.Int64Value(1440), types.Int64Value(900), types.Int64Value(30)),
		Headless:          types.BoolValue(false),
		KioskMode:         types.BoolValue(true),
		Stealth:           types.BoolValue(true),
		StartURL:          types.StringValue("https://new.example"),
		TimeoutSeconds:    types.Int64Value(120),
		FillRatePerMinute: types.Int64Value(0),
	}
	state := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringValue("proxy-1"),
		ExtensionIDs:      stringListForTest("ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://old.example"}`),
		Viewport:          viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Value(60)),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringValue("https://old.example"),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	params, diags := expandUpdateParams(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"name":                 "pool-b",
		"size":                 float64(3),
		"profile":              map[string]any{"id": "profile-2"},
		"proxy_id":             "proxy-2",
		"extensions":           []any{map[string]any{"id": "ext-b"}, map[string]any{"id": "ext-a"}},
		"chrome_policy":        map[string]any{"HomepageLocation": "https://new.example"},
		"viewport":             map[string]any{"width": float64(1440), "height": float64(900), "refresh_rate": float64(30)},
		"headless":             false,
		"kiosk_mode":           true,
		"stealth":              true,
		"start_url":            "https://new.example",
		"timeout_seconds":      float64(120),
		"fill_rate_per_minute": float64(0),
	}
	if !jsonEqual(body, want) {
		t.Fatalf("expanded SDK JSON mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
}

func TestExpandUpdateParamsOmitsUnchangedDurableConfig(t *testing.T) {
	model := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringValue("proxy-1"),
		ExtensionIDs:      stringListForTest("ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
		Viewport:          viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Value(60)),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringValue("https://example.com"),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	params, diags := expandUpdateParams(context.Background(), model, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	if len(body) != 0 {
		t.Fatalf("update params = %#v, want empty patch for unchanged config", body)
	}
}

func TestExpandUpdateParamsClearsSupportedDurableConfig(t *testing.T) {
	plan := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringNull(),
		ExtensionIDs:      types.ListNull(types.StringType),
		ChromePolicy:      chromePolicyNull(),
		Viewport:          viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Null()),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringNull(),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}
	state := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringValue("proxy-1"),
		ExtensionIDs:      stringListForTest("ext-a"),
		ChromePolicy:      chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
		Viewport:          plan.Viewport,
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringValue("https://example.com"),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	params, diags := expandUpdateParams(context.Background(), plan, state)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	body := marshalSDKParams(t, params)
	want := map[string]any{
		"proxy_id":      "",
		"extensions":    []any{},
		"chrome_policy": map[string]any{},
		"start_url":     "",
	}
	if !jsonEqual(body, want) {
		t.Fatalf("expanded SDK JSON mismatch\ngot:  %#v\nwant: %#v", body, want)
	}
}

func TestExpandUpdateParamsRejectsUnsupportedClears(t *testing.T) {
	plan := browserPoolModel{
		Name:              types.StringNull(),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringNull(),
		ProxyID:           types.StringNull(),
		ExtensionIDs:      types.ListNull(types.StringType),
		ChromePolicy:      chromePolicyNull(),
		Viewport:          types.ObjectNull(viewportAttrTypes()),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringNull(),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}
	state := browserPoolModel{
		Name:              types.StringValue("pool-a"),
		Size:              types.Int64Value(1),
		ProfileID:         types.StringValue("profile-1"),
		ProxyID:           types.StringNull(),
		ExtensionIDs:      types.ListNull(types.StringType),
		ChromePolicy:      chromePolicyNull(),
		Viewport:          viewportObjectForTest(types.Int64Value(1280), types.Int64Value(800), types.Int64Null()),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		StartURL:          types.StringNull(),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	params, diags := expandUpdateParams(context.Background(), plan, state)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for unsupported clear operations")
	}
	for _, want := range []path.Path{path.Root("name"), path.Root("profile_id"), path.Root("viewport")} {
		if !hasDiagnosticPath(diags, want) {
			t.Fatalf("expected diagnostic at %s, got %v", want.String(), diags)
		}
	}
	assertEmptyUpdateSDKParams(t, params)
}

func TestExpandUpdateParamsRejectsInvalidChromePolicy(t *testing.T) {
	plan := browserPoolModel{
		Size:              types.Int64Value(1),
		ChromePolicy:      chromePolicyValueForTest(`[]`),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}
	state := browserPoolModel{
		Size:              types.Int64Value(1),
		ChromePolicy:      chromePolicyNull(),
		Headless:          types.BoolValue(true),
		KioskMode:         types.BoolValue(false),
		Stealth:           types.BoolValue(false),
		TimeoutSeconds:    types.Int64Value(90),
		FillRatePerMinute: types.Int64Value(10),
	}

	params, diags := expandUpdateParams(context.Background(), plan, state)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid chrome_policy")
	}
	if !hasDiagnosticPath(diags, path.Root("chrome_policy")) {
		t.Fatalf("expected diagnostic at chrome_policy, got %v", diags)
	}
	assertEmptyUpdateSDKParams(t, params)
}

func chromePolicyValueForTest(value string) chromePolicyValue {
	return chromePolicyValue{StringValue: basetypes.NewStringValue(value)}
}

func TestExpandCreateParamsRejectsUnknownExtensionIDElement(t *testing.T) {
	model := browserPoolModel{
		Name: types.StringValue("partial-would-be-set"),
		Size: types.Int64Value(1),
		ExtensionIDs: types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("ext-a"),
			types.StringUnknown(),
		}),
	}

	params, diags := expandCreateParams(context.Background(), model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for an unknown extension_ids element")
	}
	if !hasDiagnosticPath(diags, path.Root("extension_ids").AtListIndex(1)) {
		t.Fatalf("expected diagnostic at extension_ids[1], got %v", diags)
	}
	assertEmptySDKParams(t, params)
}

func stringListForTest(values ...string) types.List {
	attrs := make([]attr.Value, 0, len(values))
	for _, value := range values {
		attrs = append(attrs, types.StringValue(value))
	}
	return types.ListValueMust(types.StringType, attrs)
}

func viewportObjectForTest(width, height, refreshRate types.Int64) types.Object {
	return types.ObjectValueMust(
		map[string]attr.Type{
			"width":        types.Int64Type,
			"height":       types.Int64Type,
			"refresh_rate": types.Int64Type,
		},
		map[string]attr.Value{
			"width":        width,
			"height":       height,
			"refresh_rate": refreshRate,
		},
	)
}

func marshalSDKParams(t *testing.T, params any) map[string]any {
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

func assertEmptySDKParams(t *testing.T, params kernel.BrowserPoolNewParams) {
	t.Helper()

	if !reflect.DeepEqual(params, kernel.BrowserPoolNewParams{}) {
		t.Fatalf("params = %#v, want zero SDK params on diagnostics", params)
	}
}

func assertEmptyUpdateSDKParams(t *testing.T, params kernel.BrowserPoolUpdateParams) {
	t.Helper()

	if !reflect.DeepEqual(params, kernel.BrowserPoolUpdateParams{}) {
		t.Fatalf("params = %#v, want zero SDK params on diagnostics", params)
	}
}

func hasDiagnosticPath(diags diag.Diagnostics, want path.Path) bool {
	for _, diagnostic := range diags {
		withPath, ok := diagnostic.(diag.DiagnosticWithPath)
		if ok && withPath.Path().Equal(want) {
			return true
		}
	}
	return false
}
