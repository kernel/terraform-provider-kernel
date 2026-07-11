package browserpool

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestFlattenBrowserPoolMapsDurableState(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"name": "top-level-name",
		"profile_id": "profile-resolved",
		"extension_ids": ["ext-resolved-b", "ext-resolved-a"],
		"acquired_count": 2,
		"available_count": 3,
		"browser_pool_config": {
			"size": 5,
			"name": "config-name",
			"profile": {"name": "profile-selector"},
			"proxy_id": "proxy-1",
			"extensions": [{"name": "extension-b"}, {"name": "extension-a"}],
			"chrome_policy": {
				"RestoreOnStartup": 4,
				"HomepageLocation": "https://example.com"
			},
			"viewport": {
				"width": 1280,
				"height": 800,
				"refresh_rate": 60
			},
			"headless": true,
			"kiosk_mode": true,
			"stealth": false,
			"start_url": "https://start.example",
			"timeout_seconds": 90,
			"fill_rate_per_minute": 20
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.ID.ValueString() != "pool-1" {
		t.Fatalf("id = %q, want pool-1", got.ID.ValueString())
	}
	if got.Name.ValueString() != "top-level-name" {
		t.Fatalf("name = %q, want top-level-name", got.Name.ValueString())
	}
	if got.Size.ValueInt64() != 5 {
		t.Fatalf("size = %d, want 5", got.Size.ValueInt64())
	}
	if got.ProfileID.ValueString() != "profile-resolved" {
		t.Fatalf("profile_id = %q, want profile-resolved", got.ProfileID.ValueString())
	}
	if got.ProxyID.ValueString() != "proxy-1" {
		t.Fatalf("proxy_id = %q, want proxy-1", got.ProxyID.ValueString())
	}
	assertStringList(t, got.ExtensionIDs, []string{"ext-resolved-b", "ext-resolved-a"})
	if got.ChromePolicy.ValueString() != `{"HomepageLocation":"https://example.com","RestoreOnStartup":4}` {
		t.Fatalf("chrome_policy = %q, want normalized JSON", got.ChromePolicy.ValueString())
	}
	assertViewport(t, got.Viewport, 1280, 800, types.Int64Value(60))
	if !got.Headless.ValueBool() {
		t.Fatal("headless = false, want true")
	}
	if !got.KioskMode.ValueBool() {
		t.Fatal("kiosk_mode = false, want true")
	}
	if got.Stealth.ValueBool() {
		t.Fatal("stealth = true, want false")
	}
	if got.StartURL.ValueString() != "https://start.example" {
		t.Fatalf("start_url = %q, want https://start.example", got.StartURL.ValueString())
	}
	if got.TimeoutSeconds.ValueInt64() != 90 {
		t.Fatalf("timeout_seconds = %d, want 90", got.TimeoutSeconds.ValueInt64())
	}
	if got.FillRatePerMinute.ValueInt64() != 20 {
		t.Fatalf("fill_rate_per_minute = %d, want 20", got.FillRatePerMinute.ValueInt64())
	}
}

func TestFlattenBrowserPoolUsesLegacySelectorsWhenResolvedFieldsAreOmitted(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"profile": {"id": "profile-legacy"},
			"extensions": [{"id": "extension-b"}, {"id": "extension-a"}]
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.ProfileID.ValueString() != "profile-legacy" {
		t.Fatalf("profile_id = %q, want profile-legacy", got.ProfileID.ValueString())
	}
	assertStringList(t, got.ExtensionIDs, []string{"extension-b", "extension-a"})
}

func TestFlattenBrowserPoolUsesConfigNameWhenTopLevelNameOmitted(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"name": "config-name"
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.Name.ValueString() != "config-name" {
		t.Fatalf("name = %q, want config-name", got.Name.ValueString())
	}
}

func TestFlattenBrowserPoolUsesConfigNameWhenTopLevelNameNull(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"name": null,
		"browser_pool_config": {
			"size": 1,
			"name": "config-name"
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.Name.ValueString() != "config-name" {
		t.Fatalf("name = %q, want config-name", got.Name.ValueString())
	}
}

func TestFlattenBrowserPoolNullsOmittedOptionalFields(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"extension_ids": [],
		"browser_pool_config": {
			"size": 1
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertStringNull(t, "name", got.Name)
	assertStringNull(t, "profile_id", got.ProfileID)
	assertStringNull(t, "proxy_id", got.ProxyID)
	if !got.ExtensionIDs.IsNull() || got.ExtensionIDs.ElementType(t.Context()) != types.StringType {
		t.Fatalf("extension_ids = %#v, want typed string list null", got.ExtensionIDs)
	}
	if !got.ChromePolicy.IsNull() {
		t.Fatalf("chrome_policy = %#v, want null", got.ChromePolicy)
	}
	if !got.Viewport.IsNull() || len(got.Viewport.AttributeTypes(t.Context())) != 3 {
		t.Fatalf("viewport = %#v, want typed object null", got.Viewport)
	}
	if !got.Headless.IsNull() {
		t.Fatalf("headless = %#v, want null", got.Headless)
	}
	if !got.KioskMode.IsNull() {
		t.Fatalf("kiosk_mode = %#v, want null", got.KioskMode)
	}
	if !got.Stealth.IsNull() {
		t.Fatalf("stealth = %#v, want null", got.Stealth)
	}
	assertStringNull(t, "start_url", got.StartURL)
	if !got.TimeoutSeconds.IsNull() {
		t.Fatalf("timeout_seconds = %#v, want null", got.TimeoutSeconds)
	}
	if !got.FillRatePerMinute.IsNull() {
		t.Fatalf("fill_rate_per_minute = %#v, want null", got.FillRatePerMinute)
	}
}

func TestFlattenBrowserPoolPreservesExplicitEmptyConfigWhenResolvedIDsAreEmpty(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"extension_ids": [],
		"browser_pool_config": {
			"size": 1
		}
	}`)
	base := browserPoolModel{
		ExtensionIDs: stringListForTest(),
		ChromePolicy: chromePolicyValueForTest(`{}`),
	}

	got, diags := flattenBrowserPool(pool, base)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertStringList(t, got.ExtensionIDs, []string{})
	if got.ChromePolicy.ValueString() != `{}` {
		t.Fatalf("chrome_policy = %q, want explicit empty object preserved", got.ChromePolicy.ValueString())
	}
}

func TestFlattenBrowserPoolDoesNotPreserveExplicitEmptyConfigWhenAPIReturnsNull(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"extensions": null,
			"chrome_policy": null
		}
	}`)
	base := browserPoolModel{
		ExtensionIDs: stringListForTest(),
		ChromePolicy: chromePolicyValueForTest(`{}`),
	}

	got, diags := flattenBrowserPool(pool, base)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.ExtensionIDs.IsNull() {
		t.Fatalf("extension_ids = %#v, want null for explicit API null", got.ExtensionIDs)
	}
	if !got.ChromePolicy.IsNull() {
		t.Fatalf("chrome_policy = %#v, want null for explicit API null", got.ChromePolicy)
	}
}

func TestFlattenBrowserPoolDoesNotPreserveNonEmptyConfigWhenAPIOmitsIt(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1
		}
	}`)
	base := browserPoolModel{
		ExtensionIDs: stringListForTest("ext-1"),
		ChromePolicy: chromePolicyValueForTest(`{"HomepageLocation":"https://example.com"}`),
	}

	got, diags := flattenBrowserPool(pool, base)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.ExtensionIDs.IsNull() {
		t.Fatalf("extension_ids = %#v, want null when non-empty config is omitted by API", got.ExtensionIDs)
	}
	if !got.ChromePolicy.IsNull() {
		t.Fatalf("chrome_policy = %#v, want null when non-empty config is omitted by API", got.ChromePolicy)
	}
}

func TestFlattenBrowserPoolRejectsMissingRequiredFields(t *testing.T) {
	tests := map[string]string{
		"missing id": `{
			"browser_pool_config": {
				"size": 1
			}
		}`,
		"numeric id": `{
			"id": 123,
			"browser_pool_config": {
				"size": 1
			}
		}`,
		"missing size": `{
			"id": "pool-1",
			"browser_pool_config": {}
		}`,
		"string size": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": "1"
			}
		}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := flattenBrowserPool(unmarshalBrowserPool(t, data), browserPoolModel{})
			if !diags.HasError() {
				t.Fatal("expected diagnostics for invalid required response field")
			}
		})
	}
}

func TestFlattenBrowserPoolRejectsProfileResponseWithoutID(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"profile": {
				"name": "profile-name"
			}
		}
	}`)

	_, diags := flattenBrowserPool(pool, browserPoolModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for profile response without id")
	}
}

