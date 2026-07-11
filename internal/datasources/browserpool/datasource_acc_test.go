package browserpool_test

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

const browserPoolAcceptanceResourceName = "kernel_browser_pool.data_source_test"

func TestAccBrowserPoolDataSourceByIDAndName(t *testing.T) {
	name := acctest.UniqueName(t, "browser-pool-data")
	defaultProjectID := os.Getenv(acctest.EnvProjectID)
	projectID := os.Getenv(acctest.EnvAltProjectID)
	if projectID == "" {
		projectID = defaultProjectID
	}
	config := testAccBrowserPoolDataSourceConfig(name, projectID)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if defaultProjectID == "" {
				t.Fatalf("%s must be set for the browser pool data source acceptance test", acctest.EnvProjectID)
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckBrowserPoolDataSourceDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureBrowserPoolDataSourceID(t),
					resource.TestCheckResourceAttrPair("data.kernel_browser_pool.by_id", "id", browserPoolAcceptanceResourceName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_browser_pool.by_name", "id", browserPoolAcceptanceResourceName, "id"),
					resource.TestCheckResourceAttrPair("data.kernel_browser_pool.by_id", "name", browserPoolAcceptanceResourceName, "name"),
					resource.TestCheckResourceAttr("data.kernel_browser_pool.by_id", "project_id", projectID),
					resource.TestCheckResourceAttr("data.kernel_browser_pool.by_name", "project_id", projectID),
					testAccCheckBrowserPoolDataSourceState("data.kernel_browser_pool.by_id", name),
					testAccCheckBrowserPoolDataSourceState("data.kernel_browser_pool.by_name", name),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccCheckBrowserPoolDataSourceDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != "kernel_browser_pool" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			_, err := client.GetBrowserPool(ctx, resourceState.Primary.Attributes["project_id"], resourceState.Primary.ID)
			if projectscope.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read Kernel browser pool %s after destroy: %w", resourceState.Primary.ID, err)
			}
			return fmt.Errorf("Kernel browser pool %s still exists after destroy", resourceState.Primary.ID)
		}
		return nil
	}
}

func testAccBrowserPoolDataSourceConfig(name, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_browser_pool" "data_source_test" {
  name                 = %[1]q
  size                 = 1
  project_id           = %[2]q
  start_url            = "chrome://newtab"
  headless             = true
  kiosk_mode           = false
  stealth              = false
  timeout_seconds      = 90
  fill_rate_per_minute = 0
  viewport = {
    width        = 1280
    height       = 800
    refresh_rate = 60
  }
  chrome_policy = jsonencode({
    HomepageLocation = "https://example.com"
    RestoreOnStartup = 4
  })
}

data "kernel_browser_pool" "by_id" {
  id         = kernel_browser_pool.data_source_test.id
  project_id = %[2]q
}

data "kernel_browser_pool" "by_name" {
  name       = kernel_browser_pool.data_source_test.name
  project_id = %[2]q
}
`, name, projectID)
}

func testAccCaptureBrowserPoolDataSourceID(t *testing.T) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[browserPoolAcceptanceResourceName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("missing ID for %s", browserPoolAcceptanceResourceName)
		}
		acctest.CleanupBrowserPool(t, resourceState.Primary.Attributes["project_id"], resourceState.Primary.ID)
		return nil
	}
}

func testAccCheckBrowserPoolDataSourceState(resourceName, name string) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(resourceName, "name", name),
		resource.TestCheckResourceAttr(resourceName, "size", "1"),
		resource.TestCheckResourceAttr(resourceName, "start_url", "chrome://newtab"),
		resource.TestCheckResourceAttr(resourceName, "headless", "true"),
		resource.TestCheckResourceAttr(resourceName, "kiosk_mode", "false"),
		resource.TestCheckResourceAttr(resourceName, "stealth", "false"),
		resource.TestCheckResourceAttr(resourceName, "timeout_seconds", "90"),
		resource.TestCheckResourceAttr(resourceName, "fill_rate_per_minute", "0"),
		resource.TestCheckResourceAttr(resourceName, "viewport.width", "1280"),
		resource.TestCheckResourceAttr(resourceName, "viewport.height", "800"),
		resource.TestCheckResourceAttr(resourceName, "viewport.refresh_rate", "60"),
		resource.TestCheckResourceAttr(resourceName, "chrome_policy", `{"HomepageLocation":"https://example.com","RestoreOnStartup":4}`),
		resource.TestCheckResourceAttr(resourceName, "extension_ids.#", "0"),
		resource.TestCheckNoResourceAttr(resourceName, "profile_id"),
		resource.TestCheckNoResourceAttr(resourceName, "proxy_id"),
	)
}
