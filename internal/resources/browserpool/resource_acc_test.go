package browserpool_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const browserPoolResourceName = "kernel_browser_pool.test"

func TestAccBrowserPoolLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "browser-pool")
	var poolID string

	updatedConfig := testAccBrowserPoolConfig(name, "https://example.com/two")
	capturePoolID := acctest.CaptureResourceID(t, browserPoolResourceName, &poolID, func(id string, attributes map[string]string) {
		acctest.CleanupBrowserPool(t, attributes["project_id"], id)
	})
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckBrowserPoolDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccBrowserPoolConfig(name, "https://example.com/one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					capturePoolID,
					testAccCheckBrowserPoolProject(browserPoolResourceName, os.Getenv(acctest.EnvProjectID)),
					resource.TestCheckResourceAttrSet(browserPoolResourceName, "id"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "name", name),
					resource.TestCheckResourceAttr(browserPoolResourceName, "size", "1"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "start_url", "https://example.com/one"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "headless", "true"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "kiosk_mode", "false"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "stealth", "false"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "timeout_seconds", "90"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "fill_rate_per_minute", "0"),
				),
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					capturePoolID,
					testAccCheckBrowserPoolID(browserPoolResourceName, &poolID),
					resource.TestCheckResourceAttr(browserPoolResourceName, "name", name),
					resource.TestCheckResourceAttr(browserPoolResourceName, "size", "1"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "start_url", "https://example.com/two"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      browserPoolResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccBrowserPoolProjectScoped(t *testing.T) {
	// Prefer a second project so the explicit override is distinguishable
	// from inheriting the provider default.
	projectID := os.Getenv(acctest.EnvAltProjectID)
	if projectID == "" {
		projectID = os.Getenv(acctest.EnvProjectID)
	}
	if projectID == "" {
		t.Skipf("%s or %s must be set for the project-scoped browser pool test", acctest.EnvAltProjectID, acctest.EnvProjectID)
	}

	name := acctest.UniqueName(t, "browser-pool-scoped")
	var poolID string

	config := testAccBrowserPoolProjectConfig(name, projectID)
	capturePoolID := acctest.CaptureResourceID(t, browserPoolResourceName, &poolID, func(id string, attributes map[string]string) {
		acctest.CleanupBrowserPool(t, attributes["project_id"], id)
	})
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckBrowserPoolDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					capturePoolID,
					resource.TestCheckResourceAttr(browserPoolResourceName, "project_id", projectID),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      browserPoolResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					id, stateProjectID, err := browserPoolStateValues(state, browserPoolResourceName)
					if err != nil {
						return "", err
					}
					return stateProjectID + "/" + id, nil
				},
			},
		},
	})
}

func testAccBrowserPoolProjectConfig(name, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_browser_pool" "test" {
  name       = %[1]q
  size       = 1
  project_id = %[2]q
}
`, name, projectID)
}

func testAccBrowserPoolConfig(name, startURL string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_browser_pool" "test" {
  name                 = %[1]q
  size                 = 1
  start_url            = %[2]q
  headless             = true
  kiosk_mode           = false
  stealth              = false
  timeout_seconds      = 90
  fill_rate_per_minute = 0
}
`, name, startURL)
}

func testAccCheckBrowserPoolProject(resourceName, want string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		_, projectID, err := browserPoolStateValues(state, resourceName)
		if err != nil {
			return err
		}
		if projectID != want {
			return fmt.Errorf("browser pool project_id = %q, want %q", projectID, want)
		}
		return nil
	}
}

func testAccCheckBrowserPoolID(resourceName string, poolID *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		id, _, err := browserPoolStateValues(state, resourceName)
		if err != nil {
			return err
		}
		if id != *poolID {
			return fmt.Errorf("Kernel browser pool ID = %s, want %s", id, *poolID)
		}
		return nil
	}
}

func testAccCheckBrowserPoolDestroyed() resource.TestCheckFunc {
	return acctest.CheckResourceDestroyed(
		"kernel_browser_pool",
		func(ctx context.Context, id string, attributes map[string]string) error {
			client := acctest.ClientFromEnv()
			_, err := client.GetBrowserPool(ctx, attributes["project_id"], id)
			return err
		},
	)
}

func browserPoolStateValues(state *terraform.State, resourceName string) (id, projectID string, err error) {
	id, attributes, err := acctest.ResourceStateValues(state, resourceName)
	if err != nil {
		return "", "", err
	}
	return id, attributes["project_id"], nil
}
