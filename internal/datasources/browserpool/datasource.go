package browserpool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/shared"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ datasource.DataSource              = (*browserPoolDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*browserPoolDataSource)(nil)
)

const (
	minBrowserPoolTimeoutSeconds = 10
	maxBrowserPoolTimeoutSeconds = 259200
	minBrowserPoolFillRate       = 0
	minBrowserPoolViewportValue  = 1
)

type browserPoolClient interface {
	DefaultProjectID() string
	GetBrowserPool(context.Context, string, string) (*kernel.BrowserPool, error)
}

type browserPoolDataSource struct {
	client browserPoolClient
}

type browserPoolModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	ProjectID         types.String `tfsdk:"project_id"`
	Size              types.Int64  `tfsdk:"size"`
	ProfileID         types.String `tfsdk:"profile_id"`
	ExtensionIDs      types.List   `tfsdk:"extension_ids"`
	ProxyID           types.String `tfsdk:"proxy_id"`
	Headless          types.Bool   `tfsdk:"headless"`
	KioskMode         types.Bool   `tfsdk:"kiosk_mode"`
	Stealth           types.Bool   `tfsdk:"stealth"`
	StartURL          types.String `tfsdk:"start_url"`
	TimeoutSeconds    types.Int64  `tfsdk:"timeout_seconds"`
	FillRatePerMinute types.Int64  `tfsdk:"fill_rate_per_minute"`
	Viewport          types.Object `tfsdk:"viewport"`
	ChromePolicy      types.String `tfsdk:"chrome_policy"`
}

func NewDataSource() datasource.DataSource {
	return &browserPoolDataSource{}
}

func newDataSourceWithClient(client browserPoolClient) *browserPoolDataSource {
	return &browserPoolDataSource{client: client}
}

func (d *browserPoolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_browser_pool"
}

func (d *browserPoolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup durable Kernel browser pool configuration.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Browser pool ID.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Browser pool name for exact lookup.",
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the browser pool up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"size": dschema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of browsers maintained in the pool.",
			},
			"profile_id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved profile ID attached to the pool, if any.",
			},
			"extension_ids": dschema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Resolved extension IDs attached to the pool, in load order.",
			},
			"proxy_id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Proxy ID attached to browsers in the pool, if any.",
			},
			"headless": dschema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether browsers use a headless image.",
			},
			"kiosk_mode": dschema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether browsers launch in kiosk mode.",
			},
			"stealth": dschema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether browsers launch in stealth mode.",
			},
			"start_url": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL opened when a browser is warmed into the pool, if configured.",
			},
			"timeout_seconds": dschema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Default idle timeout in seconds for acquired browsers.",
			},
			"fill_rate_per_minute": dschema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Percentage of the pool filled per minute.",
			},
			"viewport": dschema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Browser viewport configured for the pool, if any.",
				Attributes: map[string]dschema.Attribute{
					"width":        dschema.Int64Attribute{Computed: true, MarkdownDescription: "Browser window width in pixels."},
					"height":       dschema.Int64Attribute{Computed: true, MarkdownDescription: "Browser window height in pixels."},
					"refresh_rate": dschema.Int64Attribute{Computed: true, MarkdownDescription: "Display refresh rate in Hz, if configured."},
				},
			},
			"chrome_policy": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Normalized JSON object of Chrome enterprise policy overrides, if configured.",
			},
		},
	}
}

func (d *browserPoolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(browserPoolClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the browser pool data source durable client contract.",
		)
		return
	}
	d.client = client
}

