package extension

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ datasource.DataSource              = (*extensionDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*extensionDataSource)(nil)
)

type extensionClient interface {
	DefaultProjectID() string
	GetExtension(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

type extensionDataSource struct {
	client extensionClient
}

type extensionModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ProjectID types.String `tfsdk:"project_id"`
	CreatedAt types.String `tfsdk:"created_at"`
	SizeBytes types.Int64  `tfsdk:"size_bytes"`
}

func NewDataSource() datasource.DataSource {
	return &extensionDataSource{}
}

func newDataSourceWithClient(client extensionClient) *extensionDataSource {
	return &extensionDataSource{client: client}
}

func (d *extensionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_extension"
}

func (d *extensionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup durable Kernel extension metadata.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Extension ID.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Extension name for exact lookup.",
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the extension up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"created_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Extension creation timestamp.",
			},
			"size_bytes": dschema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Extension archive size in bytes.",
			},
		},
	}
}

func (d *extensionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(extensionClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the extension data source durable client contract.",
		)
		return
	}

	d.client = client
}

func (d *extensionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config extensionModel
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

func (d *extensionDataSource) read(ctx context.Context, config extensionModel) (extensionModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The extension data source was not configured with a Kernel client.",
		)
		return extensionModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("Extension", "kernel_extension", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return extensionModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return extensionModel{}, diags
	}

	var idOrName string
	switch {
	case selector.HasID:
		idOrName = config.ID.ValueString()
	case selector.HasName:
		idOrName = config.Name.ValueString()
	default:
		diags.AddError(
			"Missing Extension Selector",
			"Configure id or name for kernel_extension.",
		)
		return extensionModel{}, diags
	}

	// The API resolves the GET path parameter by id or name within the project,
	// so a single Get covers both selectors.
	state, readDiags := d.get(ctx, projectID, idOrName)
	diags.Append(readDiags...)
	if diags.HasError() {
		return extensionModel{}, diags
	}

	state.ProjectID = config.ProjectID
	return state, diags
}

func (d *extensionDataSource) get(ctx context.Context, projectID, idOrName string) (extensionModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	extension, err := d.client.GetExtension(ctx, projectID, idOrName)
	if err != nil {
		projectscope.AddError(&diags, "Read Kernel Extension", projectID, err)
		return extensionModel{}, diags
	}
	if extension == nil {
		diags.AddError(
			"Read Kernel Extension",
			"Kernel returned an empty extension response.",
		)
		return extensionModel{}, diags
	}

	return flattenExtension(*extension)
}

func flattenExtension(extension kernel.ExtensionGetResponse) (extensionModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !datasources.ValidResponseString(extension.JSON.ID.Raw(), extension.JSON.ID.Valid(), extension.ID) {
		datasources.AddInvalidResponseField(&diags, "Extension", "id")
	}
	if !datasources.ValidResponseTime(extension.JSON.CreatedAt.Raw(), extension.JSON.CreatedAt.Valid(), extension.CreatedAt) {
		datasources.AddInvalidResponseField(&diags, "Extension", "created_at")
	}
	if !validResponseInt64(extension.JSON.SizeBytes.Raw(), extension.JSON.SizeBytes.Valid(), extension.SizeBytes) {
		datasources.AddInvalidResponseField(&diags, "Extension", "size_bytes")
	}

	name := types.StringNull()
	if datasources.FieldPresent(extension.JSON.Name.Raw()) {
		if !datasources.ValidResponseString(extension.JSON.Name.Raw(), extension.JSON.Name.Valid(), extension.Name) {
			datasources.AddInvalidResponseField(&diags, "Extension", "name")
		} else {
			name = types.StringValue(extension.Name)
		}
	}

	if diags.HasError() {
		return extensionModel{}, diags
	}

	return extensionModel{
		ID:        types.StringValue(extension.ID),
		Name:      name,
		CreatedAt: types.StringValue(extension.CreatedAt.Format(time.RFC3339Nano)),
		SizeBytes: types.Int64Value(extension.SizeBytes),
	}, diags
}

func validResponseInt64(raw string, valid bool, value int64) bool {
	if !datasources.FieldPresent(raw) || !valid {
		return false
	}

	decoded, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false
	}
	return decoded == value
}
