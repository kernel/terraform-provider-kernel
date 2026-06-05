package browserpool

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tfattr "github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSchemaContainsOnlyDurableAttributes(t *testing.T) {
	s := BrowserPoolSchema()

	want := map[string]struct{}{
		"id":                   {},
		"name":                 {},
		"project_id":           {},
		"size":                 {},
		"profile_id":           {},
		"proxy_id":             {},
		"extension_ids":        {},
		"chrome_policy":        {},
		"viewport":             {},
		"headless":             {},
		"kiosk_mode":           {},
		"stealth":              {},
		"start_url":            {},
		"timeout_seconds":      {},
		"fill_rate_per_minute": {},
	}

	for name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Fatalf("expected schema attribute %q", name)
		}
	}

	for name := range s.Attributes {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected schema attribute %q", name)
		}
	}

	for _, forbidden := range []string{
		"acquired_count",
		"available_count",
		"discard_all_idle",
		"force_delete",
		"acquire",
		"release",
		"flush",
		"standby",
	} {
		if _, ok := s.Attributes[forbidden]; ok {
			t.Fatalf("schema must not expose runtime/control attribute %q", forbidden)
		}
	}
}

func TestSchemaRequiredComputedOptionalSemantics(t *testing.T) {
	s := BrowserPoolSchema()

	assertStringAttribute(t, s, "id", func(attr rschema.StringAttribute) bool {
		return attr.Computed && !attr.Optional && !attr.Required
	})
	assertStringAttribute(t, s, "name", func(attr rschema.StringAttribute) bool {
		return attr.Optional && !attr.Computed && !attr.Required
	})
	assertInt64Attribute(t, s, "size", func(attr rschema.Int64Attribute) bool {
		return attr.Required && !attr.Optional && !attr.Computed
	})
}

func TestSchemaProjectIDSemantics(t *testing.T) {
	s := BrowserPoolSchema()

	attr, ok := s.Attributes["project_id"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("project_id is not a string attribute")
	}
	if !attr.Optional || !attr.Computed || attr.Required {
		t.Fatal("project_id should be optional and computed so the resolved project is stored in state")
	}

	planned, requiresReplace := runProjectIDPlanModifiers(t, attr,
		types.StringValue("proj_a"), types.StringValue("proj_b"), types.StringValue("proj_b"))
	if !requiresReplace {
		t.Fatal("changing project_id must replace the pool; pools cannot move between projects")
	}
	if !planned.Equal(types.StringValue("proj_b")) {
		t.Fatalf("planned project_id = %v, want proj_b", planned)
	}

	planned, requiresReplace = runProjectIDPlanModifiers(t, attr,
		types.StringValue("proj_a"), types.StringUnknown(), types.StringNull())
	if requiresReplace {
		t.Fatal("unset project_id must not replace a pool with an inherited project")
	}
	if !planned.Equal(types.StringValue("proj_a")) {
		t.Fatalf("unset project_id should keep the state value, got %v", planned)
	}

	planned, requiresReplace = runProjectIDPlanModifiers(t, attr,
		types.StringNull(), types.StringUnknown(), types.StringNull())
	if requiresReplace {
		t.Fatal("unscoped pools must not plan replacement on refresh")
	}
	if !planned.IsNull() {
		t.Fatalf("unscoped project_id should stay null, got %v", planned)
	}
}

func runProjectIDPlanModifiers(t *testing.T, attr rschema.StringAttribute, state, plan, config types.String) (types.String, bool) {
	t.Helper()

	nonNullRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	req := planmodifier.StringRequest{
		State:       tfsdk.State{Raw: nonNullRaw},
		Plan:        tfsdk.Plan{Raw: nonNullRaw},
		StateValue:  state,
		PlanValue:   plan,
		ConfigValue: config,
	}

	requiresReplace := false
	for _, m := range attr.PlanModifiers {
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		m.PlanModifyString(context.Background(), req, resp)
		req.PlanValue = resp.PlanValue
		requiresReplace = requiresReplace || resp.RequiresReplace
	}
	return req.PlanValue, requiresReplace
}

