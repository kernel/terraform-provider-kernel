package proxy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/packages/respjson"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ datasource.DataSource              = (*proxyDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*proxyDataSource)(nil)
)

type proxyClient interface {
	DefaultProjectID() string
	GetProxy(context.Context, string, string) (*kernel.ProxyGetResponse, error)
	ListProxyPage(context.Context, string, int64) (kernelclient.ProxyPage, error)
}

type proxyDataSource struct {
	client proxyClient
}

type proxyModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ProjectID types.String `tfsdk:"project_id"`
	Type      types.String `tfsdk:"type"`
	Protocol  types.String `tfsdk:"protocol"`
}

type proxyFields struct {
	id        string
	name      string
	protocol  string
	proxyType string

	idField       respjson.Field
	nameField     respjson.Field
	protocolField respjson.Field
	typeField     respjson.Field
}

func NewDataSource() datasource.DataSource {
	return &proxyDataSource{}
}

func newDataSourceWithClient(client proxyClient) *proxyDataSource {
	return &proxyDataSource{client: client}
}

func (d *proxyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_proxy"
}

func (d *proxyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup durable Kernel proxy metadata.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Proxy ID.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Proxy name for exact lookup.",
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the proxy up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"type": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Proxy type as reported by the API, e.g. `datacenter`, `isp`, `residential`, `mobile`, or `custom`.",
			},
			"protocol": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Proxy protocol as reported by the API, e.g. `http` or `https`.",
			},
		},
	}
}

func (d *proxyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(proxyClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the proxy data source durable client contract.",
		)
		return
	}

	d.client = client
}

func (d *proxyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config proxyModel
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

func (d *proxyDataSource) read(ctx context.Context, config proxyModel) (proxyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The proxy data source was not configured with a Kernel client.",
		)
		return proxyModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("Proxy", "kernel_proxy", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return proxyModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return proxyModel{}, diags
	}

	var state proxyModel
	var readDiags diag.Diagnostics
	switch {
	case selector.HasID:
		state, readDiags = d.get(ctx, projectID, config.ID.ValueString())
	case selector.HasName:
		state, readDiags = d.lookupName(ctx, projectID, config.Name.ValueString())
	default:
		diags.AddError(
			"Missing Proxy Selector",
			"Configure id or name for kernel_proxy.",
		)
		return proxyModel{}, diags
	}

	diags.Append(readDiags...)
	if diags.HasError() {
		return proxyModel{}, diags
	}

	state.ProjectID = config.ProjectID
	return state, diags
}

func (d *proxyDataSource) get(ctx context.Context, projectID, id string) (proxyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	proxy, err := d.client.GetProxy(ctx, projectID, id)
	if err != nil {
		projectscope.AddError(&diags, "Read Kernel Proxy", projectID, err)
		return proxyModel{}, diags
	}
	if proxy == nil {
		diags.AddError("Read Kernel Proxy", "Kernel returned an empty proxy response.")
		return proxyModel{}, diags
	}

	state, flattenDiags := flattenProxy(*proxy)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return proxyModel{}, diags
	}
	if state.ID.ValueString() != id {
		diags.AddError(
			"Proxy ID Mismatch",
			"Kernel returned proxy "+state.ID.ValueString()+" for id selector "+id+". Use the name selector for name lookups.",
		)
		return proxyModel{}, diags
	}
	return state, diags
}

func (d *proxyDataSource) lookupName(ctx context.Context, projectID, name string) (proxyModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	proxy, count := d.findProxiesByName(ctx, projectID, name, &diags)
	if diags.HasError() {
		return proxyModel{}, diags
	}

	switch count {
	case 0:
		diags.AddError(
			"Lookup Kernel Proxy",
			"No Kernel proxy found with exact name "+name+".",
		)
		return proxyModel{}, diags
	case 1:
		return flattenListedProxy(*proxy)
	default:
		diags.AddError(
			"Ambiguous Kernel Proxy Name",
			"Found multiple Kernel proxies with exact name "+name+". Configure id instead.",
		)
		return proxyModel{}, diags
	}
}

