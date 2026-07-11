package extension

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	kernel "github.com/kernel/kernel-go-sdk"
)

func prepareExtensionUpload(config extensionModel) (kernel.ExtensionUploadParams, diag.Diagnostics) {
	var diags diag.Diagnostics
	if config.SourcePath.IsNull() || config.SourcePath.IsUnknown() {
		diags.AddAttributeError(
			path.Root("source_path"),
			"Invalid Extension Source Path",
			"source_path must be configured and known before uploading a Kernel extension.",
		)
		return kernel.ExtensionUploadParams{}, diags
	}

	sourcePath := config.SourcePath.ValueString()
	snapshot, err := loadArchiveSnapshot(sourcePath)
	if err != nil {
		diags.AddAttributeError(
			path.Root("source_path"),
			"Read Extension Archive",
			"Cannot read source_path "+strconv.Quote(sourcePath)+": "+err.Error(),
		)
		return kernel.ExtensionUploadParams{}, diags
	}

	params, expandDiags := expandExtensionUpload(config, snapshot)
	diags.Append(expandDiags...)
	return params, diags
}
