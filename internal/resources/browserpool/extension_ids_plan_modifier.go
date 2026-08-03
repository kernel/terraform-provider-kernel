package browserpool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ planmodifier.List = defaultEmptyExtensionIDsOnCreate{}

// defaultEmptyExtensionIDsOnCreate resolves an omitted extension list only for
// a new pool. Existing pools use UseStateForUnknown so imports and refreshes
// preserve the extension IDs returned by the API.
type defaultEmptyExtensionIDsOnCreate struct{}

func (defaultEmptyExtensionIDsOnCreate) Description(context.Context) string {
	return "Defaults omitted extension_ids to an empty list when creating a browser pool."
}

func (m defaultEmptyExtensionIDsOnCreate) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (defaultEmptyExtensionIDsOnCreate) PlanModifyList(_ context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if !req.State.Raw.IsNull() || !req.ConfigValue.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}
	resp.PlanValue = types.ListValueMust(types.StringType, nil)
}
