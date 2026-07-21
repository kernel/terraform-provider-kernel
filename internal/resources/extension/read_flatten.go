package extension

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func flattenExtensionRead(response kernel.ExtensionGetResponse, prior extensionModel) (extensionModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !validExtensionResponseString(response.JSON.ID.Raw(), response.JSON.ID.Valid(), response.ID) {
		addInvalidExtensionResponseField(&diags, "id")
	} else if !prior.ID.IsNull() && !prior.ID.IsUnknown() && response.ID != prior.ID.ValueString() {
		diags.AddError(
			"Unexpected Kernel Extension ID",
			fmt.Sprintf("Kernel returned extension ID %q while reading extension %q.", response.ID, prior.ID.ValueString()),
		)
	}

	name := types.StringNull()
	if extensionResponseFieldPresent(response.JSON.Name.Raw()) {
		if !validExtensionResponseString(response.JSON.Name.Raw(), response.JSON.Name.Valid(), response.Name) {
			addInvalidExtensionResponseField(&diags, "name")
		} else {
			name = types.StringValue(response.Name)
		}
	}

	checksum := types.StringNull()
	if extensionResponseFieldPresent(response.JSON.Checksum.Raw()) {
		if !validExtensionResponseString(response.JSON.Checksum.Raw(), response.JSON.Checksum.Valid(), response.Checksum) ||
			!extensionChecksumPattern.MatchString(response.Checksum) {
			addInvalidExtensionResponseField(&diags, "checksum")
		} else {
			checksum = types.StringValue(response.Checksum)
		}
	} else if !prior.SourceSHA256.IsNull() && !prior.SourceSHA256.IsUnknown() {
		diags.AddError(
			"Missing Kernel Extension Checksum",
			"Kernel no longer returned a checksum for extension "+prior.ID.ValueString()+", so Terraform cannot verify the managed archive content.",
		)
	}

	if diags.HasError() {
		return extensionModel{}, diags
	}

	return extensionModel{
		ID:           types.StringValue(response.ID),
		Name:         name,
		ProjectID:    prior.ProjectID,
		SourcePath:   types.StringNull(),
		SourceSHA256: checksum,
	}, diags
}
