package auditlogexportdestination_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const destinationResourceName = "kernel_audit_log_export_destination.test"

func TestAccAuditLogExportDestinationLifecycle(t *testing.T) {
	bucket := os.Getenv(acctest.EnvAuditExportBucket)
	region := os.Getenv(acctest.EnvAuditExportRegion)
	roleARN := os.Getenv(acctest.EnvAuditExportRoleARN)
	prefix := acctest.UniqueName(t, "audit") + "/"
	updatedPrefix := acctest.UniqueName(t, "audit-updated")
	finalConfig := testAccDestinationConfig(bucket, region, updatedPrefix, roleARN, "paused")
	var destinationID string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if bucket == "" || region == "" || roleARN == "" {
				t.Skipf("set %s, %s and %s to run the audit log export destination acceptance test",
					acctest.EnvAuditExportBucket, acctest.EnvAuditExportRegion, acctest.EnvAuditExportRoleARN)
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckDestinationDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccDestinationConfig(bucket, region, prefix, roleARN, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckResourceAttr(destinationResourceName, "type", "s3"),
					resource.TestCheckResourceAttr(destinationResourceName, "format", "jsonl.gz"),
					resource.TestCheckResourceAttr(destinationResourceName, "bucket", bucket),
					resource.TestCheckResourceAttr(destinationResourceName, "prefix", prefix),
					resource.TestCheckResourceAttr(destinationResourceName, "status", "paused"),
					testAccCheckRemoteStatus("paused"),
					resource.TestCheckResourceAttrSet(destinationResourceName, "external_id"),
					resource.TestCheckResourceAttrSet(destinationResourceName, "kernel_role_arn"),
				),
			},
			{
				Config: testAccDestinationConfig(bucket, region, prefix, roleARN, "active"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckResourceAttr(destinationResourceName, "status", "active"),
					testAccCheckRemoteStatus("active"),
				),
			},
			{
				Config: finalConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureDestinationID(t, &destinationID),
					resource.TestCheckResourceAttr(destinationResourceName, "prefix", updatedPrefix),
					resource.TestCheckResourceAttr(destinationResourceName, "status", "paused"),
					testAccCheckRemoteStatus("paused"),
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

func testAccDestinationConfig(bucket, region, prefix, roleARN, status string) string {
	config := acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_audit_log_export_destination" "test" {
  bucket   = %q
  region   = %q
  prefix   = %q
  role_arn = %q
`, bucket, region, prefix, roleARN)
	if status != "" {
		config += fmt.Sprintf("  status   = %q\n", status)
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
			return fmt.Errorf("Kernel audit log export destination ID changed from %s to %s", *destinationID, id)
		}
		if *destinationID == "" {
			*destinationID = id
			acctest.CleanupAuditLogExportDestination(t, id)
		}
		return nil
	}
}

func testAccCheckRemoteStatus(want string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		id, err := destinationStateID(state)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		remote, err := acctest.ClientFromEnv().GetAuditLogExportDestination(ctx, id)
		if err != nil {
			return fmt.Errorf("read Kernel audit log export destination %s: %w", id, err)
		}
		if got := string(remote.Status); got != want {
			return fmt.Errorf("Kernel audit log export destination status = %s, want %s", got, want)
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
			if resourceState.Type != "kernel_audit_log_export_destination" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			_, err := client.GetAuditLogExportDestination(ctx, resourceState.Primary.ID)
			if acctest.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read Kernel audit log export destination %s after destroy: %w", resourceState.Primary.ID, err)
			}
			return fmt.Errorf("Kernel audit log export destination %s still exists after destroy", resourceState.Primary.ID)
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
