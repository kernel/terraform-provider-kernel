package extension_test

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

const extensionDataSourceFixtureName = "kernel_extension.data_source_test"

func TestAccExtensionDataSourceByIDAndName(t *testing.T) {
	name := acctest.UniqueName(t, "extension-data")
	sourcePath, _ := acctest.ExtensionArchive(t, "data-source")
	projectID := os.Getenv(acctest.EnvProjectID)
	config := testAccExtensionDataSourceConfig(name, sourcePath, projectID)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if projectID == "" {
				t.Fatalf("%s must be set for the extension data source acceptance test", acctest.EnvProjectID)
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckExtensionDataSourceFixtureDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureExtensionDataSourceFixture(t),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_id", "id", extensionDataSourceFixtureName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_id", "name", extensionDataSourceFixtureName, "name"),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_name", "id", extensionDataSourceFixtureName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_name", "name", extensionDataSourceFixtureName, "name"),
					resource.TestCheckResourceAttr("data.kernel_extension.by_id", "project_id", projectID),
					resource.TestCheckNoResourceAttr("data.kernel_extension.by_name", "project_id"),
					resource.TestCheckResourceAttrSet("data.kernel_extension.by_id", "created_at"),
					resource.TestCheckResourceAttrSet("data.kernel_extension.by_id", "size_bytes"),
					resource.TestCheckNoResourceAttr("data.kernel_extension.by_id", "last_used_at"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccExtensionDataSourceConfig(name, sourcePath, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_extension" "data_source_test" {
  name          = %[1]q
  source_path   = %[2]q
  source_sha256 = filesha256(%[2]q)
}

data "kernel_extension" "by_id" {
  id         = kernel_extension.data_source_test.id
  project_id = %[3]q
}

data "kernel_extension" "by_name" {
  name = kernel_extension.data_source_test.name
}
`, name, sourcePath, projectID)
}

func testAccCaptureExtensionDataSourceFixture(t *testing.T) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[extensionDataSourceFixtureName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("missing ID for %s", extensionDataSourceFixtureName)
		}
		acctest.CleanupExtension(t, resourceState.Primary.Attributes["project_id"], resourceState.Primary.ID)
		return nil
	}
}

func testAccCheckExtensionDataSourceFixtureDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[extensionDataSourceFixtureName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		projectID := resourceState.Primary.Attributes["project_id"]
		_, err := acctest.ClientFromEnv().GetExtension(ctx, projectID, resourceState.Primary.ID)
		if projectscope.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Kernel extension %s after destroy: %w", resourceState.Primary.ID, err)
		}
		return fmt.Errorf("Kernel extension %s still exists after destroy", resourceState.Primary.ID)
	}
}
