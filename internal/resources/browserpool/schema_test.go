package browserpool

import (
	"testing"

	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestSchemaContainsOnlyDurableAttributes(t *testing.T) {
	s := BrowserPoolSchema()

	want := map[string]struct{}{
		"id":                   {},
		"name":                 {},
		"size":                 {},
		"profile_id":           {},
		"profile_save_changes": {},
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

func assertStringAttribute(t *testing.T, s rschema.Schema, name string, check func(rschema.StringAttribute) bool) {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want StringAttribute", name, s.Attributes[name])
	}
	if !check(attr) {
		t.Fatalf("attribute %q has unexpected required/optional/computed flags: %#v", name, attr)
	}
}

func assertInt64Attribute(t *testing.T, s rschema.Schema, name string, check func(rschema.Int64Attribute) bool) {
	t.Helper()

	attr, ok := s.Attributes[name].(rschema.Int64Attribute)
	if !ok {
		t.Fatalf("attribute %q has type %T, want Int64Attribute", name, s.Attributes[name])
	}
	if !check(attr) {
		t.Fatalf("attribute %q has unexpected required/optional/computed flags: %#v", name, attr)
	}
}
