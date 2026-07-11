package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type extensionDeleter interface {
	DeleteExtension(context.Context, string, string) error
}

func deleteExtension(ctx context.Context, client extensionDeleter, state extensionModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if client == nil {
		diags.AddError("Missing Kernel Client", "The Kernel provider was not configured before using the extension resource.")
		return diags
	}

	id, ok := extensionStateID(state, "delete", &diags)
	if !ok {
		return diags
	}
	if state.ProjectID.IsUnknown() {
		diags.AddAttributeError(path.Root("project_id"), "Unknown Kernel Project ID", "Cannot delete a Kernel extension while project_id is unknown in Terraform state.")
		return diags
	}

	projectID := state.ProjectID.ValueString()
	if err := client.DeleteExtension(ctx, projectID, id); err != nil {
		if projectscope.IsNotFound(err) {
			return diags
		}
		if extensionDeleteInUse(err) {
			diags.AddError(
				"Delete Kernel Extension",
				fmt.Sprintf("Kernel refused to delete extension %q because one or more browser pools reference it. Remove the extension from those durable browser pool configurations, then retry. Terraform will not mutate browser pools or running browsers implicitly.", id),
			)
			return diags
		}
		projectscope.AddError(&diags, "Delete Kernel Extension", projectID, fmt.Errorf("delete extension %q: %w", id, err))
	}
	return diags
}

func extensionDeleteInUse(err error) bool {
	var apiError *kernel.Error
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusBadRequest {
		return false
	}

	var body struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiError.RawJSON()), &body) != nil {
		return false
	}
	return body.Code == "resource_in_use" || body.Error.Code == "resource_in_use"
}