func TestSchemaValidatesDurableNumericBounds(t *testing.T) {
	s := BrowserPoolSchema()

	assertInt64Rejects(t, int64Attribute(t, s, "size"), "size", 0)
	assertInt64Accepts(t, int64Attribute(t, s, "size"), "size", 1)
	assertInt64Rejects(t, int64Attribute(t, s, "timeout_seconds"), "timeout_seconds", 9)
	assertInt64Accepts(t, int64Attribute(t, s, "timeout_seconds"), "timeout_seconds", 10)
	assertInt64Accepts(t, int64Attribute(t, s, "timeout_seconds"), "timeout_seconds", 259200)
	assertInt64Rejects(t, int64Attribute(t, s, "timeout_seconds"), "timeout_seconds", 259201)
	assertInt64Rejects(t, int64Attribute(t, s, "fill_rate_per_minute"), "fill_rate_per_minute", -1)
	assertInt64Accepts(t, int64Attribute(t, s, "fill_rate_per_minute"), "fill_rate_per_minute", 0)
	assertInt64Accepts(t, int64Attribute(t, s, "fill_rate_per_minute"), "fill_rate_per_minute", 100)

	viewport := singleNestedAttribute(t, s, "viewport")
	assertInt64Rejects(t, nestedInt64Attribute(t, viewport, "width"), "viewport.width", 0)
	assertInt64Accepts(t, nestedInt64Attribute(t, viewport, "width"), "viewport.width", 1)
	assertInt64Rejects(t, nestedInt64Attribute(t, viewport, "height"), "viewport.height", 0)
	assertInt64Accepts(t, nestedInt64Attribute(t, viewport, "height"), "viewport.height", 1)
	assertInt64Rejects(t, nestedInt64Attribute(t, viewport, "refresh_rate"), "viewport.refresh_rate", 0)
	assertInt64Accepts(t, nestedInt64Attribute(t, viewport, "refresh_rate"), "viewport.refresh_rate", 1)
}

func TestSchemaValidatesChromePolicyJSON(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "chrome_policy")
	if _, ok := attr.CustomType.(ChromePolicyType); !ok {
		t.Fatalf("chrome_policy custom type is %T, want ChromePolicyType", attr.CustomType)
	}

	assertStringRejects(t, attr, "chrome_policy", `{`)
	assertStringRejects(t, attr, "chrome_policy", `[]`)
	assertStringAccepts(t, attr, "chrome_policy", `{"HomepageLocation":"https://example.com"}`)
}

func TestSchemaValidatesNameAPIContract(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "name")

	assertStringRejects(t, attr, "name", "")
	assertStringRejects(t, attr, "name", "bad name")
	assertStringRejects(t, attr, "name", "bad/name")
	assertStringRejects(t, attr, "name", strings.Repeat("a", 256))
	assertStringRejects(t, attr, "name", "abcdefghijklmnopqrstuvwx")
	assertStringAccepts(t, attr, "name", "pool-1")
	assertStringAccepts(t, attr, "name", "pool.name_1")
}

func TestSchemaValidatesProfileIDNonEmpty(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "profile_id")

	assertStringRejects(t, attr, "profile_id", "")
	assertStringAccepts(t, attr, "profile_id", "profile-1")
}

func TestSchemaValidatesProxyIDNonEmpty(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "proxy_id")

	assertStringRejects(t, attr, "proxy_id", "")
	assertStringAccepts(t, attr, "proxy_id", "proxy-1")
}

func TestSchemaValidatesStartURLNonEmpty(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "start_url")

	assertStringRejects(t, attr, "start_url", "")
	assertStringAccepts(t, attr, "start_url", "https://example.com")
}

func TestSchemaValidatesExtensionIDsAreNonEmpty(t *testing.T) {
	attr := setAttribute(t, BrowserPoolSchema(), "extension_ids")

	assertSetRejects(t, attr, "extension_ids", stringSet(types.StringValue("")))
	assertSetRejects(t, attr, "extension_ids", stringSet(types.StringNull()))
	assertSetRejects(t, attr, "extension_ids", extensionIDSet(21))
	assertSetAccepts(t, attr, "extension_ids", stringSet(types.StringUnknown()))
	assertSetAccepts(t, attr, "extension_ids", stringSet(types.StringValue("ext-1")))
}

func assertStringAttribute(t *testing.T, s rschema.Schema, name string, check func(rschema.StringAttribute) bool) {
	t.Helper()

	attr := stringAttribute(t, s, name)
	if !check(attr) {
		t.Fatalf("attribute %q has unexpected required/optional/computed flags: %#v", name, attr)
	}
}

