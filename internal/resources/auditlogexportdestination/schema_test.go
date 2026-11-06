package auditlogexportdestination

import (
	"context"
	"testing"

	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchemaContainsOnlyDurableAttributes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		required, optional, computed bool
	}{
		"id":              {computed: true},
		"type":            {optional: true, computed: true},
		"format":          {optional: true, computed: true},
		"bucket":          {required: true},
		"region":          {required: true},
		"prefix":          {required: true},
		"role_arn":        {required: true},
		"kms_key_id":      {optional: true},
		"status":          {optional: true, computed: true},
		"external_id":     {computed: true},
		"kernel_role_arn": {computed: true},
	}

	attributes := destinationSchema().Attributes
	if len(attributes) != len(tests) {
		t.Fatalf("attribute count = %d, want %d", len(attributes), len(tests))
	}
	for name, want := range tests {
		attribute, ok := attributes[name].(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute: %#v", name, attributes[name])
		}
		if attribute.Required != want.required || attribute.Optional != want.optional || attribute.Computed != want.computed {
			t.Fatalf("%s flags = required %t, optional %t, computed %t; want %+v", name, attribute.Required, attribute.Optional, attribute.Computed, want)
		}
	}
}

func TestSchemaValidatesEnums(t *testing.T) {
	t.Parallel()

	attributes := destinationSchema().Attributes
	for _, test := range []struct {
		attribute, value string
		wantError        bool
	}{
		{attribute: "type", value: "s3"},
		{attribute: "type", value: "gcs", wantError: true},
		{attribute: "format", value: "jsonl.gz"},
		{attribute: "format", value: "csv", wantError: true},
		{attribute: "status", value: "active"},
		{attribute: "status", value: "paused"},
		{attribute: "status", value: "disabled", wantError: true},
		{attribute: "kms_key_id", value: "", wantError: true},
		{attribute: "bucket", value: "", wantError: true},
	} {
		t.Run(test.attribute+"="+test.value, func(t *testing.T) {
			t.Parallel()

			attribute := attributes[test.attribute].(rschema.StringAttribute)
			hasError := false
			for _, v := range attribute.Validators {
				resp := &validator.StringResponse{}
				v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(test.value)}, resp)
				hasError = hasError || resp.Diagnostics.HasError()
			}
			if hasError != test.wantError {
				t.Fatalf("validation error = %t, want %t", hasError, test.wantError)
			}
		})
	}
}
