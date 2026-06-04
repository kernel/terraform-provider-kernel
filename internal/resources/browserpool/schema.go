package browserpool

import (
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func BrowserPoolSchema() rschema.Schema {
	return rschema.Schema{
		MarkdownDescription: "Kernel browser pool durable configuration.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique browser pool identifier.",
			},
			"name": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional browser pool name. Must be unique within the project.",
			},
			"size": rschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of browsers to maintain in the pool.",
			},
			"profile_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional profile ID to load for browsers created by this pool.",
			},
			"profile_save_changes": rschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Persist browser changes back to the configured profile when supported by the profile.",
			},
			"proxy_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional proxy ID to use for browsers created by this pool.",
			},
			"extension_ids": rschema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Stable set of extension IDs to load into browsers created by this pool.",
			},
			"chrome_policy": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Normalized JSON object containing Chrome enterprise policy overrides.",
			},
			"viewport": rschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Optional browser viewport.",
				Attributes: map[string]rschema.Attribute{
					"width": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window width in pixels.",
					},
					"height": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window height in pixels.",
					},
					"refresh_rate": rschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Optional display refresh rate in Hz.",
					},
				},
			},
			"headless": rschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Launch browsers using a headless image.",
			},
			"kiosk_mode": rschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Launch browsers in kiosk mode.",
			},
			"stealth": rschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Launch browsers in stealth mode.",
			},
			"start_url": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional URL to navigate to when a browser is warmed into the pool.",
			},
			"timeout_seconds": rschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Default idle timeout in seconds for acquired browsers.",
			},
			"fill_rate_per_minute": rschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Percentage of the pool to fill per minute.",
			},
		},
	}
}
