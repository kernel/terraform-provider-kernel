package project_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

const projectResourceName = "kernel_project.test"

func TestAccProjectLifecycle(t *testing.T) {
	name := acctest.UniqueName(t, "project")
	updatedName := acctest.UniqueName(t, "project-updated")
	updatedConfig := testAccProjectConfig(updatedName)
	var projectID string
	captureProjectID := acctest.CaptureResourceID(t, projectResourceName, &projectID, func(id string, _ map[string]string) {
		acctest.CleanupProject(t, id)
	})

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			acctest.PreCheck(t)
		},
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		CheckDestroy:             testAccCheckProjectDestroyed(),
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureProjectID,
					resource.TestCheckResourceAttrSet(projectResourceName, "id"),
					resource.TestCheckResourceAttr(projectResourceName, "name", name),
				),
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureProjectID,
					resource.TestCheckResourceAttr(projectResourceName, "name", updatedName),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:       projectResourceName,
				ImportState:        true,
				ImportStateVerify:  true,
				ImportStatePersist: true,
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
		},
	})
}

func testAccProjectConfig(name string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
resource "kernel_project" "test" {
  name = %q
}
`, name)
}

func testAccCheckProjectDestroyed() resource.TestCheckFunc {
	return acctest.CheckResourceDestroyed(
		"kernel_project",
		func(ctx context.Context, id string, _ map[string]string) error {
			client := acctest.ClientFromEnv()
			_, err := client.GetProject(ctx, id)
			return err
		},
	)
}
