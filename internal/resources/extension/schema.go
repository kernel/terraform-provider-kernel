package extension

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var extensionChecksumPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func extensionSchema() rschema.Schema {
	return rschema.Schema{
		MarkdownDescription: "Kernel uploaded extension durable configuration.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique extension identifier.",
			},
			"name": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Optional extension name. Must be unique within the project. Adding or changing a configured name replaces the extension; omitting it preserves the remote name because the API cannot clear a name.",
				PlanModifiers:       immutableExtensionPlanModifiers(),
				Validators: []validator.String{
					extensionNameValidator{},
				},
			},
			"project_id": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Project this extension belongs to. Defaults to the provider `project_id` when unset; when neither is set, the API key's project binding determines the project. Adding or changing it replaces the extension.",
				PlanModifiers:       immutableExtensionPlanModifiers(),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"source_path": rschema.StringAttribute{
				Optional:            true,
				WriteOnly:           true,
				MarkdownDescription: "Local path to the extension ZIP. Required when creating or replacing the extension and never stored in Terraform plan or state artifacts. Requires Terraform 1.11 or later.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"source_sha256": rschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Lowercase hexadecimal SHA-256 checksum of the exact extension ZIP bytes. Configure with `filesha256(source_path)`. Adding or changing it replaces the extension.",
				PlanModifiers:       immutableExtensionPlanModifiers(),
				Validators: []validator.String{
					stringvalidator.RegexMatches(extensionChecksumPattern, "must be a 64-character lowercase hexadecimal SHA-256 checksum"),
				},
			},
		},
	}
}

func immutableExtensionPlanModifiers() []planmodifier.String {
	return []planmodifier.String{
		stringplanmodifier.UseStateForUnknown(),
		stringplanmodifier.RequiresReplace(),
	}
}
