package proxy_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

func TestAccProxyDataSourceByIDAndName(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := os.Getenv(acctest.EnvProjectID)
	if projectID == "" {
		t.Fatalf("%s must be set for the proxy data source acceptance test", acctest.EnvProjectID)
	}

	name := acctest.UniqueName(t, "proxy-data")
	id := testAccCreateProxyFixture(t, projectID, name)
	config := testAccProxyDataSourceConfig(id, name, projectID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kernel_proxy.by_id", "id", id),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_id", "name", name),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_id", "project_id", projectID),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_id", "type", "datacenter"),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_id", "protocol", "https"),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_name", "id", id),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_name", "name", name),
					resource.TestCheckNoResourceAttr("data.kernel_proxy.by_name", "project_id"),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_name", "type", "datacenter"),
					resource.TestCheckResourceAttr("data.kernel_proxy.by_name", "protocol", "https"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccCreateProxyFixture(t *testing.T, projectID, name string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), kernelclient.DefaultRequestTimeout)
	defer cancel()

	proxy, err := acctest.ClientFromEnv().CreateProxy(ctx, projectID, kernel.ProxyNewParams{
		Name: kernel.String(name),
		Type: kernel.ProxyNewParamsTypeDatacenter,
	})
	if err != nil {
		t.Fatalf("create Kernel proxy fixture: %v", err)
	}
	if proxy == nil || proxy.ID == "" {
		t.Fatal("Kernel returned an empty proxy fixture response")
	}

	// Cleanups run last-in-first-out, so deletion must register after verification.
	testAccRequireProxyDeleted(t, projectID, proxy.ID)
	testAccDeleteProxy(t, projectID, proxy.ID)

	if proxy.Name != name {
		t.Fatalf("created Kernel proxy name = %q, want %q", proxy.Name, name)
	}
	if proxy.Type != kernel.ProxyNewResponseTypeDatacenter {
		t.Fatalf("created Kernel proxy type = %q, want datacenter", proxy.Type)
	}
	return proxy.ID
}

func testAccDeleteProxy(t *testing.T, projectID, id string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := acctest.ClientFromEnv().DeleteProxy(ctx, projectID, id); err != nil && !acctest.IsNotFound(err) {
			t.Errorf("cleanup Kernel proxy %s: %v", id, err)
		}
	})
}

func testAccRequireProxyDeleted(t *testing.T, projectID, id string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := acctest.ClientFromEnv().GetProxy(ctx, projectID, id)
		if projectscope.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("read Kernel proxy %s after cleanup: %v", id, err)
			return
		}
		t.Errorf("Kernel proxy %s still exists after cleanup", id)
	})
}

func testAccProxyDataSourceConfig(id, name, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_proxy" "by_id" {
  id         = %q
  project_id = %q
}

data "kernel_proxy" "by_name" {
  name = %q
}
`, id, projectID, name)
}
