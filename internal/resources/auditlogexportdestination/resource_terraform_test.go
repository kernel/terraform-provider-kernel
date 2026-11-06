package auditlogexportdestination_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
)

func TestDestinationTerraformLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Terraform CLI")
	}

	var mu sync.Mutex
	var destination map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&destination); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			destination["id"] = "dest_123"
			destination["status"] = "paused"
			destination["external_id"] = "ext_123"
			destination["kernel_role_arn"] = "arn:aws:iam::999999999999:role/exporter"
		case http.MethodPatch:
			var patch map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for k, v := range patch {
				destination[k] = v
			}
		case http.MethodDelete:
			destination = nil
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if destination == nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"code":"not_found"}`)
			return
		}
		// Match the API's canonical prefix and paused-on-create behavior.
		destination["prefix"] = strings.TrimRight(destination["prefix"].(string), "/")
		if err := json.NewEncoder(w).Encode(destination); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	config := func(prefix, status string) string {
		return fmt.Sprintf(`provider "kernel" {
  api_key = "test-key"
  base_url = %q
}
resource "kernel_audit_log_export_destination" "test" {
  bucket = "audit-bucket"
  region = "us-east-1"
  prefix = %q
  role_arn = "arn:aws:iam::123456789012:role/audit"
  %s
}
`, server.URL, prefix, status)
	}
	initial := config("kernel/audit-logs/", "")
	active := config("updated///", `status = "active"`)
	canonical := config("updated", `status = "paused"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: initial,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(destinationResourceName, "prefix", "kernel/audit-logs/"),
					resource.TestCheckResourceAttr(destinationResourceName, "status", "paused"),
				),
			},
			{Config: initial, PlanOnly: true},
			{
				Config: active,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(destinationResourceName, "prefix", "updated///"),
					resource.TestCheckResourceAttr(destinationResourceName, "status", "active"),
				),
			},
			{Config: active, PlanOnly: true},
			{Config: canonical},
			{ResourceName: destinationResourceName, ImportState: true, ImportStateVerify: true},
			{Config: canonical, PlanOnly: true},
		},
	})
}
