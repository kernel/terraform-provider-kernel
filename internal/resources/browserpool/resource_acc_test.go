package browserpool_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/kernel-go-sdk/option"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const browserPoolResourceName = "kernel_browser_pool.test"

func TestAccBrowserPoolLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "browser-pool")
	var poolID string

	updatedConfig := testAccBrowserPoolConfig(name, "https://example.com/two", true)
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckBrowserPoolDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccBrowserPoolConfig(name, "https://example.com/one", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureBrowserPoolID(t, browserPoolResourceName, &poolID),
					testAccWaitForAvailableBrowser(browserPoolResourceName),
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
					resource.TestCheckResourceAttr(browserPoolResourceName, "rebuild_idle_browsers_on_update", "true"),
				),
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureBrowserPoolID(t, browserPoolResourceName, &poolID),
					testAccCheckBrowserPoolID(browserPoolResourceName, &poolID),
					resource.TestCheckResourceAttr(browserPoolResourceName, "name", name),
					resource.TestCheckResourceAttr(browserPoolResourceName, "size", "1"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "start_url", "https://example.com/two"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "stealth", "true"),
					resource.TestCheckResourceAttr(browserPoolResourceName, "rebuild_idle_browsers_on_update", "true"),
					testAccCheckAcquiredBrowserStealth(t, browserPoolResourceName, true),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:            browserPoolResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"rebuild_idle_browsers_on_update"},
			},
		},
	})
}

func testAccWaitForAvailableBrowser(resourceName string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		poolID, projectID, err := browserPoolStateValues(state, resourceName)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		client := acctest.ClientFromEnv()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			pool, err := client.GetBrowserPool(ctx, projectID, poolID)
			if err != nil {
				return fmt.Errorf("wait for idle browser in Kernel pool %s: %w", poolID, err)
			}
			if pool != nil && pool.AvailableCount > 0 {
				return nil
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("wait for idle browser in Kernel pool %s: %w", poolID, ctx.Err())
			case <-ticker.C:
			}
		}
	}
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
					testAccCaptureBrowserPoolID(t, browserPoolResourceName, &poolID),
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

func testAccBrowserPoolConfig(name, startURL string, stealth bool) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_browser_pool" "test" {
  name                 = %[1]q
  size                 = 1
  start_url            = %[2]q
  headless             = true
  kiosk_mode           = false
  stealth              = %[3]t
  timeout_seconds      = 90
  fill_rate_per_minute = 0
  rebuild_idle_browsers_on_update = true
}
`, name, startURL, stealth)
}

func testAccCheckAcquiredBrowserStealth(t *testing.T, resourceName string, want bool) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		poolID, projectID, err := browserPoolStateValues(state, resourceName)
		if err != nil {
			return err
		}

		opts := []option.RequestOption{
			option.WithEnvironmentProduction(),
			option.WithAPIKey(os.Getenv(acctest.EnvAPIKey)),
		}
		if baseURL := os.Getenv(acctest.EnvBaseURL); baseURL != "" {
			opts = append(opts, option.WithBaseURL(baseURL))
		}
		requestOpts := []option.RequestOption{option.WithProjectID(projectID)}
		client := kernel.NewClient(opts...)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		browser, err := client.BrowserPools.Acquire(ctx, poolID, kernel.BrowserPoolAcquireParams{
			AcquireTimeoutSeconds: kernel.Int(90),
		}, requestOpts...)
		if err != nil {
			return fmt.Errorf("acquire browser from updated Kernel pool %s: %w", poolID, err)
		}
		if browser == nil {
			return fmt.Errorf("acquire browser from updated Kernel pool %s returned no browser", poolID)
		}
		defer func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer releaseCancel()
			if err := client.BrowserPools.Release(releaseCtx, poolID, kernel.BrowserPoolReleaseParams{
				SessionID: browser.SessionID,
				Reuse:     kernel.Bool(false),
			}, requestOpts...); err != nil {
				t.Errorf("release acceptance browser %s: %v", browser.SessionID, err)
			}
		}()

		if browser.Stealth != want {
			return fmt.Errorf("acquired browser stealth = %t, want %t after pool update", browser.Stealth, want)
		}
		return nil
	}
}

func testAccCaptureBrowserPoolID(t *testing.T, resourceName string, poolID *string) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		id, projectID, err := browserPoolStateValues(state, resourceName)
		if err != nil {
			return err
		}
		if *poolID != "" && *poolID != id {
			return fmt.Errorf("Kernel browser pool ID changed from %s to %s", *poolID, id)
		}
		if *poolID == "" {
			*poolID = id
			acctest.CleanupBrowserPool(t, projectID, id)
		}
		return nil
	}
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
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != "kernel_browser_pool" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			_, err := client.GetBrowserPool(ctx, resourceState.Primary.Attributes["project_id"], resourceState.Primary.ID)
			if acctest.IsNotFound(err) {
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

func browserPoolStateValues(state *terraform.State, resourceName string) (id, projectID string, err error) {
	resourceState, ok := state.RootModule().Resources[resourceName]
	if !ok {
		return "", "", fmt.Errorf("missing resource %s in Terraform state", resourceName)
	}
	if resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", "", fmt.Errorf("missing ID for %s in Terraform state", resourceName)
	}
	return resourceState.Primary.ID, resourceState.Primary.Attributes["project_id"], nil
}
