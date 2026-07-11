package extension

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ extensionClient = kernelclient.Clients{}

func TestExtensionResourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	r := NewResource()
	var metadata resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_extension" {
		t.Fatalf("type name = %q, want kernel_extension", metadata.TypeName)
	}

	var schema resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schema)
	for _, name := range []string{"id", "name", "project_id", "source_path", "source_sha256"} {
		if _, ok := schema.Schema.Attributes[name]; !ok {
			t.Fatalf("extension schema missing %s attribute", name)
		}
	}
}

func TestExtensionResourceConfigure(t *testing.T) {
	t.Parallel()

	t.Run("durable client", func(t *testing.T) {
		t.Parallel()
		r := &extensionResource{}
		var resp resource.ConfigureResponse
		r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: kernelclient.Clients{}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if r.client == nil {
			t.Fatal("extension client was not configured")
		}
	})

	t.Run("unexpected provider data", func(t *testing.T) {
		t.Parallel()
		r := &extensionResource{}
		var resp resource.ConfigureResponse
		r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "not a client"}, &resp)
		if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Summary() != "Unexpected Kernel Client Type" {
			t.Fatalf("diagnostics = %v, want Unexpected Kernel Client Type", resp.Diagnostics)
		}
	})

	t.Run("nil provider data", func(t *testing.T) {
		t.Parallel()
		r := &extensionResource{}
		var resp resource.ConfigureResponse
		r.Configure(context.Background(), resource.ConfigureRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if r.client != nil {
			t.Fatal("nil provider data configured a client")
		}
	})
}

func TestExtensionResourceRejectsUpdate(t *testing.T) {
	t.Parallel()

	r := &extensionResource{}
	var resp resource.UpdateResponse
	r.Update(context.Background(), resource.UpdateRequest{}, &resp)

	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Summary() != "Unexpected Kernel Extension Update" {
		t.Fatalf("diagnostics = %v, want Unexpected Kernel Extension Update", resp.Diagnostics)
	}
}
