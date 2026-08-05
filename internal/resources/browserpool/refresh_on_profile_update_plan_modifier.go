package browserpool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ planmodifier.Bool = preserveRefreshOnProfileUpdate{}

type preserveRefreshOnProfileUpdate struct{}

func (preserveRefreshOnProfileUpdate) Description(context.Context) string {
	return "Preserves refresh_on_profile_update when profile_id is unchanged."
}

func (m preserveRefreshOnProfileUpdate) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (preserveRefreshOnProfileUpdate) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}

	var stateProfileID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("profile_id"), &stateProfileID)...)
	var plannedProfileID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("profile_id"), &plannedProfileID)...)
	if resp.Diagnostics.HasError() || stateProfileID.IsUnknown() || plannedProfileID.IsUnknown() || !plannedProfileID.Equal(stateProfileID) {
		return
	}

	resp.PlanValue = req.StateValue
}
