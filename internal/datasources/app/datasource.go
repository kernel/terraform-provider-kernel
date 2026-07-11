package app

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

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
	_ datasource.DataSource              = (*appDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*appDataSource)(nil)
)

type appClient interface {
	DefaultProjectID() string
	ListAppPage(context.Context, string, string, string, int64) (kernelclient.AppPage, error)
}

type appDataSource struct {
	client appClient
}

type appModel struct {
	ID           types.String `tfsdk:"id"`
	AppName      types.String `tfsdk:"app_name"`
	Version      types.String `tfsdk:"version"`
	ProjectID    types.String `tfsdk:"project_id"`
	DeploymentID types.String `tfsdk:"deployment_id"`
	Region       types.String `tfsdk:"region"`
	Actions      types.Set    `tfsdk:"actions"`
	EnvVarKeys   types.Set    `tfsdk:"env_var_keys"`
}

func NewDataSource() datasource.DataSource {
	return &appDataSource{}
}

func newDataSourceWithClient(client appClient) *appDataSource {
	return &appDataSource{client: client}
}

func (d *appDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *appDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		MarkdownDescription: "Lookup a running Kernel app version by exact app name and version.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Canonical app-version ID.",
			},
			"app_name": dschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact app name.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"version": dschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact app version label.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"project_id": dschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Project to look the app up in. Defaults to the provider `project_id`; when neither is set, the API key's project binding determines the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"deployment_id": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment backing this app version.",
			},
			"region": dschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment region.",
			},
			"actions": dschema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Action names available on this app version.",
			},
			"env_var_keys": dschema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Environment variable names configured for this app version. Values are never exposed.",
			},
		},
	}
}

func (d *appDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(appClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Kernel Client Type",
			"Expected provider data to implement the app data source durable client contract.",
		)
		return
	}

	d.client = client
}

