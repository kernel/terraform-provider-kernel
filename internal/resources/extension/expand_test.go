package extension

import (
	"io"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	_, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringUnknown(),
		SourceSHA256: types.StringValue(snapshot.checksum),
	}, snapshot)
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
			_, diags := expandExtensionUpload(extensionModel{
				Name:         types.StringNull(),
				SourceSHA256: value,
			}, archiveSnapshot{})
			assertExtensionUploadDiagnostic(t, diags, "source_sha256", "must be known")
		})
	}
}

func TestExpandExtensionUploadRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	configured := strings.Repeat("a", 64)
	actual := strings.Repeat("b", 64)
	_, diags := expandExtensionUpload(extensionModel{
		Name:         types.StringNull(),
		SourceSHA256: types.StringValue(configured),
	}, archiveSnapshot{checksum: actual})
	assertExtensionUploadDiagnostic(t, diags, "source_sha256", "archive checksum is "+actual)
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
