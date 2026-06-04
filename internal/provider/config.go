package provider

import (
	"os"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	envAPIKey    = "KERNEL_API_KEY"
	envBaseURL   = "KERNEL_BASE_URL"
	envProjectID = "KERNEL_PROJECT_ID"
)

type providerModel struct {
	APIKey    types.String `tfsdk:"api_key"`
	BaseURL   types.String `tfsdk:"base_url"`
	ProjectID types.String `tfsdk:"project_id"`
}

type configuredProvider struct {
	APIKey    string
	BaseURL   string
	ProjectID string
}

type envLookup func(string) string

func resolveProviderConfig(model providerModel, lookup envLookup) (configuredProvider, diag.Diagnostics) {
	var diags diag.Diagnostics

	if model.APIKey.IsUnknown() {
		diags.AddAttributeError(
			path.Root("api_key"),
			"Unknown Kernel API Key",
			"The provider cannot configure the Kernel API client with an unknown api_key value. Set api_key directly or use the KERNEL_API_KEY environment variable.",
		)
	}

	if model.BaseURL.IsUnknown() {
		diags.AddAttributeError(
			path.Root("base_url"),
			"Unknown Kernel Base URL",
			"The provider cannot configure the Kernel API client with an unknown base_url value. Set base_url directly or use the KERNEL_BASE_URL environment variable.",
		)
	}

	if model.ProjectID.IsUnknown() {
		diags.AddAttributeError(
			path.Root("project_id"),
			"Unknown Kernel Project ID",
			"The provider cannot configure the Kernel API client with an unknown project_id value. Set project_id directly or use the KERNEL_PROJECT_ID environment variable.",
		)
	}

	if diags.HasError() {
		return configuredProvider{}, diags
	}

	config := configuredProvider{
		APIKey:    stringValueOrEnv(model.APIKey, lookup, envAPIKey),
		BaseURL:   stringValueOrEnv(model.BaseURL, lookup, envBaseURL),
		ProjectID: stringValueOrEnv(model.ProjectID, lookup, envProjectID),
	}

	if config.APIKey == "" {
		diags.AddAttributeError(
			path.Root("api_key"),
			"Missing Kernel API Key",
			"The provider requires an API key. Set api_key in provider configuration or set the KERNEL_API_KEY environment variable.",
		)
	}

	return config, diags
}

func stringValueOrEnv(value types.String, lookup envLookup, key string) string {
	if !value.IsNull() {
		return value.ValueString()
	}

	return lookup(key)
}

func lookupEnv(key string) string {
	return os.Getenv(key)
}
