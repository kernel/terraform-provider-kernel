package browserpool

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	_ validator.String = browserPoolNameValidator{}
	_ validator.String = chromePolicyJSONValidator{}
)

var (
	browserPoolNamePattern = regexp.MustCompile(fmt.Sprintf(`^[a-zA-Z0-9._-]{1,%d}$`, maxBrowserPoolNameLength))
	// cuids are lowercase base36, exactly 24 chars; pool names that look like a
	// cuid are rejected so a name can't be confused with a resource ID. The
	// pattern is intentionally lowercase-only: an uppercase 24-char name is not
	// a cuid and stays allowed.
	cuidPattern = regexp.MustCompile(`^[a-z0-9]{24}$`)
)

type browserPoolNameValidator struct{}

func (browserPoolNameValidator) Description(context.Context) string {
	return fmt.Sprintf("name must be 1-%d characters using letters, numbers, dots, underscores, or hyphens, and must not be cuid-like", maxBrowserPoolNameLength)
}

func (v browserPoolNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (browserPoolNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if browserPoolNamePattern.MatchString(value) && !cuidPattern.MatchString(value) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Browser Pool Name",
		fmt.Sprintf("name must be 1-%d characters using letters, numbers, dots, underscores, or hyphens, and must not be a cuid-like string.", maxBrowserPoolNameLength),
	)
}

type chromePolicyJSONValidator struct{}

func (chromePolicyJSONValidator) Description(context.Context) string {
	return fmt.Sprintf("value must be a valid JSON object no larger than %d bytes", maxChromePolicyBytes)
}

func (v chromePolicyJSONValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (chromePolicyJSONValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()

	// Guard on the raw input length before parsing, so an oversized blob is
	// rejected without decoding and re-marshalling it.
	if len(value) > maxChromePolicyBytes {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Chrome Policy JSON",
			fmt.Sprintf("chrome_policy must be no larger than %d bytes (got %d bytes).", maxChromePolicyBytes, len(value)),
		)
		return
	}

	_, diags := normalizeChromePolicyJSON(value)
	for _, diagnostic := range diags {
		resp.Diagnostics.AddAttributeError(req.Path, diagnostic.Summary(), diagnostic.Detail())
	}
}
