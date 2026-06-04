package browserpool

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/shared"
)

func ExpandCreateParams(ctx context.Context, model BrowserPoolModel) (kernel.BrowserPoolNewParams, diag.Diagnostics) {
	var diags diag.Diagnostics

	params := kernel.BrowserPoolNewParams{
		Size: model.Size.ValueInt64(),
	}

	profileIDKnown := isKnownNonEmpty(model.ProfileID)
	profileSaveChangesKnown := isKnownBool(model.ProfileSaveChanges)
	if profileSaveChangesKnown && model.ProfileSaveChanges.ValueBool() && !profileIDKnown {
		diags.AddError(
			"Invalid Profile Configuration",
			"profile_save_changes requires profile_id so Kernel knows which profile should receive saved browser changes.",
		)
		return params, diags
	}

	if !model.Name.IsNull() && !model.Name.IsUnknown() {
		params.Name = kernel.String(model.Name.ValueString())
	}
	if profileIDKnown {
		params.Profile.ID = kernel.String(model.ProfileID.ValueString())
		if profileSaveChangesKnown {
			params.Profile.SaveChanges = kernel.Bool(model.ProfileSaveChanges.ValueBool())
		}
	}
	if !model.ProxyID.IsNull() && !model.ProxyID.IsUnknown() {
		params.ProxyID = kernel.String(model.ProxyID.ValueString())
	}
	if !model.ChromePolicy.IsNull() && !model.ChromePolicy.IsUnknown() {
		policy, policyDiags := decodeChromePolicyJSON(model.ChromePolicy.ValueString())
		diags.Append(policyDiags...)
		if diags.HasError() {
			return params, diags
		}
		params.ChromePolicy = policy
	}
	if !model.ExtensionIDs.IsNull() && !model.ExtensionIDs.IsUnknown() {
		ids, setDiags := extensionIDs(ctx, model.ExtensionIDs)
		diags.Append(setDiags...)
		if diags.HasError() {
			return params, diags
		}
		params.Extensions = extensionParams(ids)
	}
	if !model.Viewport.IsNull() && !model.Viewport.IsUnknown() {
		viewport, viewportDiags := expandViewport(ctx, model.Viewport)
		diags.Append(viewportDiags...)
		if diags.HasError() {
			return params, diags
		}
		params.Viewport = viewport
	}
	if !model.Headless.IsNull() && !model.Headless.IsUnknown() {
		params.Headless = kernel.Bool(model.Headless.ValueBool())
	}
	if !model.KioskMode.IsNull() && !model.KioskMode.IsUnknown() {
		params.KioskMode = kernel.Bool(model.KioskMode.ValueBool())
	}
	if !model.Stealth.IsNull() && !model.Stealth.IsUnknown() {
		params.Stealth = kernel.Bool(model.Stealth.ValueBool())
	}
	if !model.StartURL.IsNull() && !model.StartURL.IsUnknown() {
		params.StartURL = kernel.String(model.StartURL.ValueString())
	}
	if !model.TimeoutSeconds.IsNull() && !model.TimeoutSeconds.IsUnknown() {
		params.TimeoutSeconds = kernel.Int(model.TimeoutSeconds.ValueInt64())
	}
	if !model.FillRatePerMinute.IsNull() && !model.FillRatePerMinute.IsUnknown() {
		params.FillRatePerMinute = kernel.Int(model.FillRatePerMinute.ValueInt64())
	}

	return params, diags
}

func FlattenBrowserPool(pool kernel.BrowserPool, prior BrowserPoolModel) (BrowserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	config := pool.BrowserPoolConfig

	model := BrowserPoolModel{
		ID:   types.StringValue(pool.ID),
		Size: types.Int64Value(config.Size),
	}

	model.Name = flattenName(pool, prior.Name)
	model.ProfileID = prior.ProfileID
	model.ProfileSaveChanges = prior.ProfileSaveChanges
	if config.JSON.Profile.Valid() {
		model.ProfileID = flattenOptionalString(config.Profile.JSON.ID.Valid(), config.Profile.ID, prior.ProfileID)
		model.ProfileSaveChanges = flattenOptionalBool(config.Profile.JSON.SaveChanges.Valid(), config.Profile.SaveChanges, prior.ProfileSaveChanges)
	}
	model.ProxyID = flattenOptionalString(config.JSON.ProxyID.Valid(), config.ProxyID, prior.ProxyID)
	model.ChromePolicy = prior.ChromePolicy
	if config.JSON.ChromePolicy.Valid() {
		model.ChromePolicy = flattenChromePolicy(config.JSON.ChromePolicy.Raw(), prior.ChromePolicy)
	}
	model.ExtensionIDs = prior.ExtensionIDs
	if config.JSON.Extensions.Valid() {
		model.ExtensionIDs = flattenExtensionIDs(config.Extensions, prior.ExtensionIDs)
	}
	model.Viewport = prior.Viewport
	if config.JSON.Viewport.Valid() && !prior.Viewport.IsNull() {
		model.Viewport = flattenViewport(config.Viewport, prior.Viewport)
	}
	model.Headless = flattenOptionalBool(config.JSON.Headless.Valid(), config.Headless, prior.Headless)
	model.KioskMode = flattenOptionalBool(config.JSON.KioskMode.Valid(), config.KioskMode, prior.KioskMode)
	model.Stealth = flattenOptionalBool(config.JSON.Stealth.Valid(), config.Stealth, prior.Stealth)
	model.StartURL = flattenOptionalString(config.JSON.StartURL.Valid(), config.StartURL, prior.StartURL)
	model.TimeoutSeconds = flattenOptionalInt64(config.JSON.TimeoutSeconds.Valid(), config.TimeoutSeconds, prior.TimeoutSeconds)
	model.FillRatePerMinute = flattenOptionalInt64(config.JSON.FillRatePerMinute.Valid(), config.FillRatePerMinute, prior.FillRatePerMinute)

	return model, diags
}

