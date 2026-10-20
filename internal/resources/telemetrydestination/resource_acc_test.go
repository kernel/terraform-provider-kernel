package telemetrydestination_test

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const destinationResourceName = "kernel_telemetry_destination.test"

func TestAccTelemetryDestinationLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "otlp")
	updatedName := acctest.UniqueName(t, "otlp-updated")
	finalConfig := testAccDestinationConfig(updatedName, "https://otlp.datadoghq.com", "", "")
	var destinationID string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckDestinationDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccDestinationConfig(name, "https://api.honeycomb.io", "acceptance", `{
    "x-honeycomb-team" = "first-key"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckResourceAttr(destinationResourceName, "name", name),
					resource.TestCheckResourceAttr(destinationResourceName, "endpoint", "https://api.honeycomb.io"),
					resource.TestCheckResourceAttr(destinationResourceName, "description", "acceptance"),
					resource.TestCheckResourceAttr(destinationResourceName, "headers.x-honeycomb-team", "first-key"),
					testAccCheckRemoteHeaderNames("X-Honeycomb-Team"),
				),
			},
			{
				Config: testAccDestinationConfig(updatedName, "https://otlp.datadoghq.com", "", `{
    "x-honeycomb-team" = "second-key"
    "X-Dataset"        = "browsers"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckResourceAttr(destinationResourceName, "name", updatedName),
					resource.TestCheckResourceAttr(destinationResourceName, "endpoint", "https://otlp.datadoghq.com"),
					resource.TestCheckNoResourceAttr(destinationResourceName, "description"),
					resource.TestCheckResourceAttr(destinationResourceName, "headers.%", "2"),
					resource.TestCheckResourceAttr(destinationResourceName, "headers.x-honeycomb-team", "second-key"),
					testAccCheckRemoteHeaderNames("X-Dataset", "X-Honeycomb-Team"),
				),
			},
			{
				// Kernel redacts header values, so an import records names only.
				ResourceName:            destinationResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"headers"},
			},
			{
				Config: finalConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckNoResourceAttr(destinationResourceName, "headers.%"),
					testAccCheckRemoteHeaderNames(),
				),
			},
			{
				Config:   finalConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      destinationResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccDestinationConfig(name, endpoint, description, headers string) string {
	config := acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_telemetry_destination" "test" {
  name     = %q
  endpoint = %q
`, name, endpoint)
	if description != "" {
		config += fmt.Sprintf("  description = %q\n", description)
	}
	if headers != "" {
		config += "  headers = " + headers + "\n"
	}
	return config + "}\n"
}

func testAccCaptureDestinationID(t *testing.T, destinationID *string) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		id, err := destinationStateID(state)
		if err != nil {
			return err
		}
		if *destinationID != "" && *destinationID != id {
			return fmt.Errorf("Kernel telemetry destination ID changed from %s to %s", *destinationID, id)
		}
		if *destinationID == "" {
			*destinationID = id
			acctest.CleanupTelemetryDestination(t, id)
		}
		return nil
	}
}

func testAccCheckRemoteHeaderNames(want ...string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		id, err := destinationStateID(state)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		remote, err := acctest.ClientFromEnv().GetTelemetryDestination(ctx, id)
		if err != nil {
			return fmt.Errorf("read Kernel telemetry destination %s: %w", id, err)
		}
		got := slices.Sorted(maps.Keys(remote.Headers))
		if !slices.Equal(got, want) {
			return fmt.Errorf("Kernel telemetry destination headers = %v, want %v", got, want)
		}
		return nil
	}
}

func testAccCheckDestinationDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != "kernel_telemetry_destination" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			_, err := client.GetTelemetryDestination(ctx, resourceState.Primary.ID)
			if acctest.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read Kernel telemetry destination %s after destroy: %w", resourceState.Primary.ID, err)
			}
			return fmt.Errorf("Kernel telemetry destination %s still exists after destroy", resourceState.Primary.ID)
		}
		return nil
	}
}

func destinationStateID(state *terraform.State) (string, error) {
	resourceState, ok := state.RootModule().Resources[destinationResourceName]
	if !ok {
		return "", fmt.Errorf("missing resource %s in Terraform state", destinationResourceName)
	}
	if resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", fmt.Errorf("missing ID for %s in Terraform state", destinationResourceName)
	}
	return resourceState.Primary.ID, nil
}
