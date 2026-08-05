package browserpool

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/shared"
)

func expandCreateParams(ctx context.Context, model browserPoolModel) (kernel.BrowserPoolNewParams, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Collect every known-value problem before bailing so a plan with several
	// unknown attributes reports them all at once rather than one per apply.
	if model.Size.IsNull() || model.Size.IsUnknown() {
		diags.AddAttributeError(
			path.Root("size"),
			"Missing Browser Pool Size",
			"size must be known before creating a Kernel browser pool.",
		)
	}
	validateCreateKnownValues(&diags, model)
	if diags.HasError() {
		return kernel.BrowserPoolNewParams{}, diags
	}

	params := kernel.BrowserPoolNewParams{
		Size: model.Size.ValueInt64(),
	}

	if isKnownString(model.Name) {
		params.Name = kernel.String(model.Name.ValueString())
	}
	if isKnownString(model.ProfileID) {
		params.Profile.ID = kernel.String(model.ProfileID.ValueString())
	}
	if isKnownBool(model.RefreshOnProfileUpdate) {
		params.RefreshOnProfileUpdate = kernel.Bool(model.RefreshOnProfileUpdate.ValueBool())
	}
	if isKnownString(model.ProxyID) {
		params.ProxyID = kernel.String(model.ProxyID.ValueString())
	}
	if isKnownString(model.ChromePolicy.StringValue) {
		policy, policyDiags := decodeChromePolicyJSON(model.ChromePolicy.ValueString())
		diags.Append(policyDiags...)
		if diags.HasError() {
			return kernel.BrowserPoolNewParams{}, diags
		}
		params.ChromePolicy = policy
	}
	if !model.ExtensionIDs.IsNull() {
		ids, extensionDiags := extensionIDs(ctx, model.ExtensionIDs, "creating")
		diags.Append(extensionDiags...)
		if diags.HasError() {
			return kernel.BrowserPoolNewParams{}, diags
		}
		params.Extensions = extensionParams(ids)
	}
	if !model.Viewport.IsNull() {
		viewport, viewportDiags := expandViewport(ctx, model.Viewport)
		diags.Append(viewportDiags...)
		if diags.HasError() {
			return kernel.BrowserPoolNewParams{}, diags
		}
		params.Viewport = viewport
	}
	if isKnownBool(model.Headless) {
		params.Headless = kernel.Bool(model.Headless.ValueBool())
	}
	if isKnownBool(model.KioskMode) {
		params.KioskMode = kernel.Bool(model.KioskMode.ValueBool())
	}
	if isKnownBool(model.Stealth) {
		params.Stealth = kernel.Bool(model.Stealth.ValueBool())
	}
	if isKnownString(model.StartURL) {
		params.StartURL = kernel.String(model.StartURL.ValueString())
	}
	if isKnownInt64(model.TimeoutSeconds) {
		params.TimeoutSeconds = kernel.Int(model.TimeoutSeconds.ValueInt64())
	}
	if isKnownInt64(model.FillRatePerMinute) {
		params.FillRatePerMinute = kernel.Int(model.FillRatePerMinute.ValueInt64())
	}

	return params, diags
}

