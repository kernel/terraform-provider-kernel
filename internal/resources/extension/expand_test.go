package extension

import (
	"io"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
)

func TestExpandExtensionUploadMapsVerifiedSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := archiveSnapshot{
		data:     []byte("extension archive"),
		checksum: strings.Repeat("a", 64),
	}
	params, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringValue("Extension"),
		SourceSHA256: types.StringValue(snapshot.checksum),
	}, snapshot)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !params.Name.Valid() || params.Name.Value != "Extension" {
		t.Fatalf("name = %#v, want Extension", params.Name)
	}
	data, err := io.ReadAll(params.File)
	if err != nil {
		t.Fatalf("read upload file: %v", err)
	}
	if got, want := string(data), string(snapshot.data); got != want {
		t.Fatalf("upload data = %q, want %q", got, want)
	}
}

func TestExpandExtensionUploadOmitsNullName(t *testing.T) {
	t.Parallel()

	snapshot := archiveSnapshot{checksum: strings.Repeat("a", 64)}
	params, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringNull(),
		SourceSHA256: types.StringValue(snapshot.checksum),
	}, snapshot)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if params.Name.Valid() {
		t.Fatalf("name = %#v, want omitted", params.Name)
	}
}

func TestExpandExtensionUploadRejectsUnknownName(t *testing.T) {
	t.Parallel()

	snapshot := archiveSnapshot{checksum: strings.Repeat("a", 64)}
	params, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringUnknown(),
		SourceSHA256: types.StringValue(snapshot.checksum),
	}, snapshot)
	assertEmptyExtensionUploadParams(t, params)
	assertExtensionUploadDiagnostic(t, diags, "name", "must be known")
}

func TestExpandExtensionUploadRejectsUnknownOrNullChecksum(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			params, diags := expandExtensionUpload(extensionModel{
				Name:         types.StringNull(),
				SourceSHA256: value,
			}, archiveSnapshot{})
			assertEmptyExtensionUploadParams(t, params)
			assertExtensionUploadDiagnostic(t, diags, "source_sha256", "must be known")
		})
	}
}

func TestExpandExtensionUploadRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	configured := strings.Repeat("a", 64)
	actual := strings.Repeat("b", 64)
	params, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringNull(),
		SourceSHA256: types.StringValue(configured),
	}, archiveSnapshot{checksum: actual})
	assertEmptyExtensionUploadParams(t, params)
	assertExtensionUploadDiagnostic(t, diags, "source_sha256", "archive checksum is "+actual)
}

func TestExpandExtensionUploadReportsAllInvalidAttributes(t *testing.T) {
	t.Parallel()

	params, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringUnknown(),
		SourceSHA256: types.StringNull(),
	}, archiveSnapshot{})
	assertEmptyExtensionUploadParams(t, params)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %v, want two errors", diags)
	}
	for _, attribute := range []string{"name", "source_sha256"} {
		if !extensionUploadDiagnosticHasPath(diags, path.Root(attribute)) {
			t.Errorf("diagnostics = %v, want %s error", diags, attribute)
		}
	}
}

func assertEmptyExtensionUploadParams(t *testing.T, params kernel.ExtensionUploadParams) {
	t.Helper()

	if params.File != nil || params.Name.Valid() {
		t.Fatalf("params = %#v, want no upload parameters on diagnostics", params)
	}
}

func assertExtensionUploadDiagnostic(t *testing.T, diags diag.Diagnostics, attribute, detail string) {
	t.Helper()

	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one error", diags)
	}
	if !strings.Contains(diags[0].Detail(), detail) {
		t.Fatalf("diagnostic detail = %q, want it to contain %q", diags[0].Detail(), detail)
	}
	withPath, ok := diags[0].(diag.DiagnosticWithPath)
	if !ok || !withPath.Path().Equal(path.Root(attribute)) {
		t.Fatalf("diagnostic path = %v, want %s", withPath, attribute)
	}
}

func extensionUploadDiagnosticHasPath(diags diag.Diagnostics, want path.Path) bool {
	for _, diagnostic := range diags {
		withPath, ok := diagnostic.(diag.DiagnosticWithPath)
		if ok && withPath.Path().Equal(want) {
			return true
		}
	}
	return false
}
