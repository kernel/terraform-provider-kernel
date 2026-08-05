package browserpool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

var _ planmodifier.String = preserveEquivalentChromePolicy{}

// preserveEquivalentChromePolicy keeps the prior state spelling when the
// configured JSON object is semantically unchanged. Terraform's protocol does
// not use a custom string value's semantic equality to suppress a resource
// update during planning, so this must happen explicitly at the schema edge.
type preserveEquivalentChromePolicy struct{}

func (preserveEquivalentChromePolicy) Description(context.Context) string {
	return "Preserves the prior chrome_policy value when the configured JSON object is semantically equivalent."
}

func (m preserveEquivalentChromePolicy) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (preserveEquivalentChromePolicy) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	state, stateDiags := normalizeChromePolicyJSON(req.StateValue.ValueString())
	planned, plannedDiags := normalizeChromePolicyJSON(req.PlanValue.ValueString())
	if stateDiags.HasError() || plannedDiags.HasError() {
		return
	}
	if state == planned {
		resp.PlanValue = req.StateValue
	}
}
