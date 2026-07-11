package extension

import (
	"bytes"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	kernel "github.com/kernel/kernel-go-sdk"
)

func expandExtensionUpload(config extensionModel, snapshot archiveSnapshot) (kernel.ExtensionUploadParams, diag.Diagnostics) {
	var diags diag.Diagnostics
	if config.SourceSHA256.IsNull() || config.SourceSHA256.IsUnknown() {
		diags.AddAttributeError(
			path.Root("source_sha256"),
			"Invalid Extension Source Checksum",
			"source_sha256 must be known before uploading a Kernel extension.",
		)
		return kernel.ExtensionUploadParams{}, diags
	}
	if config.SourceSHA256.ValueString() != snapshot.checksum {
		diags.AddAttributeError(
			path.Root("source_sha256"),
			"Extension Source Checksum Mismatch",
			fmt.Sprintf(
				"source_sha256 is %s, but the extension archive checksum is %s. Re-run Terraform plan after the archive stops changing.",
				config.SourceSHA256.ValueString(),
				snapshot.checksum,
			),
		)
		return kernel.ExtensionUploadParams{}, diags
	}
	if config.Name.IsUnknown() {
		diags.AddAttributeError(
			path.Root("name"),
			"Invalid Extension Name",
			"name must be known before uploading a Kernel extension.",
		)
		return kernel.ExtensionUploadParams{}, diags
	}

	params := kernel.ExtensionUploadParams{
		File: bytes.NewReader(snapshot.data),
	}
	if !config.Name.IsNull() {
		params.Name = kernel.String(config.Name.ValueString())
	}
	return params, diags
}
