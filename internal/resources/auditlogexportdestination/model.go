package auditlogexportdestination

import "github.com/hashicorp/terraform-plugin-framework/types"

type destinationModel struct {
	ID            types.String `tfsdk:"id"`
	Type          types.String `tfsdk:"type"`
	Format        types.String `tfsdk:"format"`
	Bucket        types.String `tfsdk:"bucket"`
	Region        types.String `tfsdk:"region"`
	Prefix        types.String `tfsdk:"prefix"`
	RoleARN       types.String `tfsdk:"role_arn"`
	KMSKeyID      types.String `tfsdk:"kms_key_id"`
	Status        types.String `tfsdk:"status"`
	ExternalID    types.String `tfsdk:"external_id"`
	KernelRoleARN types.String `tfsdk:"kernel_role_arn"`
}
