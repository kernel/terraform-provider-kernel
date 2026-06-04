package provider_test

import (
	"context"
	"testing"

	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
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

func TestProviderStartsWithNoResourcesOrDataSources(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	if resources := p.Resources(context.Background()); len(resources) != 0 {
		t.Fatalf("Resources length = %d, want 0", len(resources))
	}

	if dataSources := p.DataSources(context.Background()); len(dataSources) != 0 {
		t.Fatalf("DataSources length = %d, want 0", len(dataSources))
	}
}
