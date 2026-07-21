package app_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const (
	envAppName    = "KERNEL_ACC_APP_NAME"
	envAppVersion = "KERNEL_ACC_APP_VERSION"
)

func TestAccAppDataSourceExplicitAndProviderDefaultScope(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := requireAppFixtureEnv(t, acctest.EnvProjectID)
	appName := requireAppFixtureEnv(t, envAppName)
	version := requireAppFixtureEnv(t, envAppVersion)
	config := testAccAppDataSourceConfig(projectID, appName, version)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.kernel_app.explicit", "id", "data.kernel_app.provider_default", "id"),
					resource.TestCheckResourceAttrPair("data.kernel_app.explicit", "deployment_id", "data.kernel_app.provider_default", "deployment_id"),
					resource.TestCheckResourceAttrPair("data.kernel_app.explicit", "region", "data.kernel_app.provider_default", "region"),
					resource.TestCheckResourceAttr("data.kernel_app.explicit", "app_name", appName),
					resource.TestCheckResourceAttr("data.kernel_app.explicit", "version", version),
					resource.TestCheckResourceAttr("data.kernel_app.explicit", "project_id", projectID),
					resource.TestCheckResourceAttrSet("data.kernel_app.explicit", "id"),
					resource.TestCheckResourceAttrSet("data.kernel_app.explicit", "deployment_id"),
					resource.TestCheckResourceAttrSet("data.kernel_app.explicit", "region"),
					resource.TestCheckResourceAttr("data.kernel_app.provider_default", "app_name", appName),
					resource.TestCheckResourceAttr("data.kernel_app.provider_default", "version", version),
					resource.TestCheckNoResourceAttr("data.kernel_app.provider_default", "project_id"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func requireAppFixtureEnv(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must identify the release-owned app fixture", name)
	}
	return value
}

func testAccAppDataSourceConfig(projectID, appName, version string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_app" "explicit" {
  app_name   = %[1]q
  version    = %[2]q
  project_id = %[3]q
}

data "kernel_app" "provider_default" {
  app_name = %[1]q
  version  = %[2]q
}
`, appName, version, projectID)
}
