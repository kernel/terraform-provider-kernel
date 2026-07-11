package project_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

const projectDataSourceFixtureName = "kernel_project.data_source_test"

func TestAccProjectDataSourceByIDNameAndProviderDefault(t *testing.T) {
	name := acctest.UniqueName(t, "project-data")
	config := testAccProjectDataSourceConfig(name)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if os.Getenv(acctest.EnvProjectID) == "" {
				t.Fatalf("%s must be set for the project data source acceptance test", acctest.EnvProjectID)
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckProjectDataSourceFixtureDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureProjectDataSourceFixture(t),
					resource.TestCheckResourceAttrPair("data.kernel_project.by_id", "id", projectDataSourceFixtureName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_project.by_id", "name", projectDataSourceFixtureName, "name"),
					resource.TestCheckResourceAttrPair("data.kernel_project.by_name", "id", projectDataSourceFixtureName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_project.by_name", "name", projectDataSourceFixtureName, "name"),
					resource.TestCheckResourceAttr("data.kernel_project.current", "id", os.Getenv(acctest.EnvProjectID)),
					resource.TestCheckResourceAttrSet("data.kernel_project.by_id", "created_at"),
					resource.TestCheckResourceAttrSet("data.kernel_project.by_id", "updated_at"),
					resource.TestCheckResourceAttrSet("data.kernel_project.by_id", "status"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccProjectDataSourceConfig(name string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_project" "data_source_test" {
  name = %q
}

data "kernel_project" "by_id" {
  id = kernel_project.data_source_test.id
}

data "kernel_project" "by_name" {
  name = kernel_project.data_source_test.name
}

data "kernel_project" "current" {}
`, name)
}

func testAccCaptureProjectDataSourceFixture(t *testing.T) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[projectDataSourceFixtureName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("missing ID for %s", projectDataSourceFixtureName)
		}
		acctest.CleanupProject(t, resourceState.Primary.ID)
		return nil
	}
}

func testAccCheckProjectDataSourceFixtureDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[projectDataSourceFixtureName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := acctest.ClientFromEnv().GetProject(ctx, resourceState.Primary.ID)
		if projectscope.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Kernel project %s after destroy: %w", resourceState.Primary.ID, err)
		}
		return fmt.Errorf("Kernel project %s still exists after destroy", resourceState.Primary.ID)
	}
}
