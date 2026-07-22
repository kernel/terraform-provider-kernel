package acctest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const resourceReadTimeout = 30 * time.Second

// ResourceStateValues returns the ID and attributes for one Terraform resource.
func ResourceStateValues(state *terraform.State, resourceName string) (string, map[string]string, error) {
	resourceState, ok := state.RootModule().Resources[resourceName]
	if !ok {
		return "", nil, fmt.Errorf("missing resource %s in Terraform state", resourceName)
	}
	if resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", nil, fmt.Errorf("missing ID for %s in Terraform state", resourceName)
	}
	return resourceState.Primary.ID, resourceState.Primary.Attributes, nil
}

// CaptureResourceID verifies that an in-place lifecycle keeps a stable ID and
// registers independent cleanup after the first successful state read.
func CaptureResourceID(
	t testing.TB,
	resourceName string,
	resourceID *string,
	cleanup func(id string, attributes map[string]string),
) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		id, attributes, err := ResourceStateValues(state, resourceName)
		if err != nil {
			return err
		}
		if *resourceID != "" && *resourceID != id {
			return fmt.Errorf("%s ID changed from %s to %s", resourceName, *resourceID, id)
		}
		if *resourceID == "" {
			*resourceID = id
			cleanup(id, attributes)
		}
		return nil
	}
}

// CheckResourceDestroyed verifies that every resource of resourceType is absent.
func CheckResourceDestroyed(
	resourceType string,
	read func(context.Context, string, map[string]string) error,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), resourceReadTimeout)
		defer cancel()

		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != resourceType || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			id := resourceState.Primary.ID
			err := read(ctx, id, resourceState.Primary.Attributes)
			if IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read %s %s after destroy: %w", resourceType, id, err)
			}
			return fmt.Errorf("%s %s still exists after destroy", resourceType, id)
		}
		return nil
	}
}
