package profile

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

var (
	_ datasource.DataSource              = (*profileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*profileDataSource)(nil)
)

type profileClient interface {
	DefaultProjectID() string
	GetProfile(context.Context, string, string) (*kernel.Profile, error)
	ListProfilePage(context.Context, string, string, int64) (kernelclient.ProfilePage, error)
}

type profileDataSource struct {
	client profileClient
}

type profileModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ProjectID types.String `tfsdk:"project_id"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func NewDataSource() datasource.DataSource {
	return &profileDataSource{}
}

func newDataSourceWithClient(client profileClient) *profileDataSource {
	return &profileDataSource{client: client}
}

func (d *profileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_profile"
}

func (d *profileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup durable Kernel profile metadata.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Profile ID.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Profile name for exact lookup.",
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the profile up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"created_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Profile creation timestamp.",
			},
		},
	}
}

func (d *profileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(profileClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the profile data source durable client contract.",
		)
		return
	}

	d.client = client
}

func (d *profileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config profileModel
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

func (d *profileDataSource) read(ctx context.Context, config profileModel) (profileModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The profile data source was not configured with a Kernel client.",
		)
		return profileModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("Profile", "kernel_profile", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return profileModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return profileModel{}, diags
	}

	var state profileModel
	var readDiags diag.Diagnostics
	switch {
	case selector.HasID:
		state, readDiags = d.get(ctx, projectID, config.ID.ValueString())
	case selector.HasName:
		state, readDiags = d.lookupName(ctx, projectID, config.Name.ValueString())
	default:
		diags.AddError(
			"Missing Profile Selector",
			"Configure id or name for kernel_profile.",
		)
		return profileModel{}, diags
	}

	diags.Append(readDiags...)
	if diags.HasError() {
		return profileModel{}, diags
	}

	state.ProjectID = config.ProjectID
	return state, diags
}

func (d *profileDataSource) get(ctx context.Context, projectID, id string) (profileModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	profile, err := d.client.GetProfile(ctx, projectID, id)
	if err != nil {
		projectscope.AddError(&diags, "Read Kernel Profile", projectID, err)
		return profileModel{}, diags
	}
	if profile == nil {
		diags.AddError("Read Kernel Profile", "Kernel returned an empty profile response.")
		return profileModel{}, diags
	}
	state, flattenDiags := flattenProfile(*profile)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return profileModel{}, diags
	}
	if state.ID.ValueString() != id {
		diags.AddError(
			"Profile ID Mismatch",
			"Kernel returned profile "+state.ID.ValueString()+" for id selector "+id+". Use the name selector for name lookups.",
		)
		return profileModel{}, diags
	}
	return state, diags
}

func (d *profileDataSource) lookupName(ctx context.Context, projectID, name string) (profileModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	profile, count := d.findProfilesByName(ctx, projectID, name, &diags)
	if diags.HasError() {
		return profileModel{}, diags
	}

	switch count {
	case 0:
		diags.AddError(
			"Lookup Kernel Profile",
			"No Kernel profile found with exact name "+name+".",
		)
		return profileModel{}, diags
	case 1:
		return flattenProfile(*profile)
	default:
		diags.AddError(
			"Ambiguous Kernel Profile Name",
			"Found multiple Kernel profiles with exact name "+name+". Configure id instead.",
		)
		return profileModel{}, diags
	}
}

func (d *profileDataSource) findProfilesByName(ctx context.Context, projectID, name string, diags *diag.Diagnostics) (*kernel.Profile, int) {
	var match *kernel.Profile
	count := 0
	offset := int64(0)
	seen := map[string]bool{}

	for {
		page, err := d.client.ListProfilePage(ctx, projectID, name, offset)
		if err != nil {
			projectscope.AddError(diags, "Lookup Kernel Profile", projectID, err)
			return nil, 0
		}

		for _, profile := range page.Items {
			// Filter on the decoded name first: the server query is fuzzy, so
			// unrelated rows (including ones with absent or malformed names)
			// come back and must be skipped, not abort the lookup. Only rows
			// claiming the requested name get raw-consistency validation.
			if profile.Name != name {
				continue
			}
			if !datasources.ValidResponseString(profile.JSON.Name.Raw(), profile.JSON.Name.Valid(), profile.Name) {
				datasources.AddInvalidResponseField(diags, "Profile", "name")
				return nil, 0
			}
			// Dedupe by id: offset pagination can repeat a row across pages
			// when the list shifts mid-scan, and a repeated row must not be
			// mistaken for a second profile with the same name.
			if seen[profile.ID] {
				continue
			}
			seen[profile.ID] = true
			count++
			if match == nil {
				matched := profile
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

func flattenProfile(profile kernel.Profile) (profileModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !datasources.ValidResponseString(profile.JSON.ID.Raw(), profile.JSON.ID.Valid(), profile.ID) {
		datasources.AddInvalidResponseField(&diags, "Profile", "id")
	}
	if !datasources.ValidResponseTime(profile.JSON.CreatedAt.Raw(), profile.JSON.CreatedAt.Valid(), profile.CreatedAt) {
		datasources.AddInvalidResponseField(&diags, "Profile", "created_at")
	}

	name := types.StringNull()
	if datasources.FieldPresent(profile.JSON.Name.Raw()) {
		if !datasources.ValidResponseString(profile.JSON.Name.Raw(), profile.JSON.Name.Valid(), profile.Name) {
			datasources.AddInvalidResponseField(&diags, "Profile", "name")
		} else {
			name = types.StringValue(profile.Name)
		}
	}

	if diags.HasError() {
		return profileModel{}, diags
	}

	return profileModel{
		ID:        types.StringValue(profile.ID),
		Name:      name,
		CreatedAt: types.StringValue(profile.CreatedAt.Format(time.RFC3339Nano)),
	}, diags
}