func (d *appDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config appModel
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

func (d *appDataSource) read(ctx context.Context, config appModel) (appModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if d.client == nil {
		diags.AddError("Missing Kernel Client", "The app data source was not configured with a Kernel client.")
		return appModel{}, diags
	}
	if config.AppName.IsNull() || config.AppName.IsUnknown() || config.AppName.ValueString() == "" {
		diags.AddError("Invalid Kernel App Name", "app_name must be a known, non-empty string.")
	}
	if config.Version.IsNull() || config.Version.IsUnknown() || config.Version.ValueString() == "" {
		diags.AddError("Invalid Kernel App Version", "version must be a known, non-empty string.")
	}
	if diags.HasError() {
		return appModel{}, diags
	}

	projectID := projectscope.ResolveDataSource(&diags, config.ProjectID, d.client.DefaultProjectID())
	if diags.HasError() {
		return appModel{}, diags
	}

	match, count := d.findExact(ctx, projectID, config.AppName.ValueString(), config.Version.ValueString(), &diags)
	if diags.HasError() {
		return appModel{}, diags
	}
	switch count {
	case 0:
		diags.AddError("Lookup Kernel App", "No running Kernel app found with the configured exact name and version.")
		return appModel{}, diags
	case 1:
		state, flattenDiags := flattenApp(ctx, *match)
		diags.Append(flattenDiags...)
		if diags.HasError() {
			return appModel{}, diags
		}
		state.ProjectID = config.ProjectID
		return state, diags
	default:
		diags.AddError("Ambiguous Kernel App", "Found multiple running Kernel apps with the configured exact name and version.")
		return appModel{}, diags
	}
}

func (d *appDataSource) findExact(ctx context.Context, projectID, appName, version string, diags *diag.Diagnostics) (*kernel.AppListResponse, int) {
	var match *kernel.AppListResponse
	count := 0
	offset := int64(0)
	seen := map[string]bool{}

	for {
		page, err := d.client.ListAppPage(ctx, projectID, appName, version, offset)
		if err != nil {
			projectscope.AddError(diags, "Lookup Kernel App", projectID, err)
			return nil, 0
		}
		for _, app := range page.Items {
			if app.AppName != appName || app.Version != version || seen[app.ID] {
				continue
			}
			if !datasources.ValidResponseString(app.JSON.ID.Raw(), app.JSON.ID.Valid(), app.ID) {
				datasources.AddInvalidResponseField(diags, "App", "id")
				return nil, 0
			}
			if !datasources.ValidResponseString(app.JSON.AppName.Raw(), app.JSON.AppName.Valid(), app.AppName) {
				datasources.AddInvalidResponseField(diags, "App", "app_name")
				return nil, 0
			}
			if !datasources.ValidResponseString(app.JSON.Version.Raw(), app.JSON.Version.Valid(), app.Version) {
				datasources.AddInvalidResponseField(diags, "App", "version")
				return nil, 0
			}
			seen[app.ID] = true
			count++
			if match == nil {
				matched := app
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

func flattenApp(ctx context.Context, app kernel.AppListResponse) (appModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !datasources.ValidResponseString(app.JSON.ID.Raw(), app.JSON.ID.Valid(), app.ID) {
		datasources.AddInvalidResponseField(&diags, "App", "id")
	}
	if !datasources.ValidResponseString(app.JSON.AppName.Raw(), app.JSON.AppName.Valid(), app.AppName) {
		datasources.AddInvalidResponseField(&diags, "App", "app_name")
	}
	if !datasources.ValidResponseString(app.JSON.Version.Raw(), app.JSON.Version.Valid(), app.Version) {
		datasources.AddInvalidResponseField(&diags, "App", "version")
	}
	if !datasources.ValidResponseString(app.JSON.Deployment.Raw(), app.JSON.Deployment.Valid(), app.Deployment) {
		datasources.AddInvalidResponseField(&diags, "App", "deployment")
	}
	if !datasources.ValidResponseString(app.JSON.Region.Raw(), app.JSON.Region.Valid(), string(app.Region)) {
		datasources.AddInvalidResponseField(&diags, "App", "region")
	}
	if !validAppActions(app) {
		datasources.AddInvalidResponseField(&diags, "App", "actions")
	}
	if !validAppEnvVars(app) {
		datasources.AddInvalidResponseField(&diags, "App", "env_vars")
	}
	if diags.HasError() {
		return appModel{}, diags
	}

	actionNames := make([]string, 0, len(app.Actions))
	seenActions := make(map[string]bool, len(app.Actions))
	for _, action := range app.Actions {
		if !datasources.ValidResponseString(action.JSON.Name.Raw(), action.JSON.Name.Valid(), action.Name) || seenActions[action.Name] {
			datasources.AddInvalidResponseField(&diags, "App", "actions")
			return appModel{}, diags
		}
		seenActions[action.Name] = true
		actionNames = append(actionNames, action.Name)
	}
	sort.Strings(actionNames)

	envVarKeys := make([]string, 0, len(app.EnvVars))
	for key := range app.EnvVars {
		envVarKeys = append(envVarKeys, key)
	}
	sort.Strings(envVarKeys)

	actions, actionDiags := types.SetValueFrom(ctx, types.StringType, actionNames)
	diags.Append(actionDiags...)
	envVars, envVarDiags := types.SetValueFrom(ctx, types.StringType, envVarKeys)
	diags.Append(envVarDiags...)
	if diags.HasError() {
		return appModel{}, diags
	}

	return appModel{
		ID:           types.StringValue(app.ID),
		AppName:      types.StringValue(app.AppName),
		Version:      types.StringValue(app.Version),
		DeploymentID: types.StringValue(app.Deployment),
		Region:       types.StringValue(string(app.Region)),
		Actions:      actions,
		EnvVarKeys:   envVars,
	}, diags
}

func validAppActions(app kernel.AppListResponse) bool {
	if !datasources.FieldPresent(app.JSON.Actions.Raw()) || !app.JSON.Actions.Valid() {
		return false
	}
	var decoded []json.RawMessage
	return json.Unmarshal([]byte(app.JSON.Actions.Raw()), &decoded) == nil && len(decoded) == len(app.Actions)
}

func validAppEnvVars(app kernel.AppListResponse) bool {
	raw := strings.TrimSpace(app.JSON.EnvVars.Raw())
	if raw == "null" {
		return len(app.EnvVars) == 0
	}
	if !datasources.FieldPresent(raw) || !app.JSON.EnvVars.Valid() {
		return false
	}
	var decoded map[string]string
	return json.Unmarshal([]byte(raw), &decoded) == nil && reflect.DeepEqual(decoded, app.EnvVars)
}
