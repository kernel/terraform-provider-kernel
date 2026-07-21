package apikey

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var (
	_ datasource.DataSource              = (*apiKeyDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*apiKeyDataSource)(nil)
)

type apiKeyClient interface {
	GetAPIKey(context.Context, string) (*kernel.APIKey, error)
	ListAPIKeyPage(context.Context, string, int64) (kernelclient.APIKeyPage, error)
}

type apiKeyDataSource struct {
	client apiKeyClient
}

type apiKeyModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	MaskedKey      types.String `tfsdk:"masked_key"`
	ProjectID      types.String `tfsdk:"project_id"`
	ProjectName    types.String `tfsdk:"project_name"`
	CreatedAt      types.String `tfsdk:"created_at"`
	ExpiresAt      types.String `tfsdk:"expires_at"`
	CreatedByID    types.String `tfsdk:"created_by_id"`
	CreatedByEmail types.String `tfsdk:"created_by_email"`
	CreatedByName  types.String `tfsdk:"created_by_name"`
}

func NewDataSource() datasource.DataSource {
	return &apiKeyDataSource{}
}

func newDataSourceWithClient(client apiKeyClient) *apiKeyDataSource {
	return &apiKeyDataSource{client: client}
}

func (d *apiKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (d *apiKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup masked metadata for a non-deleted Kernel API key by canonical ID or exact name.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Canonical API key ID.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "API key name for exact lookup. Names are not unique, so ambiguous matches fail.",
			},
			"masked_key": dschema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Masked API key metadata. Plaintext is never returned or stored.",
			},
			"project_id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project ID for a project-scoped key, or null for an organization-wide key.",
			},
			"project_name": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project name for a project-scoped key, when available.",
			},
			"created_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "API key creation timestamp.",
			},
			"expires_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "API key expiration timestamp, or null when the key does not expire.",
			},
			"created_by_id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kernel user ID of the key creator.",
			},
			"created_by_email": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Email address of the key creator.",
			},
			"created_by_name": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name of the key creator, when available.",
			},
		},
	}
}

func (d *apiKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(apiKeyClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the API key data source durable client contract.",
		)
		return
	}
	d.client = client
}

func (d *apiKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config apiKeyModel
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

func (d *apiKeyDataSource) read(ctx context.Context, config apiKeyModel) (apiKeyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError("Missing Kernel Client", "The API key data source was not configured with a Kernel client.")
		return apiKeyModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("API Key", "kernel_api_key", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return apiKeyModel{}, diags
	}

	if selector.HasID {
		state, getDiags := d.get(ctx, config.ID.ValueString())
		diags.Append(getDiags...)
		return state, diags
	}
	if selector.HasName {
		state, lookupDiags := d.lookupName(ctx, config.Name.ValueString())
		diags.Append(lookupDiags...)
		return state, diags
	}

	diags.AddError("Missing API Key Selector", "Configure id or name for kernel_api_key.")
	return apiKeyModel{}, diags
}

func (d *apiKeyDataSource) get(ctx context.Context, id string) (apiKeyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	key, err := d.client.GetAPIKey(ctx, id)
	if err != nil {
		diags.AddError("Read Kernel API Key", err.Error())
		return apiKeyModel{}, diags
	}
	if key == nil {
		diags.AddError("Read Kernel API Key", "Kernel returned an empty API key response.")
		return apiKeyModel{}, diags
	}

	state, flattenDiags := flattenAPIKey(*key)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return apiKeyModel{}, diags
	}
	if state.ID.ValueString() != id {
		diags.AddError("API Key ID Mismatch", "Kernel returned API key "+state.ID.ValueString()+" for id "+id+".")
		return apiKeyModel{}, diags
	}
	return state, diags
}

func (d *apiKeyDataSource) lookupName(ctx context.Context, name string) (apiKeyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	match, count := d.findByName(ctx, name, &diags)
	if diags.HasError() {
		return apiKeyModel{}, diags
	}
	switch count {
	case 0:
		diags.AddError("Lookup Kernel API Key", "No non-deleted Kernel API key found with exact name "+name+".")
		return apiKeyModel{}, diags
	case 1:
		state, flattenDiags := flattenAPIKey(*match)
		diags.Append(flattenDiags...)
		return state, diags
	default:
		diags.AddError("Ambiguous Kernel API Key", "Found multiple non-deleted Kernel API keys with exact name "+name+". Configure id instead.")
		return apiKeyModel{}, diags
	}
}

