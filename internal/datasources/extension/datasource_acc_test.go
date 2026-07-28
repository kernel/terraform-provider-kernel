package extension_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/acctest"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/projectscope"
)

type extensionRecoveryClient interface {
	GetExtension(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

type fakeExtensionRecoveryClient struct {
	get func(context.Context, string, string) (*kernel.ExtensionGetResponse, error)
}

func (f fakeExtensionRecoveryClient) GetExtension(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
	return f.get(ctx, projectID, idOrName)
}

func TestExtensionArchiveFixture(t *testing.T) {
	t.Parallel()

	archive := testAccExtensionArchive(t)
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open extension ZIP: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("extension ZIP entries = %d, want 2", len(reader.File))
	}
	if reader.File[0].Name != "manifest.json" || reader.File[1].Name != "marker.txt" {
		t.Fatalf("extension ZIP entries = %q, %q", reader.File[0].Name, reader.File[1].Name)
	}
}

func TestRecoverExtensionFixture(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		extension *kernel.ExtensionGetResponse
		getErr    error
		wantID    string
		wantErr   bool
	}{
		"found": {
			extension: &kernel.ExtensionGetResponse{ID: "extension_123"},
			wantID:    "extension_123",
		},
		"not found": {
			getErr: testAccCodedNotFoundError(t),
		},
		"server error": {
			getErr:  errors.New("connection reset"),
			wantErr: true,
		},
		"empty response": {
			extension: &kernel.ExtensionGetResponse{},
			wantErr:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			extension, err := testAccRecoverExtensionFixture(fakeExtensionRecoveryClient{
				get: func(ctx context.Context, projectID, idOrName string) (*kernel.ExtensionGetResponse, error) {
					if projectID != "project_123" {
						t.Fatalf("project ID = %q, want project_123", projectID)
					}
					if idOrName != "fixture" {
						t.Fatalf("selector = %q, want fixture", idOrName)
					}
					return test.extension, test.getErr
				},
			}, "project_123", "fixture")
			if (err != nil) != test.wantErr {
				t.Fatalf("recovery error = %v, want error %t", err, test.wantErr)
			}
			if test.wantID == "" {
				if extension != nil {
					t.Fatalf("recovered extension = %#v, want nil", extension)
				}
				return
			}
			if extension == nil || extension.ID != test.wantID {
				t.Fatalf("recovered extension ID = %v, want %q", extension, test.wantID)
			}
		})
	}
}

