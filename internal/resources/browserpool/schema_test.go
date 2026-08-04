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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSchemaContainsOnlySupportedAttributes(t *testing.T) {
	s := BrowserPoolSchema()

	want := map[string]struct{}{
		"id":                              {},
		"name":                            {},
		"project_id":                      {},
		"size":                            {},
		"profile_id":                      {},
		"proxy_id":                        {},
		"extension_ids":                   {},
		"chrome_policy":                   {},
		"viewport":                        {},
		"headless":                        {},
		"kiosk_mode":                      {},
		"stealth":                         {},
		"start_url":                       {},
		"timeout_seconds":                 {},
		"fill_rate_per_minute":            {},
		"rebuild_idle_browsers_on_update": {},
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
	assertBoolAttribute(t, s, "headless", func(attr rschema.BoolAttribute) bool {
		return attr.Optional && attr.Computed && !attr.Required
	})
	assertBoolAttribute(t, s, "kiosk_mode", func(attr rschema.BoolAttribute) bool {
		return attr.Optional && attr.Computed && !attr.Required
	})
	assertBoolAttribute(t, s, "stealth", func(attr rschema.BoolAttribute) bool {
		return attr.Optional && attr.Computed && !attr.Required
	})
	assertInt64Attribute(t, s, "timeout_seconds", func(attr rschema.Int64Attribute) bool {
		return attr.Optional && attr.Computed && !attr.Required
	})
	assertInt64Attribute(t, s, "fill_rate_per_minute", func(attr rschema.Int64Attribute) bool {
		return attr.Optional && attr.Computed && !attr.Required
	})
	assertBoolAttribute(t, s, "rebuild_idle_browsers_on_update", func(attr rschema.BoolAttribute) bool {
		return attr.Optional && attr.Computed && !attr.Required && attr.Default != nil
	})

	viewport := singleNestedAttribute(t, s, "viewport")
	refreshRate := nestedInt64Attribute(t, viewport, "refresh_rate")
	if !refreshRate.Optional || !refreshRate.Computed || refreshRate.Required {
		t.Fatalf("viewport.refresh_rate has unexpected flags: %#v", refreshRate)
	}
}

func TestSchemaIDKeepsStateDuringUpdate(t *testing.T) {
	t.Parallel()

	s := BrowserPoolSchema()

	attr, ok := s.Attributes["id"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("id is not a string attribute")
	}

	planned, requiresReplace := runStringPlanModifiers(t, attr,
		types.StringValue("pool-1"), types.StringUnknown(), types.StringNull())
	if requiresReplace {
		t.Fatal("an unknown id during update must not replace the pool")
	}
	if !planned.Equal(types.StringValue("pool-1")) {
		t.Fatalf("planned id = %v, want pool-1 from state", planned)
	}
}

func TestSchemaRebuildIdleBrowsersOnUpdateDefaultsFalse(t *testing.T) {
	t.Parallel()

	attr := boolAttribute(t, BrowserPoolSchema(), "rebuild_idle_browsers_on_update")
	if attr.Default == nil {
		t.Fatal("rebuild_idle_browsers_on_update has no default")
	}

	var resp defaults.BoolResponse
	attr.Default.DefaultBool(context.Background(), defaults.BoolRequest{
		Path: path.Root("rebuild_idle_browsers_on_update"),
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("default diagnostics: %v", resp.Diagnostics)
	}
	if !resp.PlanValue.Equal(types.BoolValue(false)) {
		t.Fatalf("rebuild_idle_browsers_on_update default = %v, want false", resp.PlanValue)
	}
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

	planned, requiresReplace := runStringPlanModifiers(t, attr,
		types.StringValue("proj_a"), types.StringValue("proj_b"), types.StringValue("proj_b"))
	if !requiresReplace {
		t.Fatal("changing project_id must replace the pool; pools cannot move between projects")
	}
	if !planned.Equal(types.StringValue("proj_b")) {
		t.Fatalf("planned project_id = %v, want proj_b", planned)
	}

	planned, requiresReplace = runStringPlanModifiers(t, attr,
		types.StringValue("proj_a"), types.StringUnknown(), types.StringNull())
	if requiresReplace {
		t.Fatal("unset project_id must not replace a pool with an inherited project")
	}
	if !planned.Equal(types.StringValue("proj_a")) {
		t.Fatalf("unset project_id should keep the state value, got %v", planned)
	}

	planned, requiresReplace = runStringPlanModifiers(t, attr,
		types.StringNull(), types.StringUnknown(), types.StringNull())
	if requiresReplace {
		t.Fatal("unscoped pools must not plan replacement on refresh")
	}
	if !planned.IsNull() {
		t.Fatalf("unscoped project_id should stay null, got %v", planned)
	}
}

func TestSchemaUnsupportedClearsPlanReplacement(t *testing.T) {
	s := BrowserPoolSchema()

	for _, name := range []string{"name"} {
		attr := stringAttribute(t, s, name)
		_, requiresReplace := runStringPlanModifiers(t, attr,
			types.StringValue("configured"), types.StringNull(), types.StringNull())
		if !requiresReplace {
			t.Fatalf("clearing %s must replace the pool", name)
		}

		_, requiresReplace = runStringPlanModifiers(t, attr,
			types.StringValue("old"), types.StringValue("new"), types.StringValue("new"))
		if requiresReplace {
			t.Fatalf("changing %s to another value must remain an in-place update", name)
		}
	}

	profile := stringAttribute(t, s, "profile_id")
	_, requiresReplace := runStringPlanModifiers(t, profile,
		types.StringValue("profile-1"), types.StringNull(), types.StringNull())
	if requiresReplace {
		t.Fatal("clearing profile_id must remain an in-place update")
	}

	viewport := singleNestedAttribute(t, s, "viewport")
	viewportValue := types.ObjectValueMust(
		map[string]tfattr.Type{
			"width": types.Int64Type, "height": types.Int64Type, "refresh_rate": types.Int64Type,
		},
		map[string]tfattr.Value{
			"width": types.Int64Value(1280), "height": types.Int64Value(800), "refresh_rate": types.Int64Value(60),
		},
	)
	if !runObjectPlanModifiers(t, viewport, viewportValue, types.ObjectNull(viewportValue.AttributeTypes(context.Background()))) {
		t.Fatal("clearing viewport must replace the pool")
	}
}

func runStringPlanModifiers(t *testing.T, attr rschema.StringAttribute, state, plan, config types.String) (types.String, bool) {
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
		if resp.Diagnostics.HasError() {
			t.Fatalf("plan modifier returned diagnostics: %v", resp.Diagnostics)
		}
		req.PlanValue = resp.PlanValue
		requiresReplace = requiresReplace || resp.RequiresReplace
	}
	return req.PlanValue, requiresReplace
}

func runObjectPlanModifiers(t *testing.T, attr rschema.SingleNestedAttribute, state, plan types.Object) bool {
	t.Helper()

	nonNullRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	req := planmodifier.ObjectRequest{
		State:      tfsdk.State{Raw: nonNullRaw},
		Plan:       tfsdk.Plan{Raw: nonNullRaw},
		StateValue: state,
		PlanValue:  plan,
	}

	requiresReplace := false
	for _, m := range attr.PlanModifiers {
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		m.PlanModifyObject(context.Background(), req, resp)
		requiresReplace = requiresReplace || resp.RequiresReplace
	}
	return requiresReplace
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
	if _, ok := attr.CustomType.(chromePolicyType); !ok {
		t.Fatalf("chrome_policy custom type is %T, want chromePolicyType", attr.CustomType)
	}

	assertStringRejects(t, attr, "chrome_policy", `{`)
	assertStringRejects(t, attr, "chrome_policy", `[]`)
	assertStringRejects(t, attr, "chrome_policy", `null`)
	assertStringRejects(t, attr, "chrome_policy", `{"Large":"`+strings.Repeat("a", maxChromePolicyBytes)+`"}`)
	assertStringAccepts(t, attr, "chrome_policy", `{"HomepageLocation":"https://example.com"}`)
}

func TestSchemaChromePolicyPreservesStateForEquivalentJSON(t *testing.T) {
	attr := stringAttribute(t, BrowserPoolSchema(), "chrome_policy")
	state := types.StringValue(`{"HomepageLocation":"https://example.com","RestoreOnStartup":4}`)
	config := types.StringValue(`{ "RestoreOnStartup": 4, "HomepageLocation": "https://example.com" }`)

	planned, requiresReplace := runStringPlanModifiers(t, attr, state, config, config)
	if requiresReplace {
		t.Fatal("equivalent chrome_policy JSON must not replace the pool")
	}
	if !planned.Equal(state) {
		t.Fatalf("equivalent chrome_policy JSON planned as %q, want prior state %q", planned.ValueString(), state.ValueString())
	}

	changed := types.StringValue(`{"HomepageLocation":"https://kernel.sh","RestoreOnStartup":4}`)
	planned, _ = runStringPlanModifiers(t, attr, state, changed, changed)
	if !planned.Equal(changed) {
		t.Fatalf("changed chrome_policy JSON planned as %q, want configured value %q", planned.ValueString(), changed.ValueString())
	}
}

func TestSchemaPreservesComputedDefaultsDuringUnrelatedUpdates(t *testing.T) {
	s := BrowserPoolSchema()

	for _, name := range []string{"headless", "kiosk_mode", "stealth"} {
		attr := boolAttribute(t, s, name)
		planned := runBoolPlanModifiers(t, attr, types.BoolValue(false), types.BoolUnknown(), types.BoolNull())
		if !planned.Equal(types.BoolValue(false)) {
			t.Fatalf("%s planned as %v, want prior false state", name, planned)
		}
	}

	for _, name := range []string{"timeout_seconds", "fill_rate_per_minute"} {
		attr := int64Attribute(t, s, name)
		planned := runInt64PlanModifiers(t, attr, types.Int64Value(42), types.Int64Unknown(), types.Int64Null())
		if !planned.Equal(types.Int64Value(42)) {
			t.Fatalf("%s planned as %v, want prior state", name, planned)
		}
	}

	viewport := singleNestedAttribute(t, s, "viewport")
	refreshRate := nestedInt64Attribute(t, viewport, "refresh_rate")
	planned := runInt64PlanModifiers(t, refreshRate, types.Int64Value(60), types.Int64Unknown(), types.Int64Null())
	if !planned.Equal(types.Int64Value(60)) {
		t.Fatalf("viewport.refresh_rate planned as %v, want prior state", planned)
	}
}

func TestSchemaLeavesNewViewportRefreshRateUnknown(t *testing.T) {
	viewport := singleNestedAttribute(t, BrowserPoolSchema(), "viewport")
	refreshRate := nestedInt64Attribute(t, viewport, "refresh_rate")
	planned := runInt64PlanModifiers(t, refreshRate, types.Int64Null(), types.Int64Unknown(), types.Int64Null())
	if !planned.IsUnknown() {
		t.Fatalf("new viewport refresh_rate planned as %v, want unknown for the API default", planned)
	}
}

func TestSchemaPreservesImportedEmptyExtensionIDsWhenConfigurationOmitsThem(t *testing.T) {
	attr := listAttribute(t, BrowserPoolSchema(), "extension_ids")
	if !attr.Optional || !attr.Computed || attr.Required {
		t.Fatalf("extension_ids must be optional and computed, got %#v", attr)
	}
	empty := types.ListValueMust(types.StringType, []tfattr.Value{})

	planned := runListPlanModifiers(t, attr, empty, types.ListUnknown(types.StringType), types.ListNull(types.StringType))
	if !planned.Equal(empty) {
		t.Fatalf("empty extension_ids planned as %v, want prior empty state when omitted", planned)
	}
}

func TestSchemaDefaultsOmittedExtensionIDsOnlyDuringCreate(t *testing.T) {
	attr := listAttribute(t, BrowserPoolSchema(), "extension_ids")
	empty := types.ListValueMust(types.StringType, []tfattr.Value{})
	nullResource := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, nil)

	planned := runListPlanModifiersWithStateRaw(t, attr, nullResource,
		types.ListNull(types.StringType), types.ListUnknown(types.StringType), types.ListNull(types.StringType))
	if !planned.Equal(empty) {
		t.Fatalf("omitted extension_ids planned as %v during create, want empty list", planned)
	}

	planned = runListPlanModifiersWithStateRaw(t, attr, nullResource,
		types.ListNull(types.StringType), types.ListUnknown(types.StringType), types.ListUnknown(types.StringType))
	if !planned.IsUnknown() {
		t.Fatalf("unknown configured extension_ids planned as %v, want unknown preserved", planned)
	}
}

