package extension

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type extensionReader interface {
	GetExtension(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

func readExtension(ctx context.Context, client extensionReader, state extensionModel) (extensionModel, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if client == nil {
		diags.AddError(
			"Missing Kernel Client",
			"The Kernel provider was not configured before using the extension resource.",
		)
		return extensionModel{}, false, diags
	}

	id, ok := extensionStateID(state, "read", &diags)
	if !ok {
		return extensionModel{}, false, diags
	}
	if state.ProjectID.IsUnknown() {
		diags.AddAttributeError(
			path.Root("project_id"),
			"Unknown Kernel Project ID",
			"Cannot read a Kernel extension while project_id is unknown in Terraform state.",
		)
		return extensionModel{}, false, diags
	}

	projectID := state.ProjectID.ValueString()
	scope := "the API-key-bound project"
	if projectID != "" {
		scope = fmt.Sprintf("project %q", projectID)
	}
	response, err := client.GetExtension(ctx, projectID, id)
	if err != nil {
		if projectscope.IsNotFound(err) {
			return extensionModel{}, true, diags
		}
		projectscope.AddError(&diags, "Read Kernel Extension", projectID, fmt.Errorf("read extension %q in %s: %w", id, scope, err))
		return extensionModel{}, false, diags
	}
	if response == nil {
		diags.AddError(
			"Read Kernel Extension",
			fmt.Sprintf("Kernel returned an empty response for extension %q in %s.", id, scope),
		)
		return extensionModel{}, false, diags
	}

	nextState, flattenDiags := flattenExtensionRead(*response, state)
	diags.Append(flattenDiags...)
	return nextState, false, diags
}

func extensionStateID(state extensionModel, operation string, diags *diag.Diagnostics) (string, bool) {
	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("id"),
			"Missing Kernel Extension ID",
			"Cannot "+operation+" a Kernel extension without a known id in Terraform state.",
		)
		return "", false
	}
	return state.ID.ValueString(), true
}
