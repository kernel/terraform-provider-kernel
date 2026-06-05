package browserpool

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ validator.Int64  = int64RangeValidator{}
	_ validator.String = browserPoolNameValidator{}
	_ validator.String = chromePolicyJSONValidator{}
	_ validator.String = nonEmptyStringValidator{}
	_ validator.Set    = nonEmptyStringSetValidator{}
)

var (
	browserPoolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,255}$`)
	cuidPattern            = regexp.MustCompile(`^[a-z0-9]{24}$`)
)

type int64RangeValidator struct {
	min    int64
	max    int64
	hasMax bool
}

func int64AtLeast(min int64) int64RangeValidator {
	return int64RangeValidator{min: min}
}

func int64Between(min, max int64) int64RangeValidator {
	return int64RangeValidator{min: min, max: max, hasMax: true}
}

func (v int64RangeValidator) Description(context.Context) string {
	if v.hasMax {
		return fmt.Sprintf("value must be between %d and %d", v.min, v.max)
	}
	return fmt.Sprintf("value must be at least %d", v.min)
}

func (v int64RangeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v int64RangeValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueInt64()
	if value < v.min || (v.hasMax && value > v.max) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Browser Pool Value",
			fmt.Sprintf("%s must be %s.", req.Path.String(), v.Description(ctx)),
		)
	}
}

type chromePolicyJSONValidator struct{}

func (chromePolicyJSONValidator) Description(context.Context) string {
	return "value must be a valid JSON object"
}

type browserPoolNameValidator struct{}

func (browserPoolNameValidator) Description(context.Context) string {
	return "name must be 1-255 characters using letters, numbers, dots, underscores, or hyphens, and must not be cuid-like"
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
		"name must be 1-255 characters using letters, numbers, dots, underscores, or hyphens, and must not be a cuid-like string.",
	)
}

func (v chromePolicyJSONValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (chromePolicyJSONValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	_, diags := NormalizeChromePolicyJSON(req.ConfigValue.ValueString())
	for _, diagnostic := range diags {
		resp.Diagnostics.AddAttributeError(req.Path, diagnostic.Summary(), diagnostic.Detail())
	}
}

type nonEmptyStringValidator struct {
	attributeName string
}

func (v nonEmptyStringValidator) Description(context.Context) string {
	return fmt.Sprintf("%s must not be an empty string", v.attributeName)
}

func (v nonEmptyStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nonEmptyStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() != "" {
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Browser Pool Value",
		fmt.Sprintf("%s cannot be an empty string.", v.attributeName),
	)
}

type nonEmptyStringSetValidator struct{}

func (nonEmptyStringSetValidator) Description(context.Context) string {
	return fmt.Sprintf("values must not be empty strings and must contain at most %d items", maxBrowserPoolExtensions)
}

func (v nonEmptyStringSetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (nonEmptyStringSetValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if len(req.ConfigValue.Elements()) > maxBrowserPoolExtensions {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Extension IDs",
			fmt.Sprintf("extension_ids cannot contain more than %d items.", maxBrowserPoolExtensions),
		)
		return
	}

	for _, element := range req.ConfigValue.Elements() {
		if element.IsUnknown() {
			continue
		}

		value, ok := element.(types.String)
		if !ok || value.IsNull() || value.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid Extension ID",
				"extension_ids cannot contain null or empty strings.",
			)
			return
		}
	}
}