func assertInt64Attribute(t *testing.T, s rschema.Schema, name string, check func(rschema.Int64Attribute) bool) {
	t.Helper()

	attr := int64Attribute(t, s, name)
	if !check(attr) {
		t.Fatalf("attribute %q has unexpected required/optional/computed flags: %#v", name, attr)
	}
}

func stringAttribute(t *testing.T, s rschema.Schema, name string) rschema.StringAttribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want StringAttribute", name, s.Attributes[name])
	}
	return attr
}

func int64Attribute(t *testing.T, s rschema.Schema, name string) rschema.Int64Attribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.Int64Attribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want Int64Attribute", name, s.Attributes[name])
	}
	return attr
}

func setAttribute(t *testing.T, s rschema.Schema, name string) rschema.SetAttribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.SetAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want SetAttribute", name, s.Attributes[name])
	}
	return attr
}

func singleNestedAttribute(t *testing.T, s rschema.Schema, name string) rschema.SingleNestedAttribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want SingleNestedAttribute", name, s.Attributes[name])
	}
	return attr
}

func nestedInt64Attribute(t *testing.T, parent rschema.SingleNestedAttribute, name string) rschema.Int64Attribute {
	t.Helper()

	attr, ok := parent.Attributes[name].(rschema.Int64Attribute)
	if !ok {
		t.Fatalf("nested attribute %q has type %T, want Int64Attribute", name, parent.Attributes[name])
	}
	return attr
}

func assertInt64Rejects(t *testing.T, attr rschema.Int64Attribute, name string, value int64) {
	t.Helper()

	if !validateInt64(attr.Int64Validators(), name, value).HasError() {
		t.Fatalf("%s accepted %d, want validation error", name, value)
	}
}

func assertInt64Accepts(t *testing.T, attr rschema.Int64Attribute, name string, value int64) {
	t.Helper()

	if diags := validateInt64(attr.Int64Validators(), name, value); diags.HasError() {
		t.Fatalf("%s rejected %d: %v", name, value, diags)
	}
}

func validateInt64(validators []validator.Int64, name string, value int64) diag.Diagnostics {
	req := validator.Int64Request{
		Path:        path.Root(name),
		ConfigValue: types.Int64Value(value),
	}
	var resp validator.Int64Response
	for _, v := range validators {
		v.ValidateInt64(context.Background(), req, &resp)
	}
	return resp.Diagnostics
}

func assertStringRejects(t *testing.T, attr rschema.StringAttribute, name string, value string) {
	t.Helper()

	if !validateString(attr.StringValidators(), name, value).HasError() {
		t.Fatalf("%s accepted %q, want validation error", name, value)
	}
}

func assertStringAccepts(t *testing.T, attr rschema.StringAttribute, name string, value string) {
	t.Helper()

	if diags := validateString(attr.StringValidators(), name, value); diags.HasError() {
		t.Fatalf("%s rejected %q: %v", name, value, diags)
	}
}

func validateString(validators []validator.String, name string, value string) diag.Diagnostics {
	req := validator.StringRequest{
		Path:        path.Root(name),
		ConfigValue: types.StringValue(value),
	}
	var resp validator.StringResponse
	for _, v := range validators {
		v.ValidateString(context.Background(), req, &resp)
	}
	return resp.Diagnostics
}

func assertSetRejects(t *testing.T, attr rschema.SetAttribute, name string, value types.Set) {
	t.Helper()

	if !validateSet(attr.SetValidators(), name, value).HasError() {
		t.Fatalf("%s accepted %#v, want validation error", name, value)
	}
}

func assertSetAccepts(t *testing.T, attr rschema.SetAttribute, name string, value types.Set) {
	t.Helper()

	if diags := validateSet(attr.SetValidators(), name, value); diags.HasError() {
		t.Fatalf("%s rejected %#v: %v", name, value, diags)
	}
}

func validateSet(validators []validator.Set, name string, value types.Set) diag.Diagnostics {
	req := validator.SetRequest{
		Path:        path.Root(name),
		ConfigValue: value,
	}
	var resp validator.SetResponse
	for _, v := range validators {
		v.ValidateSet(context.Background(), req, &resp)
	}
	return resp.Diagnostics
}

func stringSet(values ...tfattr.Value) types.Set {
	return types.SetValueMust(types.StringType, values)
}

func extensionIDSet(count int) types.Set {
	values := make([]tfattr.Value, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, types.StringValue("ext-"+strconv.Itoa(i)))
	}
	return stringSet(values...)
}