func (d *proxyDataSource) findProxiesByName(ctx context.Context, projectID, name string, diags *diag.Diagnostics) (*kernel.ProxyListResponse, int) {
	var match *kernel.ProxyListResponse
	count := 0
	offset := int64(0)
	seen := map[string]bool{}

	for {
		page, err := d.client.ListProxyPage(ctx, projectID, offset)
		if err != nil {
			projectscope.AddError(diags, "Lookup Kernel Proxy", projectID, err)
			return nil, 0
		}

		for _, proxy := range page.Items {
			// Filter on the decoded name first: the list is unfiltered, so
			// unrelated rows (including ones with absent or malformed names)
			// come back and must be skipped, not abort the lookup. Only rows
			// claiming the requested name get raw-consistency validation.
			if proxy.Name != name {
				continue
			}
			if !datasources.ValidResponseString(proxy.JSON.Name.Raw(), proxy.JSON.Name.Valid(), proxy.Name) {
				datasources.AddInvalidResponseField(diags, "Proxy", "name")
				return nil, 0
			}
			// Dedupe by id: offset pagination can repeat a row across pages
			// when the list shifts mid-scan, and a repeated row must not be
			// mistaken for a second proxy with the same name.
			if seen[proxy.ID] {
				continue
			}
			seen[proxy.ID] = true
			count++
			if match == nil {
				matched := proxy
				match = &matched
			}
		}

		if !page.HasNextPage {
			break
		}
		offset = page.NextOffset
	}

	return match, count
}

func flattenProxy(proxy kernel.ProxyGetResponse) (proxyModel, diag.Diagnostics) {
	return flattenProxyFields(proxyFields{
		id:            proxy.ID,
		name:          proxy.Name,
		protocol:      string(proxy.Protocol),
		proxyType:     string(proxy.Type),
		idField:       proxy.JSON.ID,
		nameField:     proxy.JSON.Name,
		protocolField: proxy.JSON.Protocol,
		typeField:     proxy.JSON.Type,
	})
}

func flattenListedProxy(proxy kernel.ProxyListResponse) (proxyModel, diag.Diagnostics) {
	return flattenProxyFields(proxyFields{
		id:            proxy.ID,
		name:          proxy.Name,
		protocol:      string(proxy.Protocol),
		proxyType:     string(proxy.Type),
		idField:       proxy.JSON.ID,
		nameField:     proxy.JSON.Name,
		protocolField: proxy.JSON.Protocol,
		typeField:     proxy.JSON.Type,
	})
}

func flattenProxyFields(fields proxyFields) (proxyModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !datasources.ValidResponseString(fields.idField.Raw(), fields.idField.Valid(), fields.id) {
		datasources.AddInvalidResponseField(&diags, "Proxy", "id")
	}
	if !datasources.ValidResponseString(fields.typeField.Raw(), fields.typeField.Valid(), fields.proxyType) {
		datasources.AddInvalidResponseField(&diags, "Proxy", "type")
	}
	if !datasources.ValidResponseString(fields.protocolField.Raw(), fields.protocolField.Valid(), fields.protocol) {
		datasources.AddInvalidResponseField(&diags, "Proxy", "protocol")
	}
	if diags.HasError() {
		return proxyModel{}, diags
	}

	name := types.StringNull()
	if datasources.FieldPresent(fields.nameField.Raw()) {
		if !datasources.ValidResponseString(fields.nameField.Raw(), fields.nameField.Valid(), fields.name) {
			datasources.AddInvalidResponseField(&diags, "Proxy", "name")
		} else {
			name = types.StringValue(fields.name)
		}
	}
	if diags.HasError() {
		return proxyModel{}, diags
	}

	return proxyModel{
		ID:       types.StringValue(fields.id),
		Name:     name,
		Type:     types.StringValue(fields.proxyType),
		Protocol: types.StringValue(fields.protocol),
	}, diags
}