func runBoolPlanModifiers(t *testing.T, attr rschema.BoolAttribute, state, plan, config types.Bool) types.Bool {
	t.Helper()
	nonNullRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	req := planmodifier.BoolRequest{
		State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw},
		StateValue: state, PlanValue: plan, ConfigValue: config,
	}
	for _, m := range attr.PlanModifiers {
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		m.PlanModifyBool(context.Background(), req, resp)
		req.PlanValue = resp.PlanValue
	}
	return req.PlanValue
}

func runInt64PlanModifiers(t *testing.T, attr rschema.Int64Attribute, state, plan, config types.Int64) types.Int64 {
	t.Helper()
	nonNullRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	req := planmodifier.Int64Request{
		State: tfsdk.State{Raw: nonNullRaw}, Plan: tfsdk.Plan{Raw: nonNullRaw},
		StateValue: state, PlanValue: plan, ConfigValue: config,
	}
	for _, m := range attr.PlanModifiers {
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
		m.PlanModifyInt64(context.Background(), req, resp)
		req.PlanValue = resp.PlanValue
	}
	return req.PlanValue
}

func runListPlanModifiers(t *testing.T, attr rschema.ListAttribute, state, plan, config types.List) types.List {
	t.Helper()
	nonNullRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	return runListPlanModifiersWithStateRaw(t, attr, nonNullRaw, state, plan, config)
}

