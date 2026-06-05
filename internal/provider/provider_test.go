package provider_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/kernel/terraform-provider-kernel/internal/provider"
)

func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	var resp tfprovider.MetadataResponse
	p.Metadata(context.Background(), tfprovider.MetadataRequest{}, &resp)

	if resp.TypeName != "kernel" {
		t.Fatalf("TypeName = %q, want %q", resp.TypeName, "kernel")
	}

	if resp.Version != "test" {
		t.Fatalf("Version = %q, want %q", resp.Version, "test")
	}
}

func TestProviderRegistersBrowserPoolResource(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	resources := p.Resources(context.Background())
	if len(resources) != 1 {
		t.Fatalf("Resources length = %d, want 1", len(resources))
	}

	var resp tfresource.MetadataResponse
	resources[0]().Metadata(
		context.Background(),
		tfresource.MetadataRequest{ProviderTypeName: "kernel"},
		&resp,
	)
	if resp.TypeName != "kernel_browser_pool" {
		t.Fatalf("resource TypeName = %q, want kernel_browser_pool", resp.TypeName)
	}

}

func TestProviderRegistersDataSources(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 3 {
		t.Fatalf("DataSources length = %d, want 3", len(dataSources))
	}

	got := make(map[string]bool, len(dataSources))
	for _, factory := range dataSources {
		var resp datasource.MetadataResponse
		factory().Metadata(
			context.Background(),
			datasource.MetadataRequest{ProviderTypeName: "kernel"},
			&resp,
		)
		got[resp.TypeName] = true
	}

	for _, want := range []string{"kernel_project", "kernel_profile", "kernel_proxy"} {
		if !got[want] {
			t.Fatalf("missing data source %s; got %v", want, got)
		}
	}
}

func TestProviderSchema(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	var resp tfprovider.SchemaResponse
	p.Schema(context.Background(), tfprovider.SchemaRequest{}, &resp)

	apiKey := assertStringAttribute(t, resp.Schema.Attributes["api_key"])
	if !apiKey.Optional {
		t.Fatal("api_key should be optional so KERNEL_API_KEY can supply it")
	}
	if !apiKey.Sensitive {
		t.Fatal("api_key should be sensitive")
	}

	baseURL := assertStringAttribute(t, resp.Schema.Attributes["base_url"])
	if !baseURL.Optional {
		t.Fatal("base_url should be optional")
	}
	if baseURL.Sensitive {
		t.Fatal("base_url should not be sensitive")
	}

	projectID := assertStringAttribute(t, resp.Schema.Attributes["project_id"])
	if !projectID.Optional {
		t.Fatal("project_id should be optional")
	}
	if projectID.Sensitive {
		t.Fatal("project_id should not be sensitive")
	}
}

func assertStringAttribute(t *testing.T, attr providerschema.Attribute) providerschema.StringAttribute {
	t.Helper()

	stringAttr, ok := attr.(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("attribute type = %T, want provider/schema.StringAttribute", attr)
	}

	return stringAttr
}
