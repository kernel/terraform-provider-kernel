package project

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const maxProjectNameLength = 255

func projectSchema() rschema.Schema {
	return rschema.Schema{
		MarkdownDescription: "Kernel project durable configuration.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique project identifier.",
			},
			"name": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Project name. Must be unique within the organization.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxProjectNameLength),
				},
			},
		},
	}
}
