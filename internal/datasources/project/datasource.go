package project

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/datasources"
)

var (
	_ datasource.DataSource              = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectDataSource)(nil)
)

type projectClient interface {
	DefaultProjectID() string
	GetProject(context.Context, string) (*kernel.Project, error)
}

type projectDataSource struct {
	client projectClient
}

type projectModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Status    types.String `tfsdk:"status"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func NewDataSource() datasource.DataSource {
	return &projectDataSource{}
}

func newDataSourceWithClient(client projectClient) *projectDataSource {
	return &projectDataSource{client: client}
}

func (d *projectDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup durable Kernel project metadata.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project ID. If both id and name are omitted, the provider project_id is used.",
			},
			"name": dschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project name for exact lookup.",
			},
			"status": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project status.",
			},
			"created_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project creation timestamp.",
			},
			"updated_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project update timestamp.",
			},
		},
	}
}

func (d *projectDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(projectClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the project data source durable client contract.",
		)
		return
	}

	d.client = client
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectModel
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

func (d *projectDataSource) read(ctx context.Context, config projectModel) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The project data source was not configured with a Kernel client.",
		)
		return projectModel{}, diags
	}

	selector, selectorDiags := datasources.ResolveIDNameSelector("Project", "kernel_project", config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return projectModel{}, diags
	}

	if selector.HasID {
		return d.get(ctx, config.ID.ValueString())
	}
	if selector.HasName {
		// The API resolves the GET path parameter by id or name (names are
		// unique within an organization), so a single Get covers name lookups.
		name := config.Name.ValueString()
		state, getDiags := d.get(ctx, name)
		diags.Append(getDiags...)
		if diags.HasError() {
			return projectModel{}, diags
		}
		// The API tries id resolution first, so a name that collides with
		// another project's id silently returns that project. Enforce the
		// exact-name contract the selector promises.
		if state.Name.ValueString() != name {
			diags.AddError(
				"Lookup Kernel Project",
				"Kernel resolved "+name+" to a project whose name does not match; the value likely collides with a project id. Configure id instead.",
			)
			return projectModel{}, diags
		}
		return state, diags
	}

	id := d.client.DefaultProjectID()
	if id == "" {
		diags.AddError(
			"Missing Project Selector",
			"Configure id, name, or provider project_id for kernel_project.",
		)
		return projectModel{}, diags
	}
	return d.get(ctx, id)
}

func (d *projectDataSource) get(ctx context.Context, id string) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	project, err := d.client.GetProject(ctx, id)
	if err != nil {
		diags.AddError("Read Kernel Project", err.Error())
		return projectModel{}, diags
	}
	if project == nil {
		diags.AddError("Read Kernel Project", "Kernel returned an empty project response.")
		return projectModel{}, diags
	}
	return flattenProject(*project)
}

func flattenProject(project kernel.Project) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !datasources.ValidResponseString(project.JSON.ID.Raw(), project.JSON.ID.Valid(), project.ID) {
		datasources.AddInvalidResponseField(&diags, "Project", "id")
	}
	if !datasources.ValidResponseString(project.JSON.Name.Raw(), project.JSON.Name.Valid(), project.Name) {
		datasources.AddInvalidResponseField(&diags, "Project", "name")
	}
	if !datasources.ValidResponseString(project.JSON.Status.Raw(), project.JSON.Status.Valid(), string(project.Status)) {
		datasources.AddInvalidResponseField(&diags, "Project", "status")
	}
	if !datasources.ValidResponseTime(project.JSON.CreatedAt.Raw(), project.JSON.CreatedAt.Valid(), project.CreatedAt) {
		datasources.AddInvalidResponseField(&diags, "Project", "created_at")
	}
	if !datasources.ValidResponseTime(project.JSON.UpdatedAt.Raw(), project.JSON.UpdatedAt.Valid(), project.UpdatedAt) {
		datasources.AddInvalidResponseField(&diags, "Project", "updated_at")
	}
	if diags.HasError() {
		return projectModel{}, diags
	}

	return projectModel{
		ID:        types.StringValue(project.ID),
		Name:      types.StringValue(project.Name),
		Status:    types.StringValue(string(project.Status)),
		CreatedAt: types.StringValue(project.CreatedAt.Format(time.RFC3339Nano)),
		UpdatedAt: types.StringValue(project.UpdatedAt.Format(time.RFC3339Nano)),
	}, diags
}
