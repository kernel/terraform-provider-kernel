package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

func createExtensionResource(ctx context.Context, client extensionUploader, defaultProjectID string, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config extensionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if client == nil {
		resp.Diagnostics.AddError(
			"Missing Kernel Client",
			"The Kernel provider was not configured before using the extension resource.",
		)
		return
	}

	if config.Name.IsUnknown() || config.ProjectID.IsUnknown() || config.SourceSHA256.IsUnknown() {
		var plan extensionModel
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if config.Name.IsUnknown() {
			config.Name = plan.Name
		}
		if config.ProjectID.IsUnknown() {
			config.ProjectID = plan.ProjectID
		}
		if config.SourceSHA256.IsUnknown() {
			config.SourceSHA256 = plan.SourceSHA256
		}
	}
	if config.ProjectID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("project_id"),
			"Unknown Kernel Project ID",
			"project_id must be known before uploading a Kernel extension. Re-run the operation after the referenced value is available.",
		)
		return
	}

	projectID := projectscope.Resolve(config.ProjectID, defaultProjectID)
	result, createDiags := createExtension(ctx, client, config, projectID)
	persistState := result.Status == extensionCreateSucceeded ||
		(result.Status == extensionCreateUncertain && result.State.ID.ValueString() != "")
	if persistState {
		resp.Diagnostics.Append(resp.State.Set(ctx, result.State)...)
	}
	resp.Diagnostics.Append(createDiags...)
}
