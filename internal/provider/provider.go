package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/resources/browserpool"
)

var _ provider.Provider = (*kernelProvider)(nil)

type kernelProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &kernelProvider{
			version: version,
		}
	}
}

func (p *kernelProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "kernel"
	resp.Version = p.version
}

func (p *kernelProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Kernel Terraform provider for durable Kernel infrastructure configuration.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Kernel API key. May also be set with the `KERNEL_API_KEY` environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Kernel API base URL, intended for testing against non-production environments. May also be set with the `KERNEL_BASE_URL` environment variable.",
			},
			"project_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default Kernel project for project-scoped resources and data sources. Resources may override it with their own `project_id`. When neither is set, the API key's project binding determines the project; to run unscoped, leave both unset. May also be set with the `KERNEL_PROJECT_ID` environment variable.",
			},
		},
	}
}

func (p *kernelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var model providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, diags := resolveProviderConfig(model, lookupEnv)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	clients := kernelclient.New(kernelclient.Config{
		APIKey:    config.APIKey,
		BaseURL:   config.BaseURL,
		ProjectID: config.ProjectID,
	})

	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *kernelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		browserpool.NewResource,
	}
}

func (p *kernelProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}