func testAccCodedNotFoundError(t *testing.T) error {
	t.Helper()

	var apiError kernel.Error
	if err := json.Unmarshal([]byte(`{"error":{"code":"not_found","message":"not found"}}`), &apiError); err != nil {
		t.Fatalf("build coded not-found error: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.example/extensions/fixture", nil)
	if err != nil {
		t.Fatalf("build coded not-found request: %v", err)
	}
	apiError.StatusCode = http.StatusNotFound
	apiError.Request = request
	apiError.Response = &http.Response{StatusCode: http.StatusNotFound}
	return &apiError
}

func TestAccExtensionDataSourceByIDAndName(t *testing.T) {
	if os.Getenv("TF_ACC") != "1" {
		t.Skip("set TF_ACC=1 to run Terraform acceptance tests")
	}
	acctest.PreCheck(t)

	projectID := os.Getenv(acctest.EnvProjectID)
	if projectID == "" {
		t.Fatalf("%s must be set for the extension data source acceptance test", acctest.EnvProjectID)
	}

	name := acctest.UniqueName(t, "extension-data")
	archive := testAccExtensionArchive(t)
	id := testAccUploadExtensionFixture(t, projectID, name, archive)
	config := testAccExtensionDataSourceConfig(id, name, projectID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kernel_extension.by_id", "id", id),
					resource.TestCheckResourceAttr("data.kernel_extension.by_id", "name", name),
					resource.TestCheckResourceAttr("data.kernel_extension.by_id", "project_id", projectID),
					resource.TestCheckResourceAttrSet("data.kernel_extension.by_id", "created_at"),
					resource.TestCheckResourceAttr("data.kernel_extension.by_id", "size_bytes", strconv.Itoa(len(archive))),
					resource.TestCheckNoResourceAttr("data.kernel_extension.by_id", "last_used_at"),
					resource.TestCheckResourceAttr("data.kernel_extension.by_name", "id", id),
					resource.TestCheckResourceAttr("data.kernel_extension.by_name", "name", name),
					resource.TestCheckNoResourceAttr("data.kernel_extension.by_name", "project_id"),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_name", "created_at", "data.kernel_extension.by_id", "created_at"),
					resource.TestCheckResourceAttrPair("data.kernel_extension.by_name", "size_bytes", "data.kernel_extension.by_id", "size_bytes"),
					resource.TestCheckNoResourceAttr("data.kernel_extension.by_name", "last_used_at"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func testAccUploadExtensionFixture(t *testing.T, projectID, name string, archive []byte) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), kernelclient.DefaultRequestTimeout)
	defer cancel()

	extension, err := acctest.ClientFromEnv().UploadExtension(ctx, projectID, kernel.ExtensionUploadParams{
		File: bytes.NewReader(archive),
		Name: kernel.String(name),
	})
	if err != nil {
		recoveryErr := testAccRecoverAndTrackExtensionFixture(t, projectID, name)
		if recoveryErr != nil {
			t.Fatalf("upload Kernel extension fixture: %v; recover fixture by name: %v", err, recoveryErr)
		}
		t.Fatalf("upload Kernel extension fixture: %v", err)
	}
	if extension == nil || extension.ID == "" {
		if recoveryErr := testAccRecoverAndTrackExtensionFixture(t, projectID, name); recoveryErr != nil {
			t.Fatalf("Kernel returned an empty extension fixture response; recover fixture by name: %v", recoveryErr)
		}
		t.Fatal("Kernel returned an empty extension fixture response")
	}

	testAccTrackExtensionFixture(t, projectID, extension.ID)

	if extension.Name != name {
		t.Fatalf("uploaded Kernel extension name = %q, want %q", extension.Name, name)
	}
	if extension.SizeBytes != int64(len(archive)) {
		t.Fatalf("uploaded Kernel extension size = %d, want %d", extension.SizeBytes, len(archive))
	}
	return extension.ID
}

func testAccRecoverAndTrackExtensionFixture(t *testing.T, projectID, name string) error {
	t.Helper()

	recovered, err := testAccRecoverExtensionFixture(acctest.ClientFromEnv(), projectID, name)
	if recovered != nil {
		testAccTrackExtensionFixture(t, projectID, recovered.ID)
	}
	return err
}

func testAccRecoverExtensionFixture(client extensionRecoveryClient, projectID, name string) (*kernel.ExtensionGetResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kernelclient.DefaultRequestTimeout)
	defer cancel()

	extension, err := client.GetExtension(ctx, projectID, name)
	if projectscope.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if extension == nil || extension.ID == "" {
		return nil, fmt.Errorf("Kernel returned an empty extension response for %q", name)
	}
	return extension, nil
}

func testAccTrackExtensionFixture(t *testing.T, projectID, id string) {
	t.Helper()

	testAccRequireExtensionDeleted(t, projectID, id)
	acctest.CleanupExtension(t, projectID, id)
}

func testAccRequireExtensionDeleted(t *testing.T, projectID, id string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := acctest.ClientFromEnv().GetExtension(ctx, projectID, id)
		if projectscope.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("read Kernel extension %s after cleanup: %v", id, err)
			return
		}
		t.Errorf("Kernel extension %s still exists after cleanup", id)
	})
}

func testAccExtensionArchive(t *testing.T) []byte {
	t.Helper()

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	testAccWriteExtensionFile(t, writer, "manifest.json", `{"manifest_version":3,"name":"Kernel Terraform acceptance","version":"1.0.0"}`)
	testAccWriteExtensionFile(t, writer, "marker.txt", "extension-data-source")
	if err := writer.Close(); err != nil {
		t.Fatalf("close extension ZIP: %v", err)
	}
	return archive.Bytes()
}

func testAccWriteExtensionFile(t *testing.T, writer *zip.Writer, name, contents string) {
	t.Helper()

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create %s in extension ZIP: %v", name, err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write %s in extension ZIP: %v", name, err)
	}
}

func testAccExtensionDataSourceConfig(id, name, projectID string) string {
	return acctest.ProviderConfig() + fmt.Sprintf(`
data "kernel_extension" "by_id" {
  id         = %q
  project_id = %q
}

data "kernel_extension" "by_name" {
  name = %q
}
`, id, projectID, name)
}
