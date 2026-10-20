package telemetrydestination

import (
	"context"
	"net/textproto"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.Map = uniqueHeaderNamesValidator{}

// trimmedPattern rejects leading or trailing whitespace, which the API trims
// on write and would otherwise surface as a perpetual diff.
var trimmedPattern = regexp.MustCompile(`^\S(?s:.*\S)?$`)

type uniqueHeaderNamesValidator struct{}

func (uniqueHeaderNamesValidator) Description(context.Context) string {
	return "header names must be unique when compared case-insensitively"
}

func (v uniqueHeaderNamesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (uniqueHeaderNamesValidator) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	seen := make(map[string]string, len(req.ConfigValue.Elements()))
	for name := range req.ConfigValue.Elements() {
		key := textproto.CanonicalMIMEHeaderKey(name)
		if other, ok := seen[key]; ok {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Duplicate Header Name",
				"Headers "+strconv.Quote(other)+" and "+strconv.Quote(name)+" are the same header; header names are case-insensitive.",
			)
			return
		}
		seen[key] = name
	}
}
