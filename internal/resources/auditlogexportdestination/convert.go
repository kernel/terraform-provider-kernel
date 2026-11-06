package auditlogexportdestination

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func expandCreate(plan destinationModel) kernel.AuditLogExportDestinationNewParams {
	request := kernel.CreateAuditLogExportDestinationRequestParam{
		Type:    kernel.CreateAuditLogExportDestinationRequestType(plan.Type.ValueString()),
		Format:  kernel.CreateAuditLogExportDestinationRequestFormat(plan.Format.ValueString()),
		Bucket:  plan.Bucket.ValueString(),
		Region:  plan.Region.ValueString(),
		Prefix:  plan.Prefix.ValueString(),
		RoleArn: plan.RoleARN.ValueString(),
	}
	if !plan.KMSKeyID.IsNull() {
		request.KmsKeyID = kernel.String(plan.KMSKeyID.ValueString())
	}
	return kernel.AuditLogExportDestinationNewParams{CreateAuditLogExportDestinationRequest: request}
}

// expandUpdate returns only the fields that differ between plan and state.
func expandUpdate(plan, state destinationModel) kernel.AuditLogExportDestinationUpdateParams {
	var request kernel.UpdateAuditLogExportDestinationRequestParam
	if !plan.Bucket.Equal(state.Bucket) {
		request.Bucket = kernel.String(plan.Bucket.ValueString())
	}
	if !plan.Region.Equal(state.Region) {
		request.Region = kernel.String(plan.Region.ValueString())
	}
	if !plan.Prefix.Equal(state.Prefix) {
		request.Prefix = kernel.String(plan.Prefix.ValueString())
	}
	if !plan.RoleARN.Equal(state.RoleARN) {
		request.RoleArn = kernel.String(plan.RoleARN.ValueString())
	}
	if !plan.KMSKeyID.Equal(state.KMSKeyID) {
		// The API clears the key when sent an empty string.
		request.KmsKeyID = kernel.String(plan.KMSKeyID.ValueString())
	}
	if !plan.Status.IsUnknown() && !plan.Status.Equal(state.Status) {
		request.Status = kernel.UpdateAuditLogExportDestinationRequestStatus(plan.Status.ValueString())
	}
	return kernel.AuditLogExportDestinationUpdateParams{UpdateAuditLogExportDestinationRequest: request}
}

// flatten builds state from a destination response, leaving out delivery
// health fields such as last_error and consecutive_failures.
func flatten(remote kernel.AuditLogExportDestination, prefix types.String) (destinationModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	required := []struct {
		field, value string
	}{
		{"id", remote.ID},
		{"type", string(remote.Type)},
		{"format", string(remote.Format)},
		{"bucket", remote.Bucket},
		{"region", remote.Region},
		{"role_arn", remote.RoleArn},
		{"status", string(remote.Status)},
		{"external_id", remote.ExternalID},
		{"kernel_role_arn", remote.KernelRoleArn},
	}
	for _, r := range required {
		if r.value == "" {
			diags.AddError(
				"Invalid Kernel Audit Log Export Destination Response",
				"Kernel returned an audit log export destination with missing field "+r.field+".",
			)
		}
	}
	if diags.HasError() {
		return destinationModel{}, diags
	}

	model := destinationModel{
		ID:            types.StringValue(remote.ID),
		Type:          types.StringValue(string(remote.Type)),
		Format:        types.StringValue(string(remote.Format)),
		Bucket:        types.StringValue(remote.Bucket),
		Region:        types.StringValue(remote.Region),
		Prefix:        types.StringValue(remote.Prefix),
		RoleARN:       types.StringValue(remote.RoleArn),
		KMSKeyID:      types.StringNull(),
		Status:        types.StringValue(string(remote.Status)),
		ExternalID:    types.StringValue(remote.ExternalID),
		KernelRoleARN: types.StringValue(remote.KernelRoleArn),
	}
	// Kernel strips trailing slashes; retain the configured spelling when equivalent.
	if !prefix.IsNull() && !prefix.IsUnknown() && strings.TrimRight(prefix.ValueString(), "/") == remote.Prefix {
		model.Prefix = prefix
	}
	if remote.KmsKeyID != "" {
		model.KMSKeyID = types.StringValue(remote.KmsKeyID)
	}
	return model, diags
}
