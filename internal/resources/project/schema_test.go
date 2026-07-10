package project

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchemaContainsOnlyDurableProjectAttributes(t *testing.T) {
	t.Parallel()

	schema := projectSchema()
	if len(schema.Attributes) != 2 {
		t.Fatalf("attribute count = %d, want 2", len(schema.Attributes))
	}

	id, ok := schema.Attributes["id"].(rschema.StringAttribute)
	if !ok || !id.Computed || id.Required || id.Optional {
		t.Fatalf("id has unexpected schema: %#v", schema.Attributes["id"])
	}

	name, ok := schema.Attributes["name"].(rschema.StringAttribute)
	if !ok || !name.Required || name.Computed || name.Optional {
		t.Fatalf("name has unexpected schema: %#v", schema.Attributes["name"])
	}

}

func TestSchemaValidatesProjectNameLength(t *testing.T) {
	t.Parallel()

	name := projectSchema().Attributes["name"].(rschema.StringAttribute)
	for _, test := range []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "empty", value: "", wantError: true},
		{name: "one character", value: "a"},
		{name: "maximum", value: strings.Repeat("a", maxProjectNameLength)},
		{name: "too long", value: strings.Repeat("a", maxProjectNameLength+1), wantError: true},
		{name: "multibyte over byte limit", value: strings.Repeat("é", maxProjectNameLength/2+1), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diags := validateString(name.Validators, test.value)
			if diags.HasError() != test.wantError {
				t.Fatalf("validation diagnostics = %v, want error %t", diags, test.wantError)
			}
		})
	}
}

func validateString(validators []validator.String, value string) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, candidate := range validators {
		req := validator.StringRequest{ConfigValue: types.StringValue(value)}
		var resp validator.StringResponse
		candidate.ValidateString(context.Background(), req, &resp)
		diags.Append(resp.Diagnostics...)
	}
	return diags
}
