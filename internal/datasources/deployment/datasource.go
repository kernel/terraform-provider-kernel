package deployment

import (
	"context"
	"encoding/json"
	"sort"
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
	_ datasource.DataSource              = (*deploymentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*deploymentDataSource)(nil)
)

type deploymentClient interface {
	DefaultProjectID() string
	GetDeployment(context.Context, string, string) (*kernel.DeploymentGetResponse, error)
}

type deploymentDataSource struct {
	client deploymentClient
}

type deploymentModel struct {
	ID                types.String `tfsdk:"id"`
	ProjectID         types.String `tfsdk:"project_id"`
	EntrypointRelPath types.String `tfsdk:"entrypoint_rel_path"`
	Region            types.String `tfsdk:"region"`
	Status            types.String `tfsdk:"status"`
	StatusReason      types.String `tfsdk:"status_reason"`
	EnvVarKeys        types.Set    `tfsdk:"env_var_keys"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func NewDataSource() datasource.DataSource {
	return &deploymentDataSource{}
}

func newDataSourceWithClient(client deploymentClient) *deploymentDataSource {
	return &deploymentDataSource{client: client}
}

func (d *deploymentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (d *deploymentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup a Kernel deployment by canonical ID without reading logs or event streams.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Canonical deployment ID.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the deployment up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"entrypoint_rel_path": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Relative path to the deployed application entrypoint, if recorded.",
			},
			"region": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment region.",
			},
			"status": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current deployment status for inspection only.",
			},
			"status_reason": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current status reason, if provided by Kernel.",
			},
			"env_var_keys": dschema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Environment variable names configured for the deployment. Values are never exposed.",
			},
			"created_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment creation timestamp.",
			},
			"updated_at": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment update timestamp, if available.",
			},
		},
	}
}

func (d *deploymentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(deploymentClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the deployment data source durable client contract.",
		)
		return
	}
	d.client = client
}

func (d *deploymentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config deploymentModel
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

func (d *deploymentDataSource) read(ctx context.Context, config deploymentModel) (deploymentModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError("Missing Kernel Client", "The deployment data source was not configured with a Kernel client.")
		return deploymentModel{}, diags
	}
	if config.ID.IsNull() || config.ID.IsUnknown() || config.ID.ValueString() == "" {
		diags.AddError("Invalid Kernel Deployment ID", "id must be a known, non-empty deployment ID.")
		return deploymentModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return deploymentModel{}, diags
	}

	deployment, err := d.client.GetDeployment(ctx, projectID, config.ID.ValueString())
	if err != nil {
		projectscope.AddError(&diags, "Read Kernel Deployment", projectID, err)
		return deploymentModel{}, diags
	}
	if deployment == nil {
		diags.AddError("Read Kernel Deployment", "Kernel returned an empty deployment response.")
		return deploymentModel{}, diags
	}

	state, flattenDiags := flattenDeployment(ctx, *deployment)
	diags.Append(flattenDiags...)
	if diags.HasError() {
		return deploymentModel{}, diags
	}
	if state.ID.ValueString() != config.ID.ValueString() {
		diags.AddError(
			"Deployment ID Mismatch",
			"Kernel returned deployment "+state.ID.ValueString()+" for id "+config.ID.ValueString()+".",
		)
		return deploymentModel{}, diags
	}

	state.ProjectID = config.ProjectID
	return state, diags
}

func flattenDeployment(ctx context.Context, deployment kernel.DeploymentGetResponse) (deploymentModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !datasources.ValidResponseString(deployment.JSON.ID.Raw(), deployment.JSON.ID.Valid(), deployment.ID) {
		datasources.AddInvalidResponseField(&diags, "Deployment", "id")
	}
	if !datasources.ValidResponseString(deployment.JSON.Region.Raw(), deployment.JSON.Region.Valid(), string(deployment.Region)) {
		datasources.AddInvalidResponseField(&diags, "Deployment", "region")
	}
	if !datasources.ValidResponseString(deployment.JSON.Status.Raw(), deployment.JSON.Status.Valid(), string(deployment.Status)) {
		datasources.AddInvalidResponseField(&diags, "Deployment", "status")
	}
	if !datasources.ValidResponseTime(deployment.JSON.CreatedAt.Raw(), deployment.JSON.CreatedAt.Valid(), deployment.CreatedAt) {
		datasources.AddInvalidResponseField(&diags, "Deployment", "created_at")
	}
	if !validDeploymentEnvVars(deployment) {
		datasources.AddInvalidResponseField(&diags, "Deployment", "env_vars")
	}
	entrypoint := optionalDeploymentString(deployment.JSON.EntrypointRelPath.Raw(), deployment.JSON.EntrypointRelPath.Valid(), deployment.EntrypointRelPath, "entrypoint_rel_path", &diags)
	statusReason := optionalDeploymentString(deployment.JSON.StatusReason.Raw(), deployment.JSON.StatusReason.Valid(), deployment.StatusReason, "status_reason", &diags)
	updatedAt := types.StringNull()
	if datasources.FieldPresent(deployment.JSON.UpdatedAt.Raw()) {
		if !datasources.ValidResponseTime(deployment.JSON.UpdatedAt.Raw(), deployment.JSON.UpdatedAt.Valid(), deployment.UpdatedAt) {
			datasources.AddInvalidResponseField(&diags, "Deployment", "updated_at")
		} else {
			updatedAt = types.StringValue(deployment.UpdatedAt.Format(time.RFC3339Nano))
		}
	}
	if diags.HasError() {
		return deploymentModel{}, diags
	}

	keys := make([]string, 0, len(deployment.EnvVars))
	for key := range deployment.EnvVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	envVarKeys, setDiags := types.SetValueFrom(ctx, types.StringType, keys)
	diags.Append(setDiags...)
	if diags.HasError() {
		return deploymentModel{}, diags
	}

	return deploymentModel{
		ID:                types.StringValue(deployment.ID),
		EntrypointRelPath: entrypoint,
		Region:            types.StringValue(string(deployment.Region)),
		Status:            types.StringValue(string(deployment.Status)),
		StatusReason:      statusReason,
		EnvVarKeys:        envVarKeys,
		CreatedAt:         types.StringValue(deployment.CreatedAt.Format(time.RFC3339Nano)),
		UpdatedAt:         updatedAt,
	}, diags
}

func validDeploymentEnvVars(deployment kernel.DeploymentGetResponse) bool {
	raw := deployment.JSON.EnvVars.Raw()
	if !datasources.FieldPresent(raw) {
		return len(deployment.EnvVars) == 0
	}
	if !deployment.JSON.EnvVars.Valid() {
		return false
	}

	var decoded map[string]string
	if json.Unmarshal([]byte(raw), &decoded) != nil || len(decoded) != len(deployment.EnvVars) {
		return false
	}
	for key, value := range decoded {
		parsed, ok := deployment.EnvVars[key]
		if !ok || parsed != value {
			return false
		}
	}
	return true
}

func optionalDeploymentString(raw string, valid bool, value, field string, diags *diag.Diagnostics) types.String {
	if !datasources.FieldPresent(raw) {
		return types.StringNull()
	}

	var decoded string
	if !valid || json.Unmarshal([]byte(raw), &decoded) != nil || decoded != value {
		datasources.AddInvalidResponseField(diags, "Deployment", field)
		return types.StringNull()
	}
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