func (d *browserPoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config browserPoolModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, diags := d.read(ctx, config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (d *browserPoolDataSource) read(ctx context.Context, config browserPoolModel) (browserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError("Missing Kernel Client", "The browser pool data source was not configured with a Kernel client.")
		return browserPoolModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("Browser Pool", "kernel_browser_pool", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	var idOrName string
	switch {
	case selector.HasID:
		idOrName = config.ID.ValueString()
	case selector.HasName:
		idOrName = config.Name.ValueString()
	default:
		diags.AddError("Missing Browser Pool Selector", "Configure id or name for kernel_browser_pool.")
		return browserPoolModel{}, diags
	}

	pool, err := d.client.GetBrowserPool(ctx, projectID, idOrName)
	if err != nil {
		projectscope.AddError(&diags, "Read Kernel Browser Pool", projectID, err)
		return browserPoolModel{}, diags
	}
	if pool == nil {
		diags.AddError("Read Kernel Browser Pool", "Kernel returned an empty browser pool response.")
		return browserPoolModel{}, diags
	}

	state, flattenDiags := flattenBrowserPool(*pool)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return browserPoolModel{}, diags
	}
	if selector.HasID && state.ID.ValueString() != config.ID.ValueString() {
		diags.AddError(
			"Browser Pool ID Mismatch",
			"Kernel returned browser pool "+strconv.Quote(state.ID.ValueString())+" for id selector "+strconv.Quote(config.ID.ValueString())+".",
		)
		return browserPoolModel{}, diags
	}
	if selector.HasName && (state.Name.IsNull() || state.Name.ValueString() != config.Name.ValueString()) {
		diags.AddError(
			"Browser Pool Name Mismatch",
			"Kernel returned a browser pool whose name does not match exact selector "+strconv.Quote(config.Name.ValueString())+".",
		)
		return browserPoolModel{}, diags
	}

	state.ProjectID = config.ProjectID
	return state, diags
}

func flattenBrowserPool(pool kernel.BrowserPool) (browserPoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !datasources.ValidResponseString(pool.JSON.ID.Raw(), pool.JSON.ID.Valid(), pool.ID) {
		datasources.AddInvalidResponseField(&diags, "Browser Pool", "id")
	}
	if !validResponseInt64(pool.BrowserPoolConfig.JSON.Size.Raw(), pool.BrowserPoolConfig.JSON.Size.Valid(), pool.BrowserPoolConfig.Size) || pool.BrowserPoolConfig.Size < 1 {
		datasources.AddInvalidResponseField(&diags, "Browser Pool", "browser_pool_config.size")
	}

	name := types.StringNull()
	switch {
	case datasources.FieldPresent(pool.JSON.Name.Raw()):
		if !datasources.ValidResponseString(pool.JSON.Name.Raw(), pool.JSON.Name.Valid(), pool.Name) {
			datasources.AddInvalidResponseField(&diags, "Browser Pool", "name")
		} else {
			name = types.StringValue(pool.Name)
		}
	case datasources.FieldPresent(pool.BrowserPoolConfig.JSON.Name.Raw()):
		if !datasources.ValidResponseString(pool.BrowserPoolConfig.JSON.Name.Raw(), pool.BrowserPoolConfig.JSON.Name.Valid(), pool.BrowserPoolConfig.Name) {
			datasources.AddInvalidResponseField(&diags, "Browser Pool", "browser_pool_config.name")
		} else {
			name = types.StringValue(pool.BrowserPoolConfig.Name)
		}
	}
	if diags.HasError() {
		return browserPoolModel{}, diags
	}

	config := pool.BrowserPoolConfig
	return browserPoolModel{
		ID:                types.StringValue(pool.ID),
		Name:              name,
		Size:              types.Int64Value(config.Size),
		ProfileID:         flattenResolvedProfileID(pool, &diags),
		ExtensionIDs:      flattenResolvedExtensionIDs(pool, &diags),
		ProxyID:           flattenOptionalString("browser_pool_config.proxy_id", config.JSON.ProxyID.Raw(), config.JSON.ProxyID.Valid(), config.ProxyID, &diags),
		Headless:          flattenOptionalBool("browser_pool_config.headless", config.JSON.Headless.Raw(), config.JSON.Headless.Valid(), config.Headless, &diags),
		KioskMode:         flattenOptionalBool("browser_pool_config.kiosk_mode", config.JSON.KioskMode.Raw(), config.JSON.KioskMode.Valid(), config.KioskMode, &diags),
		Stealth:           flattenOptionalBool("browser_pool_config.stealth", config.JSON.Stealth.Raw(), config.JSON.Stealth.Valid(), config.Stealth, &diags),
		StartURL:          flattenOptionalString("browser_pool_config.start_url", config.JSON.StartURL.Raw(), config.JSON.StartURL.Valid(), config.StartURL, &diags),
		TimeoutSeconds:    flattenTimeoutSeconds(config.JSON.TimeoutSeconds.Raw(), config.JSON.TimeoutSeconds.Valid(), config.TimeoutSeconds, &diags),
		FillRatePerMinute: flattenFillRatePerMinute(config.JSON.FillRatePerMinute.Raw(), config.JSON.FillRatePerMinute.Valid(), config.FillRatePerMinute, &diags),
		Viewport:          flattenViewport(config.JSON.Viewport.Raw(), config.JSON.Viewport.Valid(), config.Viewport, &diags),
		ChromePolicy:      flattenChromePolicy(config.JSON.ChromePolicy.Raw(), config.JSON.ChromePolicy.Valid(), &diags),
	}, diags
}

func flattenOptionalString(field, raw string, valid bool, value string, diags *diag.Diagnostics) types.String {
	if raw == "" {
		return types.StringNull()
	}
	if !datasources.ValidResponseString(raw, valid, value) {
		datasources.AddInvalidResponseField(diags, "Browser Pool", field)
		return types.StringNull()
	}
	return types.StringValue(value)
}

func flattenOptionalBool(field, raw string, valid bool, value bool, diags *diag.Diagnostics) types.Bool {
	if raw == "" {
		return types.BoolNull()
	}
	if !validResponseBool(raw, valid, value) {
		datasources.AddInvalidResponseField(diags, "Browser Pool", field)
		return types.BoolNull()
	}
	return types.BoolValue(value)
}

func flattenTimeoutSeconds(raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	result := flattenOptionalInt64("browser_pool_config.timeout_seconds", raw, valid, value, diags)
	if result.IsNull() {
		return result
	}
	if value < minBrowserPoolTimeoutSeconds || value > maxBrowserPoolTimeoutSeconds {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.timeout_seconds")
		return types.Int64Null()
	}
	return result
}

func flattenFillRatePerMinute(raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	result := flattenOptionalInt64("browser_pool_config.fill_rate_per_minute", raw, valid, value, diags)
	if result.IsNull() {
		return result
	}
	if value < minBrowserPoolFillRate {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.fill_rate_per_minute")
		return types.Int64Null()
	}
	return result
}

func flattenOptionalInt64(field, raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	if raw == "" {
		return types.Int64Null()
	}
	if !validResponseInt64(raw, valid, value) {
		datasources.AddInvalidResponseField(diags, "Browser Pool", field)
		return types.Int64Null()
	}
	return types.Int64Value(value)
}

func flattenViewport(raw string, valid bool, viewport shared.BrowserViewport, diags *diag.Diagnostics) types.Object {
	if raw == "" {
		return types.ObjectNull(viewportAttributeTypes())
	}
	if !datasources.FieldPresent(raw) || !valid {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.viewport")
		return types.ObjectNull(viewportAttributeTypes())
	}

	diagnosticsBefore := len(*diags)
	width := flattenRequiredPositiveInt64("browser_pool_config.viewport.width", viewport.JSON.Width.Raw(), viewport.JSON.Width.Valid(), viewport.Width, diags)
	height := flattenRequiredPositiveInt64("browser_pool_config.viewport.height", viewport.JSON.Height.Raw(), viewport.JSON.Height.Valid(), viewport.Height, diags)
	refreshRate := types.Int64Null()
	if viewport.JSON.RefreshRate.Raw() != "" {
		refreshRate = flattenRequiredPositiveInt64("browser_pool_config.viewport.refresh_rate", viewport.JSON.RefreshRate.Raw(), viewport.JSON.RefreshRate.Valid(), viewport.RefreshRate, diags)
	}
	if len(*diags) > diagnosticsBefore {
		return types.ObjectNull(viewportAttributeTypes())
	}

	return types.ObjectValueMust(viewportAttributeTypes(), map[string]attr.Value{
		"width":        width,
		"height":       height,
		"refresh_rate": refreshRate,
	})
}

func flattenRequiredPositiveInt64(field, raw string, valid bool, value int64, diags *diag.Diagnostics) types.Int64 {
	if !validResponseInt64(raw, valid, value) || value < minBrowserPoolViewportValue {
		datasources.AddInvalidResponseField(diags, "Browser Pool", field)
		return types.Int64Null()
	}
	return types.Int64Value(value)
}

func viewportAttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"width":        types.Int64Type,
		"height":       types.Int64Type,
		"refresh_rate": types.Int64Type,
	}
}

