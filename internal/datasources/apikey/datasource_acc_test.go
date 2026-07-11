package apikey_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

func TestAccAPIKeyDataSourceIDAndExactName(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := os.Getenv(acctest.EnvProjectID)
	if projectID == "" {
		t.Fatalf("%s must be set for the project-scoped API key fixture", acctest.EnvProjectID)
	}
	name := acctest.UniqueName(t, "api-key-data")
	client := acctest.ClientFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	created, err := client.CreateAPIKey(ctx, kernel.APIKeyNewParams{
		Name:      name,
		ProjectID: kernel.String(projectID),
	})
	cancel()
	if err != nil {
		t.Fatalf("create Kernel API key fixture: %v", err)
	}
	if created == nil || created.ID == "" {
		t.Fatal("Kernel returned an empty API key fixture response")
	}
	id := created.ID

	// Cleanups run last-in-first-out: delete first, then prove ordinary Get no
	// longer returns the soft-deleted fixture.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_, err := client.GetAPIKey(ctx, id)
		if err == nil || !acctest.IsNotFound(err) {
			t.Errorf("verify Kernel API key fixture %s deletion: %v", id, err)
		}
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := client.DeleteAPIKey(ctx, id); err != nil && !acctest.IsNotFound(err) {
			t.Errorf("delete Kernel API key fixture %s: %v", id, err)
		}
	})

	config := testAccAPIKeyDataSourceConfig(id, name)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.kernel_api_key.by_id", "id", "data.kernel_api_key.by_name", "id"),
					resource.TestCheckResourceAttrPair("data.kernel_api_key.by_id", "masked_key", "data.kernel_api_key.by_name", "masked_key"),
					resource.TestCheckResourceAttr("data.kernel_api_key.by_id", "id", id),
					resource.TestCheckResourceAttr("data.kernel_api_key.by_id", "name", name),
					resource.TestCheckResourceAttr("data.kernel_api_key.by_id", "project_id", projectID),
					resource.TestCheckResourceAttrSet("data.kernel_api_key.by_id", "masked_key"),
					resource.TestCheckResourceAttrSet("data.kernel_api_key.by_id", "created_at"),
					resource.TestCheckNoResourceAttr("data.kernel_api_key.by_id", "key"),
					resource.TestCheckNoResourceAttr("data.kernel_api_key.by_id", "deleted_at"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccAPIKeyDataSourceConfig(id, name string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_api_key" "by_id" {
  id = %[1]q
}

data "kernel_api_key" "by_name" {
  name = %[2]q
}
`, id, name)
}