func TestFlattenBrowserPoolRejectsExtensionResponseWithoutID(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"extensions": [
				{"name": "extension-name"}
			]
		}
	}`)

	_, diags := flattenBrowserPool(pool, browserPoolModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for extension response without id")
	}
}

func TestFlattenBrowserPoolRejectsInvalidResolvedReferenceFields(t *testing.T) {
	tests := map[string]string{
		"profile id is null": `{
			"id": "pool-1",
			"profile_id": null,
			"browser_pool_config": {
				"size": 1,
				"profile": {"id": "profile-fallback"}
			}
		}`,
		"profile id has wrong type": `{
			"id": "pool-1",
			"profile_id": 123,
			"browser_pool_config": {
				"size": 1,
				"profile": {"id": "profile-fallback"}
			}
		}`,
		"extension ids have wrong type": `{
			"id": "pool-1",
			"extension_ids": {},
			"browser_pool_config": {
				"size": 1,
				"extensions": [{"id": "extension-fallback"}]
			}
		}`,
		"extension ids are null": `{
			"id": "pool-1",
			"extension_ids": null,
			"browser_pool_config": {
				"size": 1,
				"extensions": [{"id": "extension-fallback"}]
			}
		}`,
		"extension id is empty": `{
			"id": "pool-1",
			"extension_ids": [""],
			"browser_pool_config": {
				"size": 1
			}
		}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := flattenBrowserPool(unmarshalBrowserPool(t, data), browserPoolModel{})
			if !diags.HasError() {
				t.Fatal("expected diagnostics for invalid resolved reference field")
			}
		})
	}
}

