package telemetrydestination

import (
	"context"
	"net/textproto"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	_ validator.String = notIDShapedValidator{}
	_ validator.Map    = uniqueHeaderNamesValidator{}
)

var (
	// trimmedPattern rejects leading or trailing whitespace, which the API trims
	// on write and would otherwise surface as a perpetual diff.
	trimmedPattern = regexp.MustCompile(`^\S(?s:.*\S)?$`)
	// cuidPattern matches Kernel resource IDs. The API rejects names of this
	// shape so a name can't be confused with an ID in id-or-name lookups.
	cuidPattern = regexp.MustCompile(`^[a-z0-9]{24}$`)
)

type notIDShapedValidator struct{}

func (notIDShapedValidator) Description(context.Context) string {
	return "must not be shaped like a Kernel ID (24 lowercase letters or digits)"
}

func (v notIDShapedValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (notIDShapedValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if cuidPattern.MatchString(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Telemetry Destination Name",
			"name must not be shaped like a Kernel ID (24 lowercase letters or digits), because names and IDs share lookups.",
		)
	}
}

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