func (d *apiKeyDataSource) findByName(ctx context.Context, name string, diags *diag.Diagnostics) (*kernel.APIKey, int) {
	var match *kernel.APIKey
	count := 0
	offset := int64(0)
	seen := map[string]bool{}

	for {
		page, err := d.client.ListAPIKeyPage(ctx, name, offset)
		if err != nil {
			diags.AddError("Lookup Kernel API Key", err.Error())
			return nil, 0
		}
		for _, key := range page.Items {
			if key.Name != name || seen[key.ID] {
				continue
			}
			if !datasources.ValidResponseString(key.JSON.ID.Raw(), key.JSON.ID.Valid(), key.ID) {
				datasources.AddInvalidResponseField(diags, "API Key", "id")
				return nil, 0
			}
			if !datasources.ValidResponseString(key.JSON.Name.Raw(), key.JSON.Name.Valid(), key.Name) {
				datasources.AddInvalidResponseField(diags, "API Key", "name")
				return nil, 0
			}
			seen[key.ID] = true
			count++
			if match == nil {
				matched := key
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

func flattenAPIKey(key kernel.APIKey) (apiKeyModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !datasources.ValidResponseString(key.JSON.ID.Raw(), key.JSON.ID.Valid(), key.ID) {
		datasources.AddInvalidResponseField(&diags, "API Key", "id")
	}
	if !datasources.ValidResponseString(key.JSON.Name.Raw(), key.JSON.Name.Valid(), key.Name) {
		datasources.AddInvalidResponseField(&diags, "API Key", "name")
	}
	if !datasources.ValidResponseString(key.JSON.MaskedKey.Raw(), key.JSON.MaskedKey.Valid(), key.MaskedKey) {
		datasources.AddInvalidResponseField(&diags, "API Key", "masked_key")
	}
	if !datasources.ValidResponseTime(key.JSON.CreatedAt.Raw(), key.JSON.CreatedAt.Valid(), key.CreatedAt) {
		datasources.AddInvalidResponseField(&diags, "API Key", "created_at")
	}
	if strings.TrimSpace(key.JSON.DeletedAt.Raw()) != "null" || !key.DeletedAt.IsZero() {
		datasources.AddInvalidResponseField(&diags, "API Key", "deleted_at")
	}
	if !datasources.FieldPresent(key.JSON.CreatedBy.Raw()) || !key.JSON.CreatedBy.Valid() {
		datasources.AddInvalidResponseField(&diags, "API Key", "created_by")
	}
	if !datasources.ValidResponseString(key.CreatedBy.JSON.ID.Raw(), key.CreatedBy.JSON.ID.Valid(), key.CreatedBy.ID) {
		datasources.AddInvalidResponseField(&diags, "API Key", "created_by.id")
	}
	if !datasources.ValidResponseString(key.CreatedBy.JSON.Email.Raw(), key.CreatedBy.JSON.Email.Valid(), key.CreatedBy.Email) {
		datasources.AddInvalidResponseField(&diags, "API Key", "created_by.email")
	}

	expiresAt := nullableAPIKeyTime(key.JSON.ExpiresAt.Raw(), key.JSON.ExpiresAt.Valid(), key.ExpiresAt, "expires_at", &diags)
	projectID := nullableAPIKeyString(key.JSON.ProjectID.Raw(), key.JSON.ProjectID.Valid(), key.ProjectID, "project_id", &diags)
	projectName := nullableAPIKeyString(key.JSON.ProjectName.Raw(), key.JSON.ProjectName.Valid(), key.ProjectName, "project_name", &diags)
	createdByName := nullableAPIKeyString(key.CreatedBy.JSON.Name.Raw(), key.CreatedBy.JSON.Name.Valid(), key.CreatedBy.Name, "created_by.name", &diags)
	if diags.HasError() {
		return apiKeyModel{}, diags
	}

	return apiKeyModel{
		ID:             types.StringValue(key.ID),
		Name:           types.StringValue(key.Name),
		MaskedKey:      types.StringValue(key.MaskedKey),
		ProjectID:      projectID,
		ProjectName:    projectName,
		CreatedAt:      types.StringValue(key.CreatedAt.Format(time.RFC3339Nano)),
		ExpiresAt:      expiresAt,
		CreatedByID:    types.StringValue(key.CreatedBy.ID),
		CreatedByEmail: types.StringValue(key.CreatedBy.Email),
		CreatedByName:  createdByName,
	}, diags
}

func nullableAPIKeyString(raw string, valid bool, value, field string, diags *diag.Diagnostics) types.String {
	if raw == "" {
		datasources.AddInvalidResponseField(diags, "API Key", field)
		return types.StringNull()
	}
	if strings.TrimSpace(raw) == "null" {
		if value != "" {
			datasources.AddInvalidResponseField(diags, "API Key", field)
		}
		return types.StringNull()
	}

	var decoded string
	if !valid || json.Unmarshal([]byte(raw), &decoded) != nil || decoded != value {
		datasources.AddInvalidResponseField(diags, "API Key", field)
		return types.StringNull()
	}
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func nullableAPIKeyTime(raw string, valid bool, value time.Time, field string, diags *diag.Diagnostics) types.String {
	if raw == "" {
		datasources.AddInvalidResponseField(diags, "API Key", field)
		return types.StringNull()
	}
	if strings.TrimSpace(raw) == "null" {
		if !value.IsZero() {
			datasources.AddInvalidResponseField(diags, "API Key", field)
		}
		return types.StringNull()
	}
	if !datasources.ValidResponseTime(raw, valid, value) {
		datasources.AddInvalidResponseField(diags, "API Key", field)
		return types.StringNull()
	}
	return types.StringValue(value.Format(time.RFC3339Nano))
}
