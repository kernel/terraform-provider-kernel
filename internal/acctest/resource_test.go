package acctest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestResourceStateValues(t *testing.T) {
	t.Parallel()

	state := testResourceState("kernel_project", "project-1", map[string]string{"name": "test"})
	id, attributes, err := ResourceStateValues(state, "kernel_project.test")
	if err != nil {
		t.Fatalf("ResourceStateValues() error = %v", err)
	}
	if id != "project-1" {
		t.Fatalf("ResourceStateValues() ID = %q, want %q", id, "project-1")
	}
	if attributes["name"] != "test" {
		t.Fatalf("ResourceStateValues() name = %q, want %q", attributes["name"], "test")
	}
}

func TestResourceStateValuesRejectsMissingState(t *testing.T) {
	t.Parallel()

	missingPrimary := terraform.NewState()
	missingPrimary.RootModule().Resources["kernel_project.test"] = &terraform.ResourceState{Type: "kernel_project"}
	tests := map[string]*terraform.State{
		"resource":    terraform.NewState(),
		"primary":     missingPrimary,
		"resource ID": testResourceState("kernel_project", "", nil),
	}
	for name, state := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := ResourceStateValues(state, "kernel_project.test")
			if err == nil {
				t.Fatal("ResourceStateValues() error = nil, want state validation error")
			}
		})
	}
}

func TestCaptureResourceIDRegistersCleanupOnceAndRejectsReplacement(t *testing.T) {
	t.Parallel()

	var capturedID string
	var cleanupIDs []string
	capture := CaptureResourceID(t, "kernel_project.test", &capturedID, func(id string, _ map[string]string) {
		cleanupIDs = append(cleanupIDs, id)
	})

	if err := capture(testResourceState("kernel_project", "project-1", nil)); err != nil {
		t.Fatalf("first capture error = %v", err)
	}
	if err := capture(testResourceState("kernel_project", "project-1", nil)); err != nil {
		t.Fatalf("second capture error = %v", err)
	}
	if capturedID != "project-1" {
		t.Fatalf("captured ID = %q, want %q", capturedID, "project-1")
	}
	if len(cleanupIDs) != 1 || cleanupIDs[0] != "project-1" {
		t.Fatalf("cleanup IDs = %v, want [project-1]", cleanupIDs)
	}

	err := capture(testResourceState("kernel_project", "project-2", nil))
	if err == nil || !strings.Contains(err.Error(), "ID changed") {
		t.Fatalf("replacement capture error = %v, want ID changed error", err)
	}
}

func TestCheckResourceDestroyed(t *testing.T) {
	t.Parallel()

	state := testResourceState("kernel_project", "project-1", map[string]string{"name": "test"})
	check := CheckResourceDestroyed("kernel_project", func(_ context.Context, id string, attributes map[string]string) error {
		if id != "project-1" || attributes["name"] != "test" {
			t.Fatalf("read state = (%q, %v), want project-1 and name=test", id, attributes)
		}
		return notFoundAPIError()
	})
	if err := check(state); err != nil {
		t.Fatalf("CheckResourceDestroyed() error = %v", err)
	}
}

func TestCheckResourceDestroyedReportsExistingAndReadErrors(t *testing.T) {
	t.Parallel()

	state := testResourceState("kernel_project", "project-1", nil)
	if err := CheckResourceDestroyed("kernel_project", func(context.Context, string, map[string]string) error {
		return nil
	})(state); err == nil || !strings.Contains(err.Error(), "still exists") {
		t.Fatalf("existing resource error = %v, want still exists error", err)
	}

	readErr := errors.New("read failed")
	if err := CheckResourceDestroyed("kernel_project", func(context.Context, string, map[string]string) error {
		return readErr
	})(state); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v, want wrapped %v", err, readErr)
	}
}

func testResourceState(resourceType, id string, attributes map[string]string) *terraform.State {
	state := terraform.NewState()
	state.RootModule().Resources[resourceType+".test"] = &terraform.ResourceState{
		Type: resourceType,
		Primary: &terraform.InstanceState{
			ID:         id,
			Attributes: attributes,
		},
	}
	return state
}
