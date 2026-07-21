package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func deleteExtensionResource(ctx context.Context, client extensionDeleter, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state extensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(deleteExtension(ctx, client, state)...)
}