func runListPlanModifiersWithStateRaw(t *testing.T, attr rschema.ListAttribute, stateRaw tftypes.Value, state, plan, config types.List) types.List {
	t.Helper()
	req := planmodifier.ListRequest{
		State:      tfsdk.State{Raw: stateRaw},
		StateValue: state, PlanValue: plan, ConfigValue: config,
	}
	for _, m := range attr.PlanModifiers {
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		m.PlanModifyList(context.Background(), req, resp)
		req.PlanValue = resp.PlanValue
	}
	return req.PlanValue
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
	assertStringRejects(t, attr, "start_url", strings.Repeat("a", maxStartURLBytes+1))
	assertStringAccepts(t, attr, "start_url", strings.Repeat("a", maxStartURLBytes))
	assertStringAccepts(t, attr, "start_url", "https://example.com")
}

func TestSchemaValidatesExtensionIDsAreNonEmpty(t *testing.T) {
	attr := listAttribute(t, BrowserPoolSchema(), "extension_ids")

	assertListRejects(t, attr, "extension_ids", stringList(types.StringValue("")))
	assertListRejects(t, attr, "extension_ids", stringList(types.StringNull()))
	assertListRejects(t, attr, "extension_ids", extensionIDList(21))
	assertListAccepts(t, attr, "extension_ids", stringList(types.StringUnknown()))
	assertListAccepts(t, attr, "extension_ids", stringList(types.StringValue("ext-1")))
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

func assertBoolAttribute(t *testing.T, s rschema.Schema, name string, check func(rschema.BoolAttribute) bool) {
	t.Helper()

	attr := boolAttribute(t, s, name)
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

func boolAttribute(t *testing.T, s rschema.Schema, name string) rschema.BoolAttribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.BoolAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want BoolAttribute", name, s.Attributes[name])
	}
	return attr
}

