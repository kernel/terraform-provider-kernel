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

	if model.Size.IsNull() || model.Size.IsUnknown() {
		diags.AddAttributeError(
			path.Root("size"),
			"Missing Browser Pool Size",
			"size must be known before creating a Kernel browser pool.",
		)
		return kernel.BrowserPoolNewParams{}, diags
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
		ids, extensionDiags := extensionIDs(ctx, model.ExtensionIDs)
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

func validateCreateKnownValues(diags *diag.Diagnostics, model browserPoolModel) {
	requireKnownOptional(diags, path.Root("name"), model.Name)
	requireKnownOptional(diags, path.Root("profile_id"), model.ProfileID)
	requireKnownOptional(diags, path.Root("proxy_id"), model.ProxyID)
	requireKnownOptional(diags, path.Root("chrome_policy"), model.ChromePolicy)
	requireKnownOptional(diags, path.Root("extension_ids"), model.ExtensionIDs)
	requireKnownOptional(diags, path.Root("viewport"), model.Viewport)
	requireKnownOptional(diags, path.Root("start_url"), model.StartURL)
}

func requireKnownOptional(diags *diag.Diagnostics, attrPath path.Path, value attr.Value) {
	if value.IsNull() || !value.IsUnknown() {
		return
	}

	diags.AddAttributeError(
		attrPath,
		"Unknown Browser Pool Value",
		fmt.Sprintf("%s must be known before creating a Kernel browser pool.", attrPath.String()),
	)
}

func extensionIDs(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Guard unknown elements explicitly (mirroring the viewport dimension checks)
	// so the diagnostic names the offending index rather than surfacing a generic
	// framework conversion error from ElementsAs.
	for i, elem := range list.Elements() {
		if elem.IsUnknown() {
			diags.AddAttributeError(
				path.Root("extension_ids").AtListIndex(i),
				"Unknown Browser Pool Extension ID",
				fmt.Sprintf("extension_ids[%d] must be known before creating a Kernel browser pool.", i),
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
			"viewport.width must be known before creating a Kernel browser pool.",
		)
	}
	if model.Height.IsNull() || model.Height.IsUnknown() {
		diags.AddAttributeError(
			path.Root("viewport").AtName("height"),
			"Missing Browser Pool Viewport Height",
			"viewport.height must be known before creating a Kernel browser pool.",
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
			"Invalid Chrome Policy JSON",
			"chrome_policy must be valid JSON object syntax: "+err.Error(),
		)
		return nil, diags
	}

	if policy == nil {
		diags.AddAttributeError(
			path.Root("chrome_policy"),
			"Invalid Chrome Policy JSON",
			"chrome_policy must be a JSON object.",
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
