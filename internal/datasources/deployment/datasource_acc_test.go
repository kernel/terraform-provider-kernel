package deployment_test

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

func TestAccDeploymentDataSourceExplicitAndProviderDefaultScope(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := requireFixtureEnv(t, acctest.EnvProjectID)
	appName := requireFixtureEnv(t, envAppName)
	version := requireFixtureEnv(t, envAppVersion)
	config := testAccDeploymentDataSourceConfig(projectID, appName, version)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.kernel_deployment.explicit", "id", "data.kernel_app.fixture", "deployment_id"),
					resource.TestCheckResourceAttrPair("data.kernel_deployment.explicit", "id", "data.kernel_deployment.provider_default", "id"),
					resource.TestCheckResourceAttrPair("data.kernel_deployment.explicit", "region", "data.kernel_deployment.provider_default", "region"),
					resource.TestCheckResourceAttrPair("data.kernel_deployment.explicit", "status", "data.kernel_deployment.provider_default", "status"),
					resource.TestCheckResourceAttr("data.kernel_deployment.explicit", "project_id", projectID),
					resource.TestCheckResourceAttrSet("data.kernel_deployment.explicit", "region"),
					resource.TestCheckResourceAttrSet("data.kernel_deployment.explicit", "status"),
					resource.TestCheckResourceAttrSet("data.kernel_deployment.explicit", "created_at"),
					resource.TestCheckNoResourceAttr("data.kernel_deployment.explicit", "env_vars"),
					resource.TestCheckNoResourceAttr("data.kernel_deployment.provider_default", "project_id"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func requireFixtureEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s must identify the release-owned app fixture", name)
	}
	return value
}

func testAccDeploymentDataSourceConfig(projectID, appName, version string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_app" "fixture" {
  app_name   = %[1]q
  version    = %[2]q
  project_id = %[3]q
}

data "kernel_deployment" "explicit" {
  id         = data.kernel_app.fixture.deployment_id
  project_id = %[3]q
}

data "kernel_deployment" "provider_default" {
  id = data.kernel_app.fixture.deployment_id
}
`, appName, version, projectID)
}