func listAttribute(t *testing.T, s rschema.Schema, name string) rschema.ListAttribute {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.ListAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want ListAttribute", name, s.Attributes[name])
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

func assertListRejects(t *testing.T, attr rschema.ListAttribute, name string, value types.List) {
	t.Helper()

	if !validateList(attr.ListValidators(), name, value).HasError() {
		t.Fatalf("%s accepted %#v, want validation error", name, value)
	}
}

func assertListAccepts(t *testing.T, attr rschema.ListAttribute, name string, value types.List) {
	t.Helper()

	if diags := validateList(attr.ListValidators(), name, value); diags.HasError() {
		t.Fatalf("%s rejected %#v: %v", name, value, diags)
	}
}

func validateList(validators []validator.List, name string, value types.List) diag.Diagnostics {
	req := validator.ListRequest{
		Path:        path.Root(name),
		ConfigValue: value,
	}
	var resp validator.ListResponse
	for _, v := range validators {
		v.ValidateList(context.Background(), req, &resp)
	}
	return resp.Diagnostics
}

func stringList(values ...tfattr.Value) types.List {
	return types.ListValueMust(types.StringType, values)
}

func extensionIDList(count int) types.List {
	values := make([]tfattr.Value, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, types.StringValue("ext-"+strconv.Itoa(i)))
	}
	return stringList(values...)
}
