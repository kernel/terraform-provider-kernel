package provider

import (
	"context"
	"testing"

	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

func TestResolveProviderConfigPrefersTerraformConfigOverEnv(t *testing.T) {
	t.Parallel()

	config, diags := resolveProviderConfig(providerModel{
		APIKey:    types.StringValue("config-api-key"),
		BaseURL:   types.StringValue("https://config.example"),
		ProjectID: types.StringValue("config-project"),
	}, mapEnv(map[string]string{
		envAPIKey:    "env-api-key",
		envBaseURL:   "https://env.example",
		envProjectID: "env-project",
	}))

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	want := configuredProvider{
		APIKey:    "config-api-key",
		BaseURL:   "https://config.example",
		ProjectID: "config-project",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestResolveProviderConfigFallsBackToEnv(t *testing.T) {
	t.Parallel()

	config, diags := resolveProviderConfig(providerModel{
		APIKey:    types.StringNull(),
		BaseURL:   types.StringNull(),
		ProjectID: types.StringNull(),
	}, mapEnv(map[string]string{
		envAPIKey:    "env-api-key",
		envBaseURL:   "https://env.example",
		envProjectID: "env-project",
	}))

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	want := configuredProvider{
		APIKey:    "env-api-key",
		BaseURL:   "https://env.example",
		ProjectID: "env-project",
	}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
}

func TestResolveProviderConfigRequiresAPIKey(t *testing.T) {
	t.Parallel()

	_, diags := resolveProviderConfig(providerModel{
		APIKey:    types.StringNull(),
		BaseURL:   types.StringNull(),
		ProjectID: types.StringNull(),
	}, mapEnv(nil))

	if !diags.HasError() {
		t.Fatal("expected missing api_key diagnostic")
	}
}

func TestResolveProviderConfigRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	_, diags := resolveProviderConfig(providerModel{
		APIKey:    types.StringUnknown(),
		BaseURL:   types.StringUnknown(),
		ProjectID: types.StringUnknown(),
	}, mapEnv(map[string]string{
		envAPIKey: "env-api-key",
	}))

	if got, want := len(diags.Errors()), 3; got != want {
		t.Fatalf("diagnostic errors = %d, want %d: %v", got, want, diags)
	}
}

func TestConfigurePassesResolvedConfigToResourcesAndDataSources(t *testing.T) {
	t.Parallel()

	p := New("test")()

	var schemaResp tfprovider.SchemaResponse
	p.Schema(context.Background(), tfprovider.SchemaRequest{}, &schemaResp)

	req := tfprovider.ConfigureRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw: tftypes.NewValue(
				tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"api_key":    tftypes.String,
						"base_url":   tftypes.String,
						"project_id": tftypes.String,
					},
				},
				map[string]tftypes.Value{
					"api_key":    tftypes.NewValue(tftypes.String, "config-api-key"),
					"base_url":   tftypes.NewValue(tftypes.String, "https://config.example"),
					"project_id": tftypes.NewValue(tftypes.String, "config-project"),
				},
			),
		},
	}

	var resp tfprovider.ConfigureResponse
	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if _, ok := resp.ResourceData.(kernelclient.Clients); !ok {
		t.Fatalf("ResourceData type = %T, want kernelclient.Clients", resp.ResourceData)
	}
	if _, ok := resp.DataSourceData.(kernelclient.Clients); !ok {
		t.Fatalf("DataSourceData type = %T, want kernelclient.Clients", resp.DataSourceData)
	}
}

func mapEnv(values map[string]string) envLookup {
	return func(key string) string {
		return values[key]
	}
}
