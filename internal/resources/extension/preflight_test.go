package extension

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPrepareExtensionUploadUsesOneVerifiedSnapshot(t *testing.T) {
	t.Parallel()

	data := []byte("extension archive")
	sourcePath := writeExtensionArchiveForTest(t, data)
	params, diags := prepareExtensionUpload(extensionModel{
		Name:         types.StringValue("Extension"),
		SourcePath:   types.StringValue(sourcePath),
		SourceSHA256: types.StringValue("9602532cc2b8bc6ca42336f7d93a1f49b5272bf308cd4a6a132e0074efd76fbc"),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !params.Name.Valid() || params.Name.Value != "Extension" {
		t.Fatalf("name = %#v, want Extension", params.Name)
	}
	if err := os.WriteFile(sourcePath, []byte("replacement"), 0o600); err != nil {
		t.Fatalf("replace extension archive: %v", err)
	}
	upload, err := io.ReadAll(params.File)
	if err != nil {
		t.Fatalf("read upload: %v", err)
	}
	if got, want := string(upload), string(data); got != want {
		t.Fatalf("upload data = %q, want %q", got, want)
	}
}

func TestPrepareExtensionUploadRequiresKnownSourcePath(t *testing.T) {
	t.Parallel()

	for name, sourcePath := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			params, diags := prepareExtensionUpload(extensionModel{
				SourcePath:   sourcePath,
				SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
			})
			assertEmptyExtensionUploadParams(t, params)
			assertExtensionUploadDiagnostic(t, diags, "source_path", "must be configured and known")
		})
	}
}

func TestPrepareExtensionUploadReportsArchiveOpenFailure(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), "missing.zip")
	params, diags := prepareExtensionUpload(extensionModel{
		SourcePath:   types.StringValue(sourcePath),
		SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
	})
	assertEmptyExtensionUploadParams(t, params)
	assertExtensionUploadDiagnostic(t, diags, "source_path", "open extension archive")
	if !strings.Contains(diags[0].Detail(), sourcePath) {
		t.Fatalf("diagnostic detail = %q, want source path", diags[0].Detail())
	}
}

func TestPrepareExtensionUploadRejectsChangedArchive(t *testing.T) {
	t.Parallel()

	params, diags := prepareExtensionUpload(extensionModel{
		SourcePath:   types.StringValue(writeExtensionArchiveForTest(t, []byte("changed"))),
		SourceSHA256: types.StringValue(strings.Repeat("a", 64)),
	})
	assertEmptyExtensionUploadParams(t, params)
	assertExtensionUploadDiagnostic(t, diags, "source_sha256", "checksum")
}

func writeExtensionArchiveForTest(t *testing.T, data []byte) string {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "extension.zip")
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		t.Fatalf("write extension archive: %v", err)
	}
	return sourcePath
}
