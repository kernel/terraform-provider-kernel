package browserpool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

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

type browserPoolClient interface {
	DefaultProjectID() string
	GetBrowserPool(context.Context, string, string) (*kernel.BrowserPool, error)
}

type browserPoolDataSource struct {
	client browserPoolClient
}

type browserPoolModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ProjectID    types.String `tfsdk:"project_id"`
	Size         types.Int64  `tfsdk:"size"`
	ProfileID    types.String `tfsdk:"profile_id"`
	ExtensionIDs types.List   `tfsdk:"extension_ids"`
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

	return browserPoolModel{
		ID:           types.StringValue(pool.ID),
		Name:         name,
		Size:         types.Int64Value(pool.BrowserPoolConfig.Size),
		ProfileID:    flattenResolvedProfileID(pool, &diags),
		ExtensionIDs: flattenResolvedExtensionIDs(pool, &diags),
	}, diags
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
