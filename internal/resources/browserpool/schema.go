package browserpool

import (
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
				Validators: []validator.String{
					browserPoolNameValidator{},
				},
			},
			"project_id": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project this browser pool belongs to. Defaults to the provider `project_id` when unset; when neither is set, the API key's project binding determines the project. Once created the pool keeps its project, and changing this attribute replaces the pool.",
				PlanModifiers: []planmodifier.String{
					// UseStateForUnknown must run first: it fills the unset
					// (unknown) plan value from state so RequiresReplace
					// compares real values and does not replace pools whose
					// project is inherited.
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					nonEmptyStringValidator{attributeName: "project_id"},
				},
			},
			"size": rschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of browsers to maintain in the pool.",
				Validators: []validator.Int64{
					int64AtLeast(minBrowserPoolSize),
				},
			},
			"profile_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional profile ID to load for browsers created by this pool.",
				Validators: []validator.String{
					nonEmptyStringValidator{attributeName: "profile_id"},
				},
			},
			"proxy_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional proxy ID to use for browsers created by this pool.",
				Validators: []validator.String{
					nonEmptyStringValidator{attributeName: "proxy_id"},
				},
			},
			"extension_ids": rschema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Stable set of extension IDs to load into browsers created by this pool.",
				Validators: []validator.Set{
					nonEmptyStringSetValidator{},
				},
			},
			"chrome_policy": rschema.StringAttribute{
				Optional:            true,
				CustomType:          ChromePolicyType{},
				MarkdownDescription: "Normalized JSON object containing Chrome enterprise policy overrides.",
				Validators: []validator.String{
					chromePolicyJSONValidator{},
				},
			},
			"viewport": rschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Optional browser viewport.",
				Attributes: map[string]rschema.Attribute{
					"width": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window width in pixels.",
						Validators: []validator.Int64{
							int64AtLeast(minViewportDimension),
						},
					},
					"height": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window height in pixels.",
						Validators: []validator.Int64{
							int64AtLeast(minViewportDimension),
						},
					},
					"refresh_rate": rschema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Optional display refresh rate in Hz.",
						Validators: []validator.Int64{
							int64AtLeast(minViewportRefreshRate),
						},
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
				Validators: []validator.String{
					nonEmptyStringValidator{attributeName: "start_url"},
				},
			},
			"timeout_seconds": rschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Default idle timeout in seconds for acquired browsers.",
				Validators: []validator.Int64{
					int64Between(minTimeoutSeconds, maxTimeoutSeconds),
				},
			},
			"fill_rate_per_minute": rschema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Percentage of the pool to fill per minute.",
				Validators: []validator.Int64{
					int64AtLeast(minFillRatePerMinute),
				},
			},
		},
	}
}
