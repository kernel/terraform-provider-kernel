package project

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
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var (
	_ datasource.DataSource              = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectDataSource)(nil)
)

type projectClient interface {
	DefaultProjectID() string
	GetProject(context.Context, string) (*kernel.Project, error)
	ListProjectPage(context.Context, string, int64) (kernelclient.ProjectPage, error)
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

type projectSelector int

const (
	projectSelectorProvider projectSelector = iota
	projectSelectorID
	projectSelectorName
)

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

	selector, selectorDiags := resolveProjectSelector(config.ID, config.Name)
	diags.Append(selectorDiags...)
	if diags.HasError() {
		return projectModel{}, diags
	}

	if selector == projectSelectorID {
		return d.get(ctx, config.ID.ValueString())
	}
	if selector == projectSelectorName {
		return d.lookupName(ctx, config.Name.ValueString())
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

func (d *projectDataSource) lookupName(ctx context.Context, name string) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	project, count := d.findProjectsByName(ctx, name, &diags)
	if diags.HasError() {
		return projectModel{}, diags
	}

	switch count {
	case 0:
		diags.AddError(
			"Lookup Kernel Project",
			"No Kernel project found with exact name "+name+".",
		)
		return projectModel{}, diags
	case 1:
		return flattenProject(*project)
	default:
		diags.AddError(
			"Ambiguous Kernel Project Name",
			"Found multiple Kernel projects with exact name "+name+". Configure id instead.",
		)
		return projectModel{}, diags
	}
}

func (d *projectDataSource) findProjectsByName(ctx context.Context, name string, diags *diag.Diagnostics) (*kernel.Project, int) {
	var match *kernel.Project
	count := 0
	offset := int64(0)

	for {
		page, err := d.client.ListProjectPage(ctx, name, offset)
		if err != nil {
			diags.AddError("Lookup Kernel Project", err.Error())
			return nil, 0
		}

		for _, project := range page.Items {
			if !validProjectString(project.JSON.Name.Raw(), project.JSON.Name.Valid(), project.Name) {
				addInvalidProjectField(diags, "name")
				return nil, 0
			}
			if project.Name != name {
				continue
			}
			count++
			if match == nil {
				matched := project
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

func resolveProjectSelector(id, name types.String) (projectSelector, diag.Diagnostics) {
	var diags diag.Diagnostics

	if id.IsUnknown() || name.IsUnknown() {
		diags.AddError(
			"Unknown Project Selector",
			"Project id and name must be known before reading the data source.",
		)
		return projectSelectorProvider, diags
	}
	if !id.IsNull() && id.ValueString() == "" {
		diags.AddError(
			"Empty Project ID",
			"Project id must be omitted or a non-empty string.",
		)
	}
	if !name.IsNull() && name.ValueString() == "" {
		diags.AddError(
			"Empty Project Name",
			"Project name must be omitted or a non-empty string.",
		)
	}
	if diags.HasError() {
		return projectSelectorProvider, diags
	}
	if !id.IsNull() && !name.IsNull() {
		diags.AddError(
			"Conflicting Project Selectors",
			"Configure only one of id or name for kernel_project.",
		)
		return projectSelectorProvider, diags
	}
	if !id.IsNull() {
		return projectSelectorID, diags
	}
	if !name.IsNull() {
		return projectSelectorName, diags
	}
	return projectSelectorProvider, diags
}

func flattenProject(project kernel.Project) (projectModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !validProjectString(project.JSON.ID.Raw(), project.JSON.ID.Valid(), project.ID) {
		addInvalidProjectField(&diags, "id")
	}
	if !validProjectString(project.JSON.Name.Raw(), project.JSON.Name.Valid(), project.Name) {
		addInvalidProjectField(&diags, "name")
	}
	if !validProjectString(project.JSON.Status.Raw(), project.JSON.Status.Valid(), string(project.Status)) {
		addInvalidProjectField(&diags, "status")
	}
	if !validProjectTime(project.JSON.CreatedAt.Raw(), project.JSON.CreatedAt.Valid(), project.CreatedAt) {
		addInvalidProjectField(&diags, "created_at")
	}
	if !validProjectTime(project.JSON.UpdatedAt.Raw(), project.JSON.UpdatedAt.Valid(), project.UpdatedAt) {
		addInvalidProjectField(&diags, "updated_at")
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

func validProjectString(raw string, valid bool, value string) bool {
	if !projectFieldPresent(raw) || !valid || value == "" {
		return false
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func validProjectTime(raw string, valid bool, value time.Time) bool {
	if !projectFieldPresent(raw) || !valid || value.IsZero() {
		return false
	}

	var decoded time.Time
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded.Equal(value)
}

func projectFieldPresent(raw string) bool {
	return raw != "" && strings.TrimSpace(raw) != "null"
}

func addInvalidProjectField(diags *diag.Diagnostics, field string) {
	diags.AddError(
		"Invalid Kernel Project Response",
		"Kernel returned a project with missing or invalid required field "+field+".",
	)
}