func flattenChromePolicy(raw string, valid bool, diags *diag.Diagnostics) types.String {
	if raw == "" || !datasources.FieldPresent(raw) {
		return types.StringNull()
	}
	if !valid {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.chrome_policy")
		return types.StringNull()
	}

	var policy map[string]any
	if err := json.Unmarshal([]byte(raw), &policy); err != nil || policy == nil {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.chrome_policy")
		return types.StringNull()
	}

	var normalized bytes.Buffer
	encoder := json.NewEncoder(&normalized)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(policy); err != nil {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.chrome_policy")
		return types.StringNull()
	}
	return types.StringValue(strings.TrimSuffix(normalized.String(), "\n"))
}

func flattenResolvedProfileID(pool kernel.BrowserPool, diags *diag.Diagnostics) types.String {
	raw := pool.JSON.ProfileID.Raw()
	if raw != "" {
		if !datasources.FieldPresent(raw) {
			datasources.AddInvalidResponseField(diags, "Browser Pool", "profile_id")
			return types.StringNull()
		}
		if !datasources.ValidResponseString(raw, pool.JSON.ProfileID.Valid(), pool.ProfileID) {
			datasources.AddInvalidResponseField(diags, "Browser Pool", "profile_id")
			return types.StringNull()
		}
		return types.StringValue(pool.ProfileID)
	}

	profile := pool.BrowserPoolConfig.Profile
	if !datasources.FieldPresent(pool.BrowserPoolConfig.JSON.Profile.Raw()) {
		return types.StringNull()
	}
	if !datasources.ValidResponseString(profile.JSON.ID.Raw(), profile.JSON.ID.Valid(), profile.ID) {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.profile.id")
		return types.StringNull()
	}
	return types.StringValue(profile.ID)
}

