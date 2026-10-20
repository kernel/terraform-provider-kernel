package telemetrydestination

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUniqueHeaderNamesValidator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		headers   types.Map
		wantError bool
	}{
		"distinct names":      {headers: headersValue(t, map[string]string{"Authorization": "a", "X-Dataset": "b"})},
		"null":                {headers: types.MapNull(types.StringType)},
		"unknown":             {headers: types.MapUnknown(types.StringType)},
		"two spellings":       {headers: headersValue(t, map[string]string{"authorization": "a", "Authorization": "b"}), wantError: true},
		"canonicalized dupes": {headers: headersValue(t, map[string]string{"x-dataset": "a", "X-DATASET": "b"}), wantError: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var resp validator.MapResponse
			uniqueHeaderNamesValidator{}.ValidateMap(context.Background(), validator.MapRequest{
				Path:        path.Root("headers"),
				ConfigValue: test.headers,
			}, &resp)
			if got := resp.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("has error = %t, want %t: %v", got, test.wantError, resp.Diagnostics)
			}
		})
	}
}

func TestTrimmedPattern(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]bool{
		"https://api.honeycomb.io": true,
		"multi\nline":              true,
		"x":                        true,
		"":                         false,
		" ":                        false,
		" leading":                 false,
		"trailing ":                false,
		"trailing\n":               false,
	} {
		if got := trimmedPattern.MatchString(value); got != want {
			t.Fatalf("trimmedPattern.MatchString(%q) = %t, want %t", value, got, want)
		}
	}
}

func TestNotIDShapedValidator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name      types.String
		wantError bool
	}{
		"ordinary name":         {name: types.StringValue("otlp-collector")},
		"24 chars with capital": {name: types.StringValue("Abcdefghijklmnopqrstuvwx")},
		"25 lowercase chars":    {name: types.StringValue("abcdefghijklmnopqrstuvwxy")},
		"null":                  {name: types.StringNull()},
		"unknown":               {name: types.StringUnknown()},
		"cuid shaped":           {name: types.StringValue("rtk6e7snm77j251v8qhqrk43"), wantError: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var resp validator.StringResponse
			notIDShapedValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("name"),
				ConfigValue: test.name,
			}, &resp)
			if got := resp.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("has error = %t, want %t: %v", got, test.wantError, resp.Diagnostics)
			}
		})
	}
}