func extensionIDs(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var ids []string

	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil, diags
	}
	for _, id := range ids {
		if id == "" {
			diags.AddError(
				"Invalid Extension ID",
				"extension_ids cannot contain empty strings.",
			)
			return nil, diags
		}
	}

	return stableUniqueStrings(ids), diags
}

func extensionParams(ids []string) []shared.BrowserExtensionParam {
	params := make([]shared.BrowserExtensionParam, 0, len(ids))
	for _, id := range ids {
		params = append(params, shared.BrowserExtensionParam{ID: kernel.String(id)})
	}
	return params
}

func expandViewport(ctx context.Context, value types.Object) (shared.BrowserViewportParam, diag.Diagnostics) {
	var diags diag.Diagnostics
	var model ViewportModel

	diags.Append(value.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return shared.BrowserViewportParam{}, diags
	}

	viewport := shared.BrowserViewportParam{
		Width:  model.Width.ValueInt64(),
		Height: model.Height.ValueInt64(),
	}
	if !model.RefreshRate.IsNull() && !model.RefreshRate.IsUnknown() {
		viewport.RefreshRate = kernel.Int(model.RefreshRate.ValueInt64())
	}

	return viewport, diags
}

func flattenOptionalString(valid bool, value string, prior types.String) types.String {
	if !valid {
		return prior
	}
	if value == "" && prior.IsNull() {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func flattenName(pool kernel.BrowserPool, prior types.String) types.String {
	if pool.JSON.Name.Valid() {
		return flattenOptionalString(true, pool.Name, prior)
	}

	config := pool.BrowserPoolConfig
	return flattenOptionalString(config.JSON.Name.Valid(), config.Name, prior)
}

func flattenOptionalBool(valid bool, value bool, prior types.Bool) types.Bool {
	if !valid {
		return prior
	}
	if !value && prior.IsNull() {
		return types.BoolNull()
	}
	return types.BoolValue(value)
}

func flattenOptionalInt64(valid bool, value int64, prior types.Int64) types.Int64 {
	if !valid {
		return prior
	}
	if prior.IsNull() {
		return types.Int64Null()
	}
	return types.Int64Value(value)
}

func flattenChromePolicy(rawPolicy string, prior types.String) types.String {
	normalized, diags := NormalizeChromePolicyJSON(rawPolicy)
	if diags.HasError() {
		return types.StringNull()
	}
	if normalized == "{}" && prior.IsNull() {
		return types.StringNull()
	}

	if !prior.IsNull() && !prior.IsUnknown() {
		priorNormalized, diags := NormalizeChromePolicyJSON(prior.ValueString())
		if !diags.HasError() && priorNormalized == normalized {
			return prior
		}
	}

	return types.StringValue(normalized)
}

func flattenExtensionIDs(extensions []shared.BrowserExtension, prior types.Set) types.Set {
	if len(extensions) == 0 && prior.IsNull() {
		return types.SetNull(types.StringType)
	}

	ids := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		if extension.ID != "" {
			ids = append(ids, extension.ID)
		}
	}

	return extensionIDsSet(ids...)
}

func flattenViewport(viewport shared.BrowserViewport, prior types.Object) types.Object {
	if prior.IsNull() {
		return types.ObjectNull(viewportAttrTypes())
	}

	refreshRate := priorRefreshRate(prior)
	if viewport.JSON.RefreshRate.Valid() {
		refreshRate = types.Int64Value(viewport.RefreshRate)
	}
	if priorRefreshRate, ok := prior.Attributes()["refresh_rate"].(types.Int64); ok && priorRefreshRate.IsNull() {
		refreshRate = types.Int64Null()
	}

	return viewportValueWithRefreshRate(viewport.Width, viewport.Height, refreshRate)
}

func priorRefreshRate(viewport types.Object) types.Int64 {
	refreshRate, ok := viewport.Attributes()["refresh_rate"].(types.Int64)
	if !ok {
		return types.Int64Null()
	}
	return refreshRate
}

func stableUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func extensionIDsSet(ids ...string) types.Set {
	ids = stableUniqueStrings(ids)
	values := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.StringValue(id))
	}
	return types.SetValueMust(types.StringType, values)
}

func viewportValue(width, height, refreshRate int64) types.Object {
	refreshRateValue := types.Int64Null()
	if refreshRate != 0 {
		refreshRateValue = types.Int64Value(refreshRate)
	}

	return viewportValueWithRefreshRate(width, height, refreshRateValue)
}

func viewportValueWithRefreshRate(width, height int64, refreshRate types.Int64) types.Object {
	attrs := map[string]attr.Value{
		"width":        types.Int64Value(width),
		"height":       types.Int64Value(height),
		"refresh_rate": refreshRate,
	}

	return types.ObjectValueMust(viewportAttrTypes(), attrs)
}

func isKnownBool(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func isKnownNonEmpty(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}
