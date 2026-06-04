package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
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
				MarkdownDescription: "Optional Kernel API base URL. May also be set with the `KERNEL_BASE_URL` environment variable.",
			},
			"project_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional Kernel project ID used to scope project resources. May also be set with the `KERNEL_PROJECT_ID` environment variable.",
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

	resp.DataSourceData = config
	resp.ResourceData = config
}

func (p *kernelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}

func (p *kernelProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}
