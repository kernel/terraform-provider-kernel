package project

import (
	"context"
	"testing"

	tfresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestProjectResourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	r := NewResource()
	var metadata tfresource.MetadataResponse
	r.Metadata(context.Background(), tfresource.MetadataRequest{ProviderTypeName: "kernel"}, &metadata)
	if metadata.TypeName != "kernel_project" {
		t.Fatalf("type name = %q, want kernel_project", metadata.TypeName)
	}

	var schema tfresource.SchemaResponse
	r.Schema(context.Background(), tfresource.SchemaRequest{}, &schema)
	if _, ok := schema.Schema.Attributes["id"]; !ok {
		t.Fatal("project schema missing id attribute")
	}
	if _, ok := schema.Schema.Attributes["name"]; !ok {
		t.Fatal("project schema missing name attribute")
	}
}

func TestProjectResourceConfigureAcceptsDurableClient(t *testing.T) {
	t.Parallel()

	r := &projectResource{}
	var resp tfresource.ConfigureResponse
	r.Configure(context.Background(), tfresource.ConfigureRequest{ProviderData: fakeProjectClient{}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if r.client == nil {
		t.Fatal("project client was not configured")
	}
}

func TestProjectResourceConfigureRejectsUnexpectedData(t *testing.T) {
	t.Parallel()

	r := &projectResource{}
	var resp tfresource.ConfigureResponse
	r.Configure(context.Background(), tfresource.ConfigureRequest{ProviderData: "not a client"}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected unexpected-client-type error")
	}
	if len(resp.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one error", resp.Diagnostics)
	}
	if got := resp.Diagnostics[0].Summary(); got != "Unexpected Kernel Client Type" {
		t.Fatalf("diagnostic summary = %q, want Unexpected Kernel Client Type", got)
	}
}

func TestProjectResourceConfigureAllowsNilProviderData(t *testing.T) {
	t.Parallel()

	r := &projectResource{}
	var resp tfresource.ConfigureResponse
	r.Configure(context.Background(), tfresource.ConfigureRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if r.client != nil {
		t.Fatal("nil provider data configured a client")
	}
}
