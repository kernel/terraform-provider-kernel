package extension

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func flattenExtensionUpload(response kernel.ExtensionUploadResponse, config extensionModel, projectID string) (extensionModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !validExtensionResponseString(response.JSON.ID.Raw(), response.JSON.ID.Valid(), response.ID) {
		addInvalidExtensionResponseField(&diags, "id")
	}

	name := types.StringNull()
	nameValid := true
	if extensionResponseFieldPresent(response.JSON.Name.Raw()) {
		if !validExtensionResponseString(response.JSON.Name.Raw(), response.JSON.Name.Valid(), response.Name) {
			addInvalidExtensionResponseField(&diags, "name")
			nameValid = false
		} else {
			name = types.StringValue(response.Name)
		}
	}
	if nameValid && !name.Equal(config.Name) {
		diags.AddError(
			"Unexpected Kernel Extension Name",
			fmt.Sprintf("Kernel returned extension name %q after uploading configured name %q.", name.ValueString(), config.Name.ValueString()),
		)
	}

	checksum := types.StringNull()
	if !validExtensionResponseString(response.JSON.Checksum.Raw(), response.JSON.Checksum.Valid(), response.Checksum) {
		addInvalidExtensionResponseField(&diags, "checksum")
	} else {
		checksum = types.StringValue(response.Checksum)
		if !checksum.Equal(config.SourceSHA256) {
			diags.AddError(
				"Unexpected Kernel Extension Checksum",
				fmt.Sprintf("Kernel returned extension checksum %q after uploading source_sha256 %q.", checksum.ValueString(), config.SourceSHA256.ValueString()),
			)
		}
	}

	if diags.HasError() {
		return extensionModel{}, diags
	}

	resolvedProjectID := types.StringNull()
	if projectID != "" {
		resolvedProjectID = types.StringValue(projectID)
	}

	return extensionModel{
		ID:           types.StringValue(response.ID),
		Name:         name,
		ProjectID:    resolvedProjectID,
		SourcePath:   types.StringNull(),
		SourceSHA256: checksum,
	}, diags
}

func extensionResponseFieldPresent(raw string) bool {
	return raw != "" && strings.TrimSpace(raw) != "null"
}

func validExtensionResponseString(raw string, valid bool, value string) bool {
	if !extensionResponseFieldPresent(raw) || !valid || value == "" {
		return false
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	return decoded == value
}

func addInvalidExtensionResponseField(diags *diag.Diagnostics, field string) {
	diags.AddError(
		"Invalid Kernel Extension Response",
		"Kernel returned an extension with missing or invalid field "+field+".",
	)
}
