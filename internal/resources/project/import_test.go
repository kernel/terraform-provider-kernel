package project

import "testing"

func TestProjectImportStateStoresCanonicalIDForRead(t *testing.T) {
	t.Parallel()

	for name, id := range map[string]string{
		"canonical id":         "project_123",
		"preserves whitespace": " project_123 ",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, diags := projectImportState(id)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := state.ID.ValueString(); got != id {
				t.Fatalf("state id = %q, want exact import id %q", got, id)
			}
			if !state.Name.IsUnknown() {
				t.Fatalf("state name = %v, want unknown until Read", state.Name)
			}
		})
	}
}

func TestProjectImportStateRejectsEmptyID(t *testing.T) {
	t.Parallel()

	state, diags := projectImportState("")
	if !diags.HasError() || diags[0].Summary() != "Invalid Kernel Project Import ID" {
		t.Fatalf("diagnostics = %v, want invalid import id error", diags)
	}
	if !state.ID.IsNull() || !state.Name.IsNull() {
		t.Fatalf("state = %+v, want empty state", state)
	}
}
