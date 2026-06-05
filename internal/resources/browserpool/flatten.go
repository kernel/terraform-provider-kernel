package browserpool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/shared"
)

func flattenBrowserPool(pool kernel.BrowserPool, base browserPoolModel) (browserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !validResponseString(pool.JSON.ID.Raw(), pool.JSON.ID.Valid(), pool.ID) {
		addInvalidResponseDiagnostic(&diags, "id")
	}
	config := pool.BrowserPoolConfig
	if !validResponseInt64(config.JSON.Size.Raw(), config.JSON.Size.Valid(), config.Size) || config.Size < minBrowserPoolSize {
		addInvalidResponseDiagnostic(&diags, "browser_pool_config.size")
	}
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	model := browserPoolModel{
		ID:                types.StringValue(pool.ID),
		Name:              flattenName(pool, &diags),
		Size:              types.Int64Value(config.Size),
		ProfileID:         types.StringNull(),
		ProxyID:           flattenString("browser_pool_config.proxy_id", config.JSON.ProxyID.Raw(), config.JSON.ProxyID.Valid(), config.ProxyID, &diags),
		ExtensionIDs:      omittedExtensionIDs(config.JSON.Extensions.Raw(), base.ExtensionIDs),
		ChromePolicy:      omittedChromePolicy(config.JSON.ChromePolicy.Raw(), base.ChromePolicy),
		Viewport:          types.ObjectNull(viewportAttrTypes()),
		Headless:          flattenBool("browser_pool_config.headless", config.JSON.Headless.Raw(), config.JSON.Headless.Valid(), config.Headless, &diags),
		KioskMode:         flattenBool("browser_pool_config.kiosk_mode", config.JSON.KioskMode.Raw(), config.JSON.KioskMode.Valid(), config.KioskMode, &diags),
		Stealth:           flattenBool("browser_pool_config.stealth", config.JSON.Stealth.Raw(), config.JSON.Stealth.Valid(), config.Stealth, &diags),
		StartURL:          flattenString("browser_pool_config.start_url", config.JSON.StartURL.Raw(), config.JSON.StartURL.Valid(), config.StartURL, &diags),
		TimeoutSeconds:    flattenTimeoutSeconds(config.JSON.TimeoutSeconds.Raw(), config.JSON.TimeoutSeconds.Valid(), config.TimeoutSeconds, &diags),
		FillRatePerMinute: flattenFillRatePerMinute(config.JSON.FillRatePerMinute.Raw(), config.JSON.FillRatePerMinute.Valid(), config.FillRatePerMinute, &diags),
	}

	if responseFieldPresent(config.JSON.Profile.Raw()) {
		model.ProfileID = flattenProfileID(config.Profile, &diags)
	}
	if responseFieldPresent(config.JSON.Extensions.Raw()) {
		model.ExtensionIDs = flattenExtensionIDs(config.JSON.Extensions.Valid(), config.Extensions, &diags)
	}
	if responseFieldPresent(config.JSON.ChromePolicy.Raw()) {
		model.ChromePolicy = flattenChromePolicy(config.JSON.ChromePolicy.Raw(), &diags)
	}
	if responseFieldPresent(config.JSON.Viewport.Raw()) {
		model.Viewport = flattenViewport(config.Viewport, &diags)
	}

	if diags.HasError() {
		return browserPoolModel{}, diags
	}
	return model, diags
}

func flattenName(pool kernel.BrowserPool, diags *diag.Diagnostics) types.String {
	if responseFieldPresent(pool.JSON.Name.Raw()) {
		return flattenString("name", pool.JSON.Name.Raw(), pool.JSON.Name.Valid(), pool.Name, diags)
	}

	config := pool.BrowserPoolConfig
	return flattenString("browser_pool_config.name", config.JSON.Name.Raw(), config.JSON.Name.Valid(), config.Name, diags)
}

func flattenString(field, raw string, valid bool, value string, diags *diag.Diagnostics) types.String {
	if !responseFieldPresent(raw) {
		return types.StringNull()
	}
	if !validResponseString(raw, valid, value) {
		addInvalidResponseDiagnostic(diags, field)
		return types.StringNull()
	}

	return types.StringValue(value)
}

func flattenBool(field, raw string, valid bool, value bool, diags *diag.Diagnostics) types.Bool {
	if !responseFieldPresent(raw) {
		return types.BoolNull()
	}
	if !validResponseBool(raw, valid, value) {
		addInvalidResponseDiagnostic(diags, field)
		return types.BoolNull()
	}

	return types.BoolValue(value)
}

func flattenInt64(field, raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	if !responseFieldPresent(raw) {
		return types.Int64Null()
	}
	if !validResponseInt64(raw, valid, value) {
		addInvalidResponseDiagnostic(diags, field)
		return types.Int64Null()
	}

	return types.Int64Value(value)
}

func flattenTimeoutSeconds(raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	result := flattenInt64("browser_pool_config.timeout_seconds", raw, valid, value, diags)
	if result.IsNull() {
		return result
	}
	if value < minTimeoutSeconds || value > maxTimeoutSeconds {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.timeout_seconds")
		return types.Int64Null()
	}
	return result
}

