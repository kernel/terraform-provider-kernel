package extension

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func extensionUploadFailureIsDefinite(err error) bool {
	var apiError *kernel.Error
	return errors.As(err, &apiError) && apiError.StatusCode >= http.StatusBadRequest && apiError.StatusCode < http.StatusInternalServerError
}

func partialExtensionState(response kernel.ExtensionUploadResponse, config extensionModel, projectID string) extensionModel {
	if !validExtensionResponseString(response.JSON.ID.Raw(), response.JSON.ID.Valid(), response.ID) {
		return extensionModel{}
	}

	resolvedProjectID := types.StringNull()
	if projectID != "" {
		resolvedProjectID = types.StringValue(projectID)
	}

	return extensionModel{
		ID:           types.StringValue(response.ID),
		Name:         config.Name,
		ProjectID:    resolvedProjectID,
		SourcePath:   types.StringNull(),
		SourceSHA256: config.SourceSHA256,
	}
}

func addUncertainExtensionCreateDiagnostic(diags *diag.Diagnostics, projectID, checksum, reason string) {
	scope := "the API-key-bound project"
	if projectID != "" {
		scope = "project " + strconv.Quote(projectID)
	}

	diags.AddError(
		"Kernel Extension Upload Outcome Uncertain",
		"Kernel may have uploaded extension content with source_sha256 "+strconv.Quote(checksum)+" in "+scope+", but Terraform did not receive a complete confirmation. "+
			"Check Kernel for a matching extension. If it exists and Terraform is not tracking it, either import its canonical extension ID or delete it before applying again. If no match exists, retry the apply. "+
			"Reason: "+reason,
	)
}
