package extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type extensionCreateStatus uint8

const (
	extensionCreateFailed extensionCreateStatus = iota
	extensionCreateSucceeded
	extensionCreateUncertain
)

type extensionCreateResult struct {
	State  extensionModel
	Status extensionCreateStatus
}

type extensionUploader interface {
	UploadExtension(context.Context, string, kernel.ExtensionUploadParams) (*kernel.ExtensionUploadResponse, error)
}

func createExtension(ctx context.Context, client extensionUploader, config extensionModel, projectID string) (extensionCreateResult, diag.Diagnostics) {
	params, diags := prepareExtensionUpload(config)
	if diags.HasError() {
		return extensionCreateResult{Status: extensionCreateFailed}, diags
	}

	created, err := client.UploadExtension(ctx, projectID, params)
	if err != nil {
		if extensionUploadFailureIsDefinite(err) {
			projectscope.AddError(&diags, "Create Kernel Extension", projectID, err)
			return extensionCreateResult{Status: extensionCreateFailed}, diags
		}
		addUncertainExtensionCreateDiagnostic(&diags, projectID, config.SourceSHA256.ValueString(), err.Error())
		return extensionCreateResult{Status: extensionCreateUncertain}, diags
	}
	if created == nil {
		addUncertainExtensionCreateDiagnostic(&diags, projectID, config.SourceSHA256.ValueString(), "Kernel returned an empty extension upload response.")
		return extensionCreateResult{Status: extensionCreateUncertain}, diags
	}

	state, flattenDiags := flattenExtensionUpload(*created, config, projectID)
	if flattenDiags.HasError() {
		addUncertainExtensionCreateDiagnostic(&diags, projectID, config.SourceSHA256.ValueString(), flattenDiags[0].Detail())
		return extensionCreateResult{
			State:  partialExtensionState(*created, config, projectID),
			Status: extensionCreateUncertain,
		}, diags
	}

	return extensionCreateResult{State: state, Status: extensionCreateSucceeded}, diags
}
