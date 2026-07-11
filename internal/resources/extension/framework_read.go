package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func readExtensionResource(ctx context.Context, client extensionReader, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state extensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nextState, removed, readDiags := readExtension(ctx, client, state)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if removed {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, nextState)...)
}