func flattenResolvedExtensionIDs(pool kernel.BrowserPool, diags *diag.Diagnostics) types.List {
	raw := pool.JSON.ExtensionIDs.Raw()
	if raw != "" {
		return flattenStringList("extension_ids", raw, pool.JSON.ExtensionIDs.Valid(), pool.ExtensionIDs, diags)
	}

	config := pool.BrowserPoolConfig
	if !datasources.FieldPresent(config.JSON.Extensions.Raw()) {
		return types.ListValueMust(types.StringType, nil)
	}
	return flattenExtensionIDs(config.JSON.Extensions.Valid(), config.Extensions, diags)
}

func flattenStringList(field, raw string, valid bool, values []string, diags *diag.Diagnostics) types.List {
	var decoded []string
	if !datasources.FieldPresent(raw) || !valid || json.Unmarshal([]byte(raw), &decoded) != nil || len(decoded) != len(values) {
		datasources.AddInvalidResponseField(diags, "Browser Pool", field)
		return types.ListNull(types.StringType)
	}

	elements := make([]attr.Value, 0, len(values))
	for index, value := range values {
		if value == "" || decoded[index] != value {
			datasources.AddInvalidResponseField(diags, "Browser Pool", fmt.Sprintf("%s[%d]", field, index))
			return types.ListNull(types.StringType)
		}
		elements = append(elements, types.StringValue(value))
	}
	return types.ListValueMust(types.StringType, elements)
}

func flattenExtensionIDs(valid bool, extensions []shared.BrowserExtension, diags *diag.Diagnostics) types.List {
	if !valid {
		datasources.AddInvalidResponseField(diags, "Browser Pool", "browser_pool_config.extensions")
		return types.ListNull(types.StringType)
	}

	elements := make([]attr.Value, 0, len(extensions))
	for index, extension := range extensions {
		if !datasources.ValidResponseString(extension.JSON.ID.Raw(), extension.JSON.ID.Valid(), extension.ID) {
			datasources.AddInvalidResponseField(diags, "Browser Pool", fmt.Sprintf("browser_pool_config.extensions[%d].id", index))
			return types.ListNull(types.StringType)
		}
		elements = append(elements, types.StringValue(extension.ID))
	}
	return types.ListValueMust(types.StringType, elements)
}

func validResponseInt64(raw string, valid bool, value int64) bool {
	if !datasources.FieldPresent(raw) || !valid {
		return false
	}
	var decoded int64
	return json.Unmarshal([]byte(raw), &decoded) == nil && decoded == value
}

func validResponseBool(raw string, valid bool, value bool) bool {
	if !datasources.FieldPresent(raw) || !valid {
		return false
	}
	var decoded bool
	return json.Unmarshal([]byte(raw), &decoded) == nil && decoded == value
}