func expandUpdateParams(ctx context.Context, plan, state browserPoolModel) (kernel.BrowserPoolUpdateParams, bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Collect every known-value problem before bailing so a plan with several
	// unknown attributes reports them all at once rather than one per apply.
	if plan.Size.IsNull() || plan.Size.IsUnknown() {
		diags.AddAttributeError(
			path.Root("size"),
			"Missing Browser Pool Size",
			"size must be known before updating a Kernel browser pool.",
		)
	}
	validateUpdateKnownValues(&diags, plan)
	if diags.HasError() {
		return kernel.BrowserPoolUpdateParams{}, false, diags
	}

	var params kernel.BrowserPoolUpdateParams
	hasPatch := false
	if isKnownBool(plan.RebuildIdle) && plan.RebuildIdle.ValueBool() && browserLaunchConfigurationChanged(plan, state) {
		// Pool updates change the template for future browsers. Rebuild browsers
		// that are idle now only when the customer explicitly opts into the
		// disruptive replacement behavior.
		params.DiscardAllIdle = kernel.Bool(true)
		hasPatch = true
	}

	if !plan.Name.Equal(state.Name) && isKnownString(plan.Name) {
		params.Name = kernel.String(plan.Name.ValueString())
		hasPatch = true
	}
	if !plan.Size.Equal(state.Size) {
		params.Size = kernel.Int(plan.Size.ValueInt64())
		hasPatch = true
	}
	profileChanged := !plan.ProfileID.Equal(state.ProfileID)
	if profileChanged {
		if plan.ProfileID.IsNull() {
			params.Profile.ID = kernel.String("")
			hasPatch = true
		} else if isKnownString(plan.ProfileID) {
			params.Profile.ID = kernel.String(plan.ProfileID.ValueString())
			hasPatch = true
		}
	}
	// An unknown value on a profile change is omitted so Kernel can choose the applicable default.
	if (!plan.RefreshOnProfileUpdate.Equal(state.RefreshOnProfileUpdate) || profileChanged) && isKnownBool(plan.RefreshOnProfileUpdate) {
		params.RefreshOnProfileUpdate = kernel.Bool(plan.RefreshOnProfileUpdate.ValueBool())
		hasPatch = true
	}
	if !plan.ProxyID.Equal(state.ProxyID) {
		if plan.ProxyID.IsNull() {
			params.ProxyID = kernel.String("")
			hasPatch = true
		} else if isKnownString(plan.ProxyID) {
			params.ProxyID = kernel.String(plan.ProxyID.ValueString())
			hasPatch = true
		}
	}
	if !plan.ExtensionIDs.Equal(state.ExtensionIDs) {
		if plan.ExtensionIDs.IsNull() {
			params.Extensions = []shared.BrowserExtensionParam{}
			hasPatch = true
		} else {
			ids, extensionDiags := extensionIDs(ctx, plan.ExtensionIDs, "updating")
			diags.Append(extensionDiags...)
			if diags.HasError() {
				return kernel.BrowserPoolUpdateParams{}, false, diags
			}
			params.Extensions = extensionParams(ids)
			hasPatch = true
		}
	}
	if !plan.ChromePolicy.Equal(state.ChromePolicy) {
		if plan.ChromePolicy.IsNull() {
			params.ChromePolicy = map[string]any{}
			hasPatch = true
		} else if isKnownString(plan.ChromePolicy.StringValue) {
			policy, policyDiags := decodeChromePolicyJSON(plan.ChromePolicy.ValueString())
			diags.Append(policyDiags...)
			if diags.HasError() {
				return kernel.BrowserPoolUpdateParams{}, false, diags
			}
			params.ChromePolicy = policy
			hasPatch = true
		}
	}
	if browserViewportChanged(plan.Viewport, state.Viewport) && !plan.Viewport.IsNull() {
		viewport, viewportDiags := expandViewport(ctx, plan.Viewport)
		diags.Append(viewportDiags...)
		if diags.HasError() {
			return kernel.BrowserPoolUpdateParams{}, false, diags
		}
		params.Viewport = viewport
		hasPatch = true
	}
	if !plan.Headless.Equal(state.Headless) && isKnownBool(plan.Headless) {
		params.Headless = kernel.Bool(plan.Headless.ValueBool())
		hasPatch = true
	}
	if !plan.KioskMode.Equal(state.KioskMode) && isKnownBool(plan.KioskMode) {
		params.KioskMode = kernel.Bool(plan.KioskMode.ValueBool())
		hasPatch = true
	}
	if !plan.Stealth.Equal(state.Stealth) && isKnownBool(plan.Stealth) {
		params.Stealth = kernel.Bool(plan.Stealth.ValueBool())
		hasPatch = true
	}
	if !plan.StartURL.Equal(state.StartURL) {
		if plan.StartURL.IsNull() {
			params.StartURL = kernel.String("")
			hasPatch = true
		} else if isKnownString(plan.StartURL) {
			params.StartURL = kernel.String(plan.StartURL.ValueString())
			hasPatch = true
		}
	}
	if !plan.TimeoutSeconds.Equal(state.TimeoutSeconds) && isKnownInt64(plan.TimeoutSeconds) {
		params.TimeoutSeconds = kernel.Int(plan.TimeoutSeconds.ValueInt64())
		hasPatch = true
	}
	if !plan.FillRatePerMinute.Equal(state.FillRatePerMinute) && isKnownInt64(plan.FillRatePerMinute) {
		params.FillRatePerMinute = kernel.Int(plan.FillRatePerMinute.ValueInt64())
		hasPatch = true
	}

	return params, hasPatch, diags
}

