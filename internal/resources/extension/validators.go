package extension

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const maxExtensionNameLength = 255

var (
	extensionNamePattern = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9._-]{1,%d}$`, maxExtensionNameLength))
	extensionCUIDPattern = regexp.MustCompile(`^[a-z0-9]{24}$`)
)

var _ validator.String = extensionNameValidator{}

type extensionNameValidator struct{}

func (extensionNameValidator) Description(context.Context) string {
	return fmt.Sprintf("name must be 1-%d characters using letters, numbers, dots, underscores, or hyphens, and must not be CUID-like", maxExtensionNameLength)
}

func (v extensionNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (extensionNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if extensionNamePattern.MatchString(value) && !extensionCUIDPattern.MatchString(value) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Extension Name",
		fmt.Sprintf("name must be 1-%d characters using letters, numbers, dots, underscores, or hyphens, and must not be a CUID-like string.", maxExtensionNameLength),
	)
}