func flattenFillRatePerMinute(raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	result := flattenInt64("browser_pool_config.fill_rate_per_minute", raw, valid, value, diags)
	if result.IsNull() {
		return result
	}
	if value < minFillRatePerMinute {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.fill_rate_per_minute")
		return types.Int64Null()
	}
	return result
}

func flattenProfileID(profile shared.BrowserProfile, diags *diag.Diagnostics) types.String {
	if !validResponseString(profile.JSON.ID.Raw(), profile.JSON.ID.Valid(), profile.ID) {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.profile.id")
		return types.StringNull()
	}
	return types.StringValue(profile.ID)
}

func flattenExtensionIDs(valid bool, extensions []shared.BrowserExtension, diags *diag.Diagnostics) types.List {
	if !valid {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.extensions")
		return types.ListNull(types.StringType)
	}

	values := make([]attr.Value, 0, len(extensions))
	for index, extension := range extensions {
		if !validResponseString(extension.JSON.ID.Raw(), extension.JSON.ID.Valid(), extension.ID) {
			addInvalidResponseDiagnostic(diags, fmt.Sprintf("browser_pool_config.extensions[%d].id", index))
			return types.ListNull(types.StringType)
		}
		values = append(values, types.StringValue(extension.ID))
	}

	list, listDiags := types.ListValue(types.StringType, values)
	diags.Append(listDiags...)
	return list
}

func omittedExtensionIDs(raw string, base types.List) types.List {
	if raw != "" || base.IsNull() || base.IsUnknown() || len(base.Elements()) != 0 {
		return types.ListNull(types.StringType)
	}
	return types.ListValueMust(types.StringType, nil)
}

func flattenChromePolicy(raw string, diags *diag.Diagnostics) chromePolicyValue {
	normalized, policyDiags := normalizeChromePolicyJSON(raw)
	diags.Append(policyDiags...)
	if policyDiags.HasError() {
		return chromePolicyNull()
	}
	return chromePolicyValue{StringValue: types.StringValue(normalized)}
}

func omittedChromePolicy(raw string, base chromePolicyValue) chromePolicyValue {
	if raw != "" || base.IsNull() || base.IsUnknown() {
		return chromePolicyNull()
	}

	normalized, diags := normalizeChromePolicyJSON(base.ValueString())
	if diags.HasError() || normalized != "{}" {
		return chromePolicyNull()
	}

	return chromePolicyValue{StringValue: types.StringValue("{}")}
}

func flattenViewport(viewport shared.BrowserViewport, diags *diag.Diagnostics) types.Object {
	valid := true
	if !validResponseInt64(viewport.JSON.Width.Raw(), viewport.JSON.Width.Valid(), viewport.Width) || viewport.Width < minViewportDimension {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.viewport.width")
		valid = false
	}
	if !validResponseInt64(viewport.JSON.Height.Raw(), viewport.JSON.Height.Valid(), viewport.Height) || viewport.Height < minViewportDimension {
		addInvalidResponseDiagnostic(diags, "browser_pool_config.viewport.height")
		valid = false
	}

	refreshRate := types.Int64Null()
	if responseFieldPresent(viewport.JSON.RefreshRate.Raw()) {
		if !validResponseInt64(viewport.JSON.RefreshRate.Raw(), viewport.JSON.RefreshRate.Valid(), viewport.RefreshRate) || viewport.RefreshRate < minViewportRefreshRate {
			addInvalidResponseDiagnostic(diags, "browser_pool_config.viewport.refresh_rate")
			valid = false
		} else {
			refreshRate = types.Int64Value(viewport.RefreshRate)
		}
	}
	if !valid {
		return types.ObjectNull(viewportAttrTypes())
	}

	return types.ObjectValueMust(viewportAttrTypes(), map[string]attr.Value{
		"width":        types.Int64Value(viewport.Width),
		"height":       types.Int64Value(viewport.Height),
		"refresh_rate": refreshRate,
	})
}

func responseFieldPresent(raw string) bool {
	return raw != "" && strings.TrimSpace(raw) != "null"
}

func validResponseString(raw string, valid bool, value string) bool {
	if !responseFieldPresent(raw) || !valid || value == "" {
		return false
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func validResponseBool(raw string, valid bool, value bool) bool {
	if !responseFieldPresent(raw) || !valid {
		return false
	}

	var decoded bool
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func validResponseInt64(raw string, valid bool, value int64) bool {
	if !responseFieldPresent(raw) || !valid {
		return false
	}

	var decoded int64
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func chromePolicyNull() chromePolicyValue {
	return chromePolicyValue{StringValue: types.StringNull()}
}

func viewportAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"width":        types.Int64Type,
		"height":       types.Int64Type,
		"refresh_rate": types.Int64Type,
	}
}

func addInvalidResponseDiagnostic(diags *diag.Diagnostics, field string) {
	diags.AddError(
		"Invalid Kernel Browser Pool Response",
		fmt.Sprintf("Kernel API response omitted or returned invalid field %q; Terraform cannot safely flatten it into state.", field),
	)
}
