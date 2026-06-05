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
	cuidPattern            = regexp.MustCompile(`^[a-z0-9]{24}$`)
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
	return fmt.Sprintf("value must be a valid JSON object no larger than %d serialized bytes", maxChromePolicyBytes)
}

func (v chromePolicyJSONValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (chromePolicyJSONValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	normalized, diags := normalizeChromePolicyJSON(req.ConfigValue.ValueString())
	for _, diagnostic := range diags {
		resp.Diagnostics.AddAttributeError(req.Path, diagnostic.Summary(), diagnostic.Detail())
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if len(normalized) > maxChromePolicyBytes {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Chrome Policy JSON",
			fmt.Sprintf("chrome_policy exceeds maximum size of %d bytes when serialized as JSON (got %d bytes).", maxChromePolicyBytes, len(normalized)),
		)
	}
}
