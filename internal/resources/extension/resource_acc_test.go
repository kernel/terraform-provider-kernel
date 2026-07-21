package extension_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const extensionResourceName = "kernel_extension.test"

func TestAccExtensionLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "extension")
	firstPath, firstChecksum := testAccWriteExtensionArchive(t, "first")
	secondPath, secondChecksum := testAccWriteExtensionArchive(t, "second")
	firstConfig := testAccExtensionConfig(name, firstPath)
	secondConfig := testAccExtensionConfig(name, secondPath)
	var firstID, secondID string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
			if os.Getenv(acctest.EnvProjectID) == "" {
				t.Fatalf("%s must be set for extension acceptance tests", acctest.EnvProjectID)
			}
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckExtensionDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: firstConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureExtensionID(t, extensionResourceName, &firstID),
					resource.TestCheckResourceAttrSet(extensionResourceName, "id"),
					resource.TestCheckResourceAttr(extensionResourceName, "name", name),
					resource.TestCheckResourceAttr(extensionResourceName, "project_id", os.Getenv(acctest.EnvProjectID)),
					resource.TestCheckResourceAttr(extensionResourceName, "source_sha256", firstChecksum),
				),
			},
			{
				Config:   firstConfig,
				PlanOnly: true,
			},
			{
				ResourceName:       extensionResourceName,
				ImportState:        true,
				ImportStateVerify:  true,
				ImportStatePersist: true,
			},
			{
				Config:   firstConfig,
				PlanOnly: true,
			},
			{
				Config: secondConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureExtensionID(t, extensionResourceName, &secondID),
					testAccCheckExtensionReplaced(extensionResourceName, &firstID),
					testAccCheckExtensionGone(&firstID),
					resource.TestCheckResourceAttr(extensionResourceName, "name", name),
					resource.TestCheckResourceAttr(extensionResourceName, "source_sha256", secondChecksum),
				),
			},
			{
				Config:   secondConfig,
				PlanOnly: true,
			},
		},
	})
}

func testAccExtensionConfig(name, sourcePath string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_extension" "test" {
  name          = %q
  source_path   = %q
  source_sha256 = filesha256(%q)
}
`, name, sourcePath, sourcePath)
}

func testAccWriteExtensionArchive(t *testing.T, marker string) (string, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "extension.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create extension archive: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	testAccWriteZipFile(t, writer, "manifest.json", `{"manifest_version":3,"name":"Kernel Terraform acceptance","version":"1.0.0"}`)
	testAccWriteZipFile(t, writer, "marker.txt", marker)
	if err := writer.Close(); err != nil {
		t.Fatalf("close extension ZIP: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close extension archive: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extension archive: %v", err)
	}
	checksum := sha256.Sum256(contents)
	return path, hex.EncodeToString(checksum[:])
}

func testAccWriteZipFile(t *testing.T, writer *zip.Writer, name, contents string) {
	t.Helper()

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create %s in extension ZIP: %v", name, err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write %s in extension ZIP: %v", name, err)
	}
}

func testAccCaptureExtensionID(t *testing.T, resourceName string, extensionID *string) resource.TestCheckFunc {
	t.Helper()

	return func(state *terraform.State) error {
		id, projectID, err := extensionStateValues(state, resourceName)
		if err != nil {
			return err
		}
		*extensionID = id
		acctest.CleanupExtension(t, projectID, id)
		return nil
	}
}

func testAccCheckExtensionReplaced(resourceName string, previousID *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		id, _, err := extensionStateValues(state, resourceName)
		if err != nil {
			return err
		}
		if id == *previousID {
			return fmt.Errorf("Kernel extension ID remained %s after content replacement", id)
		}
		return nil
	}
}

func testAccCheckExtensionGone(extensionID *string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		projectID := os.Getenv(acctest.EnvProjectID)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			_, err := client.GetExtension(ctx, projectID, *extensionID)
			if acctest.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read replaced Kernel extension %s: %w", *extensionID, err)
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("replaced Kernel extension %s still exists after 30 seconds", *extensionID)
			case <-ticker.C:
			}
		}
	}
}

func testAccCheckExtensionDestroyed() resource.TestCheckFunc {
	return func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := acctest.ClientFromEnv()
		for _, resourceState := range state.RootModule().Resources {
			if resourceState.Type != "kernel_extension" || resourceState.Primary == nil || resourceState.Primary.ID == "" {
				continue
			}

			projectID := resourceState.Primary.Attributes["project_id"]
			_, err := client.GetExtension(ctx, projectID, resourceState.Primary.ID)
			if acctest.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read Kernel extension %s after destroy: %w", resourceState.Primary.ID, err)
			}
			return fmt.Errorf("Kernel extension %s still exists after destroy", resourceState.Primary.ID)
		}
		return nil
	}
}

func extensionStateValues(state *terraform.State, resourceName string) (id, projectID string, err error) {
	resourceState, ok := state.RootModule().Resources[resourceName]
	if !ok {
		return "", "", fmt.Errorf("missing resource %s in Terraform state", resourceName)
	}
	if resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", "", fmt.Errorf("missing ID for %s in Terraform state", resourceName)
	}
	return resourceState.Primary.ID, resourceState.Primary.Attributes["project_id"], nil
}