func browserLaunchConfigurationChanged(plan, state browserPoolModel) bool {
	// Keep raw Equal checks aligned with validateUpdateKnownValues and all fields
	// aligned with the patch builder above. Otherwise an unknown planned value
	// could discard idle browsers without a corresponding configuration patch.
	// Optional+Computed booleans use knownBoolChanged instead.
	return !plan.ProfileID.Equal(state.ProfileID) ||
		!plan.ProxyID.Equal(state.ProxyID) ||
		!plan.ExtensionIDs.Equal(state.ExtensionIDs) ||
		!plan.ChromePolicy.Equal(state.ChromePolicy) ||
		browserViewportChanged(plan.Viewport, state.Viewport) ||
		knownBoolChanged(plan.Headless, state.Headless) ||
		knownBoolChanged(plan.KioskMode, state.KioskMode) ||
		knownBoolChanged(plan.Stealth, state.Stealth) ||
		!plan.StartURL.Equal(state.StartURL)
}

func browserLaunchConfigurationMayChange(plan, state, config browserPoolModel) bool {
	return knownValueChanged(plan.ProfileID, state.ProfileID) ||
		knownValueChanged(plan.ProxyID, state.ProxyID) ||
		knownValueChanged(plan.ExtensionIDs, state.ExtensionIDs) ||
		knownValueChanged(plan.ChromePolicy, state.ChromePolicy) ||
		knownBrowserViewportChanged(plan.Viewport, state.Viewport) ||
		knownBoolChanged(plan.Headless, state.Headless) ||
		knownBoolChanged(plan.KioskMode, state.KioskMode) ||
		knownBoolChanged(plan.Stealth, state.Stealth) ||
		knownValueChanged(plan.StartURL, state.StartURL) ||
		browserLaunchConfigurationUnknown(config)
}

func knownValueChanged(plan, state attr.Value) bool {
	return !plan.IsUnknown() && !plan.Equal(state)
}

func browserLaunchConfigurationUnknown(config browserPoolModel) bool {
	if config.ProfileID.IsUnknown() ||
		config.ProxyID.IsUnknown() ||
		config.ExtensionIDs.IsUnknown() ||
		config.ChromePolicy.IsUnknown() ||
		config.Headless.IsUnknown() ||
		config.KioskMode.IsUnknown() ||
		config.Stealth.IsUnknown() ||
		config.StartURL.IsUnknown() {
		return true
	}
	if config.Viewport.IsNull() {
		return false
	}
	if config.Viewport.IsUnknown() {
		return true
	}
	for _, value := range config.Viewport.Attributes() {
		if value.IsUnknown() {
			return true
		}
	}
	return false
}

func knownBoolChanged(plan, state types.Bool) bool {
	// Optional+Computed values can legitimately be unknown during planning.
	// Unknown means "not decided yet", not "different from state".
	return isKnownBool(plan) && !plan.Equal(state)
}

func browserViewportChanged(plan, state types.Object) bool {
	if plan.IsUnknown() {
		return false
	}
	if plan.IsNull() || state.IsNull() || state.IsUnknown() {
		return !plan.Equal(state)
	}

	planAttrs := plan.Attributes()
	stateAttrs := state.Attributes()
	for _, name := range []string{"width", "height"} {
		if !planAttrs[name].Equal(stateAttrs[name]) {
			return true
		}
	}

	planRefreshRate := planAttrs["refresh_rate"].(types.Int64)
	return !planRefreshRate.IsUnknown() && !planRefreshRate.Equal(stateAttrs["refresh_rate"])
}

func knownBrowserViewportChanged(plan, state types.Object) bool {
	if plan.IsUnknown() {
		return false
	}
	if plan.IsNull() || state.IsNull() || state.IsUnknown() {
		return !plan.Equal(state)
	}

	planAttrs := plan.Attributes()
	stateAttrs := state.Attributes()
	for _, name := range []string{"width", "height", "refresh_rate"} {
		if !planAttrs[name].IsUnknown() && !planAttrs[name].Equal(stateAttrs[name]) {
			return true
		}
	}
	return false
}

