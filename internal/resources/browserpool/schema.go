package browserpool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional browser pool name. Must be unique within the project. Removing an existing name replaces the pool because the API cannot clear it in place.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(requiresReplaceOnStringClear, "Removing the configured value replaces the browser pool.", "Removing the configured value replaces the browser pool."),
				},
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
					stringvalidator.LengthAtLeast(1),
				},
			},
			"size": rschema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Number of browsers to maintain in the pool.",
				Validators: []validator.Int64{
					int64validator.AtLeast(minBrowserPoolSize),
				},
			},
			"profile_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional profile ID to load for browsers created by this pool. Removing an existing profile ID clears the profile in place.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"refresh_on_profile_update": rschema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "When true, idle browsers are refreshed when the pool's profile is updated. Requires `profile_id` to be set.",
				Validators: []validator.Bool{
					refreshOnProfileUpdateValidator{},
				},
			},
			"proxy_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional proxy ID to use for browsers created by this pool.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"extension_ids": rschema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Ordered extension IDs to load into browsers created by this pool. For an existing pool, omission preserves the current extensions; set an empty list to clear them.",
				PlanModifiers: []planmodifier.List{
					defaultEmptyExtensionIDsOnCreate{},
					listplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.List{
					listvalidator.SizeAtMost(maxBrowserPoolExtensions),
					listvalidator.NoNullValues(),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"chrome_policy": rschema.StringAttribute{
				Optional:            true,
				CustomType:          chromePolicyType{},
				MarkdownDescription: "JSON object of Chrome enterprise policy overrides. Stored as written; key order and whitespace are ignored when detecting changes.",
				PlanModifiers: []planmodifier.String{
					preserveEquivalentChromePolicy{},
				},
				Validators: []validator.String{
					chromePolicyJSONValidator{},
				},
			},
			"viewport": rschema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Optional browser viewport. Removing an existing viewport replaces the pool because the API cannot clear it in place.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplaceIf(requiresReplaceOnObjectClear, "Removing the configured value replaces the browser pool.", "Removing the configured value replaces the browser pool."),
				},
				Attributes: map[string]rschema.Attribute{
					"width": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window width in pixels.",
						Validators: []validator.Int64{
							int64validator.AtLeast(minViewportDimension),
						},
					},
					"height": rschema.Int64Attribute{
						Required:            true,
						MarkdownDescription: "Browser window height in pixels.",
						Validators: []validator.Int64{
							int64validator.AtLeast(minViewportDimension),
						},
					},
					"refresh_rate": rschema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Optional display refresh rate in Hz.",
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseNonNullStateForUnknown(),
						},
						Validators: []validator.Int64{
							int64validator.AtLeast(minViewportRefreshRate),
						},
					},
				},
			},
			"headless": rschema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Launch browsers using a headless image.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"kiosk_mode": rschema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Launch browsers in kiosk mode.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"stealth": rschema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Launch browsers in stealth mode.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"start_url": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional URL to navigate to when a browser is warmed into the pool.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.LengthAtMost(maxStartURLBytes),
				},
			},
			"timeout_seconds": rschema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Default idle timeout in seconds for acquired browsers.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int64{
					int64validator.Between(minTimeoutSeconds, maxTimeoutSeconds),
				},
			},
			"fill_rate_per_minute": rschema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Percentage of the pool to fill per minute. The maximum is determined by the Kernel organization.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(minFillRatePerMinute),
				},
			},
			"rebuild_idle_browsers_on_update": rschema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When true, changes to profile_id, proxy_id, extension_ids, chrome_policy, viewport, headless, kiosk_mode, stealth, or start_url discard browsers that are idle when the update runs so replacements use the new configuration. Browsers that are warming or currently leased are not rebuilt. Kernel does not store this provider-local setting, so imported browser pools default to false unless configured otherwise.",
			},
		},
	}
}

func requiresReplaceOnStringClear(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = req.PlanValue.IsNull() && !req.StateValue.IsNull() && !req.StateValue.IsUnknown()
}

func requiresReplaceOnObjectClear(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = req.PlanValue.IsNull() && !req.StateValue.IsNull() && !req.StateValue.IsUnknown()
}
