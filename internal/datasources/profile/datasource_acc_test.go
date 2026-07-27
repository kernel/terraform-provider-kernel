package profile_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

func TestAccProfileDataSourceByIDAndName(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := os.Getenv(acctest.EnvProjectID)
	if projectID == "" {
		t.Fatalf("%s must be set for the profile data source acceptance test", acctest.EnvProjectID)
	}

	name := acctest.UniqueName(t, "profile-data")
	id := testAccCreateProfileFixture(t, projectID, name)
	config := testAccProfileDataSourceConfig(id, name, projectID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kernel_profile.by_id", "id", id),
					resource.TestCheckResourceAttr("data.kernel_profile.by_id", "name", name),
					resource.TestCheckResourceAttr("data.kernel_profile.by_id", "project_id", projectID),
					resource.TestCheckResourceAttrSet("data.kernel_profile.by_id", "created_at"),
					resource.TestCheckResourceAttr("data.kernel_profile.by_name", "id", id),
					resource.TestCheckResourceAttr("data.kernel_profile.by_name", "name", name),
					resource.TestCheckNoResourceAttr("data.kernel_profile.by_name", "project_id"),
					resource.TestCheckResourceAttrSet("data.kernel_profile.by_name", "created_at"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccCreateProfileFixture(t *testing.T, projectID, name string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	profile, err := acctest.ClientFromEnv().CreateProfile(ctx, projectID, kernel.ProfileNewParams{Name: kernel.String(name)})
	if err != nil {
		t.Fatalf("create Kernel profile fixture: %v", err)
	}
	if profile == nil || profile.ID == "" {
		t.Fatal("Kernel returned an empty profile fixture response")
	}

	// Cleanups run last-in-first-out, so deletion must register after verification.
	testAccRequireProfileDeleted(t, projectID, profile.ID)
	acctest.CleanupProfile(t, projectID, profile.ID)

	if profile.Name != name {
		t.Fatalf("created Kernel profile name = %q, want %q", profile.Name, name)
	}
	return profile.ID
}

func testAccRequireProfileDeleted(t *testing.T, projectID, id string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := acctest.ClientFromEnv().GetProfile(ctx, projectID, id)
		if projectscope.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("read Kernel profile %s after cleanup: %v", id, err)
			return
		}
		t.Errorf("Kernel profile %s still exists after cleanup", id)
	})
}

func testAccProfileDataSourceConfig(id, name, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_profile" "by_id" {
  id         = %q
  project_id = %q
}

data "kernel_profile" "by_name" {
  name = %q
}
`, id, projectID, name)
}