func TestFlattenBrowserPoolRejectsInvalidScalarResponseFields(t *testing.T) {
	tests := map[string]string{
		"string": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"proxy_id": {}
			}
		}`,
		"bool": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"headless": "true"
			}
		}`,
		"int64": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"timeout_seconds": {}
			}
		}`,
		"coerced int64": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"timeout_seconds": "90"
			}
		}`,
		"coerced bool": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"headless": "true"
			}
		}`,
		"empty string": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"proxy_id": ""
			}
		}`,
		"timeout too low": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"timeout_seconds": 9
			}
		}`,
		"timeout too high": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"timeout_seconds": 259201
			}
		}`,
		"fill rate negative": `{
			"id": "pool-1",
			"browser_pool_config": {
				"size": 1,
				"fill_rate_per_minute": -1
			}
		}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, diags := flattenBrowserPool(unmarshalBrowserPool(t, data), browserPoolModel{})
			if !diags.HasError() {
				t.Fatal("expected diagnostics for invalid scalar response field")
			}
		})
	}
}

func TestFlattenBrowserPoolRejectsInvalidExtensionsResponseField(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"extensions": {}
		}
	}`)

	_, diags := flattenBrowserPool(pool, browserPoolModel{})
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid extensions response field")
	}
}

func TestFlattenBrowserPoolViewportRefreshRateCanBeOmitted(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"viewport": {
				"width": 1280,
				"height": 800
			}
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	assertViewport(t, got.Viewport, 1280, 800, types.Int64Null())
}

func TestFlattenBrowserPoolKeepsEmptyDurableCollectionsWhenEchoed(t *testing.T) {
	pool := unmarshalBrowserPool(t, `{
		"id": "pool-1",
		"browser_pool_config": {
			"size": 1,
			"extensions": [],
			"chrome_policy": {}
		}
	}`)

	got, diags := flattenBrowserPool(pool, browserPoolModel{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	assertStringList(t, got.ExtensionIDs, []string{})
	if got.ChromePolicy.ValueString() != `{}` {
		t.Fatalf("chrome_policy = %q, want empty JSON object", got.ChromePolicy.ValueString())
	}
}

func unmarshalBrowserPool(t *testing.T, data string) kernel.BrowserPool {
	t.Helper()

	var pool kernel.BrowserPool
	if err := json.Unmarshal([]byte(data), &pool); err != nil {
		t.Fatalf("unmarshal browser pool: %v", err)
	}
	return pool
}

func assertStringList(t *testing.T, list types.List, want []string) {
	t.Helper()

	var got []string
	diags := list.ElementsAs(t.Context(), &got, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading string list: %v", diags)
	}
	if len(got) != len(want) {
		t.Fatalf("list length = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("list[%d] = %q, want %q; got %#v", i, got[i], want[i], got)
		}
	}
}

func assertViewport(t *testing.T, viewport types.Object, width, height int64, refreshRate types.Int64) {
	t.Helper()

	attrs := viewport.Attributes()
	if attrs["width"].(types.Int64).ValueInt64() != width {
		t.Fatalf("viewport.width = %#v, want %d", attrs["width"], width)
	}
	if attrs["height"].(types.Int64).ValueInt64() != height {
		t.Fatalf("viewport.height = %#v, want %d", attrs["height"], height)
	}
	if !attrs["refresh_rate"].(types.Int64).Equal(refreshRate) {
		t.Fatalf("viewport.refresh_rate = %#v, want %#v", attrs["refresh_rate"], refreshRate)
	}
}

func assertStringNull(t *testing.T, name string, value types.String) {
	t.Helper()

	if !value.IsNull() {
		t.Fatalf("%s = %#v, want null", name, value)
	}
}
