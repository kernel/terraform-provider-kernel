package telemetrydestination

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const maxDescriptionLength = 1024

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,255}$`)

func destinationSchema() rschema.Schema {
	return rschema.Schema{
		MarkdownDescription: "Kernel OTLP telemetry destination. Browser sessions reference a destination by ID or name in `telemetry.export.otlp.destination` to export their telemetry to it. Destinations are organization-scoped and require an organization-scoped API key.",
		Attributes: map[string]rschema.Attribute{
			"id": rschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique telemetry destination identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Destination name. Must be unique within the organization, match `^[a-zA-Z0-9._-]{1,255}$`, and not look like an ID.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(namePattern, "must contain only letters, digits, '.', '_' or '-' and be 1-255 characters long"),
					notIDShapedValidator{},
				},
			},
			"endpoint": rschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Base OTLP/HTTP collector endpoint, without a signal path such as `/v1/logs`. Must be a public http or https URL with no query string or fragment.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(trimmedPattern, "must be non-empty with no leading or trailing whitespace"),
				},
			},
			"description": rschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Destination description.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(maxDescriptionLength),
					stringvalidator.RegexMatches(trimmedPattern, "must be non-empty with no leading or trailing whitespace"),
				},
			},
			"headers": rschema.MapAttribute{
				Optional:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Headers sent with each export request, typically an ingestion key. Header names are matched case-insensitively. Kernel never returns header values, so Terraform detects added or removed header names but not values changed outside Terraform. After import, header values are empty in state and the next apply sends the configured values.",
				Validators: []validator.Map{
					uniqueHeaderNamesValidator{},
				},
			},
		},
	}
}
