package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func modifyExtensionPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var config extensionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	var plan extensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	requiresArchive := req.State.Raw.IsNull()
	if !requiresArchive {
		var state extensionModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		requiresArchive = extensionReplacementPlanned(state, plan)
	}
	if !requiresArchive {
		return
	}

	if config.SourcePath.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("source_path"),
			"Missing Extension Source Path",
			"source_path must be configured when creating or replacing a Kernel extension.",
		)
	}
	if config.SourceSHA256.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("source_sha256"),
			"Missing Extension Source Checksum",
			"source_sha256 must be configured when creating or replacing a Kernel extension. Use filesha256(source_path) to track the exact archive bytes.",
		)
	}
}

func extensionReplacementPlanned(state, plan extensionModel) bool {
	return extensionStringChanged(state.Name, plan.Name) ||
		extensionStringChanged(state.ProjectID, plan.ProjectID) ||
		extensionStringChanged(state.SourceSHA256, plan.SourceSHA256)
}

func extensionStringChanged(state, plan types.String) bool {
	return !state.Equal(plan)
}
