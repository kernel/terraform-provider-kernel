package extension

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestExtensionSchemaImplementationIsValid(t *testing.T) {
	if diagnostics := extensionSchema().ValidateImplementation(context.Background()); diagnostics.HasError() {
		t.Fatalf("schema validation failed: %v", diagnostics)
	}
}

func TestExtensionSchemaContainsOnlyDurableAttributes(t *testing.T) {
	want := map[string]struct{}{
		"id":            {},
		"name":          {},
		"project_id":    {},
		"source_path":   {},
		"source_sha256": {},
	}

	attributes := extensionSchema().Attributes
	for name := range want {
		if _, ok := attributes[name]; !ok {
			t.Fatalf("expected schema attribute %q", name)
		}
	}
	for name := range attributes {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected schema attribute %q", name)
		}
	}
}

func TestExtensionSchemaAttributeSemantics(t *testing.T) {
	schema := extensionSchema()

	id := extensionStringAttribute(t, schema, "id")
	if !id.Computed || id.Optional || id.Required || id.WriteOnly {
		t.Fatalf("id has unexpected flags: %#v", id)
	}

	for _, name := range []string{"name", "project_id", "source_sha256"} {
		attribute := extensionStringAttribute(t, schema, name)
		if !attribute.Optional || !attribute.Computed || attribute.Required || attribute.WriteOnly {
			t.Fatalf("%s has unexpected flags: %#v", name, attribute)
		}
	}

	sourcePath := extensionStringAttribute(t, schema, "source_path")
	if !sourcePath.Optional || sourcePath.Computed || sourcePath.Required || !sourcePath.WriteOnly {
		t.Fatalf("source_path has unexpected flags: %#v", sourcePath)
	}
}

func TestExtensionSchemaImmutableAttributePlanSemantics(t *testing.T) {
	for _, name := range []string{"name", "project_id", "source_sha256"} {
		attribute := extensionStringAttribute(t, extensionSchema(), name)

		planned, replace := runExtensionStringPlanModifiers(
			t,
			attribute,
			types.StringValue("old"),
			types.StringValue("new"),
			types.StringValue("new"),
		)
		if !replace {
			t.Errorf("changing %s must require replacement", name)
		}
		if !planned.Equal(types.StringValue("new")) {
			t.Errorf("planned %s = %v, want new", name, planned)
		}

		planned, replace = runExtensionStringPlanModifiers(
			t,
			attribute,
			types.StringNull(),
			types.StringValue("new"),
			types.StringValue("new"),
		)
		if !replace {
			t.Errorf("configuring %s for previously null state must require replacement", name)
		}
		if !planned.Equal(types.StringValue("new")) {
			t.Errorf("newly configured %s planned value = %v, want new", name, planned)
		}

		planned, replace = runExtensionStringPlanModifiers(
			t,
			attribute,
			types.StringValue("resolved"),
			types.StringUnknown(),
			types.StringNull(),
		)
		if replace {
			t.Errorf("unset %s must preserve imported or resolved state without replacement", name)
		}
		if !planned.Equal(types.StringValue("resolved")) {
			t.Errorf("unset %s planned value = %v, want resolved state", name, planned)
		}
	}
}

func TestExtensionSchemaNameValidation(t *testing.T) {
	attribute := extensionStringAttribute(t, extensionSchema(), "name")

	for _, value := range []string{"", "bad name", "bad/name", strings.Repeat("a", 256), "abcdefghijklmnopqrstuvwx"} {
		assertExtensionStringRejected(t, attribute, "name", value)
	}
	for _, value := range []string{"extension-1", "extension.name_1", "ABCDEFGHIJKLMNOPQRSTUVWX"} {
		assertExtensionStringAccepted(t, attribute, "name", value)
	}
}

func TestExtensionSchemaChecksumValidation(t *testing.T) {
	attribute := extensionStringAttribute(t, extensionSchema(), "source_sha256")

	assertExtensionStringRejected(t, attribute, "source_sha256", "")
	assertExtensionStringRejected(t, attribute, "source_sha256", strings.Repeat("a", 63))
	assertExtensionStringRejected(t, attribute, "source_sha256", strings.Repeat("A", 64))
	assertExtensionStringRejected(t, attribute, "source_sha256", strings.Repeat("g", 64))
	assertExtensionStringAccepted(t, attribute, "source_sha256", strings.Repeat("a", 64))
	assertExtensionStringAccepted(t, attribute, "source_sha256", strings.Repeat("0", 64))
}

func TestExtensionSchemaProjectAndPathValidation(t *testing.T) {
	for _, name := range []string{"project_id", "source_path"} {
		attribute := extensionStringAttribute(t, extensionSchema(), name)
		assertExtensionStringRejected(t, attribute, name, "")
		assertExtensionStringAccepted(t, attribute, name, "value")
	}
}

func extensionStringAttribute(t *testing.T, schema rschema.Schema, name string) rschema.StringAttribute {
	t.Helper()

	attribute, ok := schema.Attributes[name].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want StringAttribute", name, schema.Attributes[name])
	}
	return attribute
}

func runExtensionStringPlanModifiers(t *testing.T, attribute rschema.StringAttribute, state, plan, config types.String) (types.String, bool) {
	t.Helper()

	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	request := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: raw},
		Plan:        tfsdk.Plan{Raw: raw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: config,
	}

	requiresReplace := false
	for _, modifier := range attribute.PlanModifiers {
		response := &planmodifier.StringResponse{PlanValue: request.PlanValue}
		modifier.PlanModifyString(context.Background(), request, response)
		request.PlanValue = response.PlanValue
		requiresReplace = requiresReplace || response.RequiresReplace
	}
	return request.PlanValue, requiresReplace
}

func assertExtensionStringRejected(t *testing.T, attribute rschema.StringAttribute, name, value string) {
	t.Helper()

	diagnostics := validateExtensionString(attribute.Validators, name, types.StringValue(value))
	if !diagnostics.HasError() {
		t.Fatalf("expected %s value %q to be rejected", name, value)
	}
}

func assertExtensionStringAccepted(t *testing.T, attribute rschema.StringAttribute, name, value string) {
	t.Helper()

	diagnostics := validateExtensionString(attribute.Validators, name, types.StringValue(value))
	if diagnostics.HasError() {
		t.Fatalf("expected %s value %q to be accepted, got %v", name, value, diagnostics)
	}
}

func validateExtensionString(validators []validator.String, name string, value types.String) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	for _, stringValidator := range validators {
		var response validator.StringResponse
		stringValidator.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: value,
			Path:        path.Root(name),
		}, &response)
		diagnostics.Append(response.Diagnostics...)
	}
	return diagnostics
}
