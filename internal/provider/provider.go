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
	resp.Schema = schema.Schema{}
}

func (p *kernelProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
}

func (p *kernelProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}

func (p *kernelProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}
