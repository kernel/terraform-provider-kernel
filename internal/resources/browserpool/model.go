package browserpool

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type BrowserPoolModel struct {
	ID                types.String      `tfsdk:"id"`
	Name              types.String      `tfsdk:"name"`
	ProjectID         types.String      `tfsdk:"project_id"`
	Size              types.Int64       `tfsdk:"size"`
	ProfileID         types.String      `tfsdk:"profile_id"`
	ProxyID           types.String      `tfsdk:"proxy_id"`
	ExtensionIDs      types.Set         `tfsdk:"extension_ids"`
	ChromePolicy      ChromePolicyValue `tfsdk:"chrome_policy"`
	Viewport          types.Object      `tfsdk:"viewport"`
	Headless          types.Bool        `tfsdk:"headless"`
	KioskMode         types.Bool        `tfsdk:"kiosk_mode"`
	Stealth           types.Bool        `tfsdk:"stealth"`
	StartURL          types.String      `tfsdk:"start_url"`
	TimeoutSeconds    types.Int64       `tfsdk:"timeout_seconds"`
	FillRatePerMinute types.Int64       `tfsdk:"fill_rate_per_minute"`
}

type ViewportModel struct {
	Width       types.Int64 `tfsdk:"width"`
	Height      types.Int64 `tfsdk:"height"`
	RefreshRate types.Int64 `tfsdk:"refresh_rate"`
}

func viewportAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"width":        types.Int64Type,
		"height":       types.Int64Type,
		"refresh_rate": types.Int64Type,
	}
}