func validateCreateKnownValues(diags *diag.Diagnostics, model browserPoolModel) {
	requireKnownOptional(diags, path.Root("name"), model.Name, "creating")
	requireKnownOptional(diags, path.Root("profile_id"), model.ProfileID, "creating")
	requireKnownOptional(diags, path.Root("proxy_id"), model.ProxyID, "creating")
	requireKnownOptional(diags, path.Root("chrome_policy"), model.ChromePolicy, "creating")
	requireKnownOptional(diags, path.Root("extension_ids"), model.ExtensionIDs, "creating")
	requireKnownOptional(diags, path.Root("viewport"), model.Viewport, "creating")
	requireKnownOptional(diags, path.Root("start_url"), model.StartURL, "creating")
}

func validateUpdateKnownValues(diags *diag.Diagnostics, model browserPoolModel) {
	requireKnownOptional(diags, path.Root("name"), model.Name, "updating")
	requireKnownOptional(diags, path.Root("profile_id"), model.ProfileID, "updating")
	requireKnownOptional(diags, path.Root("proxy_id"), model.ProxyID, "updating")
	requireKnownOptional(diags, path.Root("chrome_policy"), model.ChromePolicy, "updating")
	requireKnownOptional(diags, path.Root("extension_ids"), model.ExtensionIDs, "updating")
	requireKnownOptional(diags, path.Root("viewport"), model.Viewport, "updating")
	requireKnownOptional(diags, path.Root("start_url"), model.StartURL, "updating")
	requireKnownOptional(diags, path.Root("rebuild_idle_browsers_on_update"), model.RebuildIdle, "updating")
}

func clearsString(plan, state types.String) bool {
	return plan.IsNull() && isKnownString(state)
}

func requireKnownOptional(diags *diag.Diagnostics, attrPath path.Path, value attr.Value, operation string) {
	if value.IsNull() || !value.IsUnknown() {
		return
	}

	diags.AddAttributeError(
		attrPath,
		"Unknown Browser Pool Value",
		fmt.Sprintf("%s must be known before %s a Kernel browser pool.", attrPath.String(), operation),
	)
}

func extensionIDs(ctx context.Context, list types.List, action string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Guard unknown elements explicitly (mirroring the viewport dimension checks)
	// so the diagnostic names the offending index rather than surfacing a generic
	// framework conversion error from ElementsAs.
	for i, elem := range list.Elements() {
		if elem.IsUnknown() {
			diags.AddAttributeError(
				path.Root("extension_ids").AtListIndex(i),
				"Unknown Browser Pool Extension ID",
				fmt.Sprintf("extension_ids[%d] must be known before %s a Kernel browser pool.", i, action),
			)
		}
	}
	if diags.HasError() {
		return nil, diags
	}

	var ids []string
	diags.Append(list.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return ids, diags
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
	var model viewportModel

	diags.Append(value.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return shared.BrowserViewportParam{}, diags
	}

	if model.Width.IsNull() || model.Width.IsUnknown() {
		diags.AddAttributeError(
			path.Root("viewport").AtName("width"),
			"Missing Browser Pool Viewport Width",
			"viewport.width must be known before configuring a Kernel browser pool.",
		)
	}
	if model.Height.IsNull() || model.Height.IsUnknown() {
		diags.AddAttributeError(
			path.Root("viewport").AtName("height"),
			"Missing Browser Pool Viewport Height",
			"viewport.height must be known before configuring a Kernel browser pool.",
		)
	}
	if diags.HasError() {
		return shared.BrowserViewportParam{}, diags
	}

	viewport := shared.BrowserViewportParam{
		Width:  model.Width.ValueInt64(),
		Height: model.Height.ValueInt64(),
	}
	if isKnownInt64(model.RefreshRate) {
		viewport.RefreshRate = kernel.Int(model.RefreshRate.ValueInt64())
	}

	return viewport, diags
}

func decodeChromePolicyJSON(input string) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics

	policy, err := decodeChromePolicy(input)
	if err != nil {
		diags.AddAttributeError(
			path.Root("chrome_policy"),
			chromePolicyInvalidJSONSummary,
			chromePolicyInvalidSyntaxDetail+err.Error(),
		)
		return nil, diags
	}

	if policy == nil {
		diags.AddAttributeError(
			path.Root("chrome_policy"),
			chromePolicyInvalidJSONSummary,
			chromePolicyNotObjectDetail,
		)
		return nil, diags
	}

	return policy, diags
}

func isKnownString(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func isKnownBool(value types.Bool) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func isKnownInt64(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown()
}
