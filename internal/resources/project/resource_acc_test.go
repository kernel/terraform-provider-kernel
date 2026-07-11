package project_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const projectResourceName = "kernel_project.test"

func TestAccProjectLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "project")
	updatedName := acctest.UniqueName(t, "project-updated")
	updatedConfig := testAccProjectConfig(updatedName)
	var projectID string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckProjectDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureProjectID(t, projectResourceName, &projectID),
					resource.TestCheckResourceAttrSet(projectResourceName, "id"),
					resource.TestCheckResourceAttr(projectResourceName, "name", name),
				),
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureProjectID(t, projectResourceName, &projectID),
					resource.TestCheckResourceAttr(projectResourceName, "name", updatedName),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:       projectResourceName,
				ImportState:        true,
				ImportStateVerify:  true,
				ImportStatePersist: true,
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
		},
	})
}

func testAccProjectConfig(name string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_project" "test" {
  name = %q
}
`, name)
}

func testAccCaptureProjectID(t *testing.T, resourceName string, projectID *string) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		id, err := projectStateID(state, resourceName)
		if err != nil {
			return err
		}
		if *projectID != "" && *projectID != id {
			return fmt.Errorf("Kernel project ID changed from %s to %s", *projectID, id)
		}
		if *projectID == "" {
			*projectID = id
			acctest.CleanupProject(t, id)
		}
		return nil
	}
}

func testAccCheckProjectDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != "kernel_project" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			_, err := client.GetProject(ctx, resourceState.Primary.ID)
			if acctest.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read Kernel project %s after destroy: %w", resourceState.Primary.ID, err)
			}
			return fmt.Errorf("Kernel project %s still exists after destroy", resourceState.Primary.ID)
		}
		return nil
	}
}

func projectStateID(state *terraform.State, resourceName string) (string, error) {
	resourceState, ok := state.RootModule().Resources[resourceName]
	if !ok {
		return "", fmt.Errorf("missing resource %s in Terraform state", resourceName)
	}
	if resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", fmt.Errorf("missing ID for %s in Terraform state", resourceName)
	}
	return resourceState.Primary.ID, nil
}
