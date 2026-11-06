package auditlogexportdestination

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	kernel "github.com/kernel/kernel-go-sdk"
)

const (
	typeS3       = string(kernel.AuditLogExportDestinationTypeS3)
	formatJSONL  = string(kernel.AuditLogExportDestinationFormatJSONLGz)
	statusActive = string(kernel.AuditLogExportDestinationStatusActive)
	statusPaused = string(kernel.AuditLogExportDestinationStatusPaused)
)

func destinationSchema() rschema.Schema {
	nonEmpty := []validator.String{stringvalidator.LengthAtLeast(1)}

	return rschema.Schema{
		MarkdownDescription: "Kernel audit-log export destination. Kernel continuously delivers the organization's audit logs to an S3 bucket: its role, `kernel_role_arn`, assumes your `role_arn` with `external_id` as the `sts:ExternalId` condition. " +
			"Destinations are organization-scoped, require an organization-scoped API key, and require a Kernel plan that includes audit logs. " +
			"New destinations are paused. Create the destination, configure the role's trust policy using the computed values, verify destination access, then set `status = \"active\"` and apply again. " +
			"Delivery starts when the destination is activated; events recorded before activation or while paused are never exported. " +
			"Replacing a destination issues a new `external_id`, so the role's trust policy has to be updated to match.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique audit-log export destination identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(typeS3),
				MarkdownDescription: "Destination type. Only `s3` is supported. Changing it replaces the destination.",
				Validators: []validator.String{
					stringvalidator.OneOf(typeS3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"format": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(formatJSONL),
				MarkdownDescription: "Object format. Only `jsonl.gz` is supported. Changing it replaces the destination.",
				Validators: []validator.String{
					stringvalidator.OneOf(formatJSONL),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bucket": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "S3 bucket name.",
				Validators:          nonEmpty,
			},
			"region": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "AWS region of the bucket.",
				Validators:          nonEmpty,
			},
			"prefix": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Key prefix for exported objects.",
			},
			"role_arn": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ARN of the IAM role in your account that Kernel assumes to write objects.",
				Validators:          nonEmpty,
			},
			"kms_key_id": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "KMS key ID, alias, or ARN used to encrypt exported objects. Removing it clears the key.",
				Validators:          nonEmpty,
			},
			"status": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`active` or `paused`. Pausing stops delivery, and resuming starts again from the time of the resume. When omitted, Terraform does not manage the status and new destinations remain paused.",
				Validators: []validator.String{
					stringvalidator.OneOf(statusActive, statusPaused),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"external_id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Value Kernel sends as `sts:ExternalId` when it assumes `role_arn`. Require it in the role's trust policy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"kernel_role_arn": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Kernel role that assumes `role_arn`. Allow it as the principal in the role's trust policy.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
