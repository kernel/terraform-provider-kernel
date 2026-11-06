package auditlogexportdestination

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
)

var _ destinationClient = kernelclient.Clients{}

const remoteDestination = `{
  "id": "dest_123",
  "type": "s3",
  "format": "jsonl.gz",
  "bucket": "audit-bucket",
  "region": "us-east-1",
  "prefix": "kernel",
  "role_arn": "arn:aws:iam::123456789012:role/kernel-audit",
  "status": "paused",
  "external_id": "ext_123",
  "kernel_role_arn": "arn:aws:iam::999999999999:role/kernel-exporter",
  "consecutive_failures": 3,
  "last_error": "access denied",
  "last_exported_cursor": "cursor",
  "created_at": "2026-11-01T00:00:00Z",
  "updated_at": "2026-11-02T00:00:00Z"
}`

func destinationForTest(t *testing.T, raw string) kernel.AuditLogExportDestination {
	t.Helper()
	var destination kernel.AuditLogExportDestination
	if err := json.Unmarshal([]byte(raw), &destination); err != nil {
		t.Fatalf("unmarshal destination: %v", err)
	}
	return destination
}

func model(t *testing.T) destinationModel {
	t.Helper()
	return destinationModel{
		ID:            types.StringValue("dest_123"),
		Type:          types.StringValue("s3"),
		Format:        types.StringValue("jsonl.gz"),
		Bucket:        types.StringValue("audit-bucket"),
		Region:        types.StringValue("us-east-1"),
		Prefix:        types.StringValue("kernel/"),
		RoleARN:       types.StringValue("arn:aws:iam::123456789012:role/kernel-audit"),
		KMSKeyID:      types.StringNull(),
		Status:        types.StringValue("paused"),
		ExternalID:    types.StringValue("ext_123"),
		KernelRoleARN: types.StringValue("arn:aws:iam::999999999999:role/kernel-exporter"),
	}
}

func marshal(t *testing.T, v json.Marshaler) string {
	t.Helper()
	body, err := v.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(body)
}

func TestExpandCreate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kmsKeyID types.String
		want     string
	}{
		"without KMS key": {
			kmsKeyID: types.StringNull(),
			want:     `{"bucket":"audit-bucket","format":"jsonl.gz","prefix":"kernel/","region":"us-east-1","role_arn":"arn:aws:iam::123456789012:role/kernel-audit","type":"s3"}`,
		},
		"with KMS key": {
			kmsKeyID: types.StringValue("alias/audit"),
			want:     `{"bucket":"audit-bucket","format":"jsonl.gz","prefix":"kernel/","region":"us-east-1","role_arn":"arn:aws:iam::123456789012:role/kernel-audit","type":"s3","kms_key_id":"alias/audit"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan := model(t)
			plan.KMSKeyID = test.kmsKeyID
			if got := marshal(t, expandCreate(plan)); got != test.want {
				t.Fatalf("create body = %s, want %s", got, test.want)
			}
		})
	}
}

func TestExpandUpdateSendsOnlyChangedFields(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate func(plan, state *destinationModel)
		want   string
	}{
		"unchanged": {
			mutate: func(*destinationModel, *destinationModel) {},
			want:   `{}`,
		},
		"bucket, region, prefix and role": {
			mutate: func(plan, _ *destinationModel) {
				plan.Bucket = types.StringValue("other-bucket")
				plan.Region = types.StringValue("us-gov-west-1")
				plan.Prefix = types.StringValue("audit/")
				plan.RoleARN = types.StringValue("arn:aws:iam::123456789012:role/other")
			},
			want: `{"bucket":"other-bucket","prefix":"audit/","region":"us-gov-west-1","role_arn":"arn:aws:iam::123456789012:role/other"}`,
		},
		"set KMS key": {
			mutate: func(plan, _ *destinationModel) {
				plan.KMSKeyID = types.StringValue("alias/audit")
			},
			want: `{"kms_key_id":"alias/audit"}`,
		},
		"clear KMS key": {
			mutate: func(_, state *destinationModel) {
				state.KMSKeyID = types.StringValue("alias/audit")
			},
			want: `{"kms_key_id":""}`,
		},
		"activate": {
			mutate: func(plan, _ *destinationModel) {
				plan.Status = types.StringValue("active")
			},
			want: `{"status":"active"}`,
		},
		"unknown status": {
			mutate: func(plan, _ *destinationModel) {
				plan.Status = types.StringUnknown()
			},
			want: `{}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan, state := model(t), model(t)
			test.mutate(&plan, &state)
			if got := marshal(t, expandUpdate(plan, state)); got != test.want {
				t.Fatalf("update body = %s, want %s", got, test.want)
			}
		})
	}
}

func TestFlattenKeepsDurableFieldsOnly(t *testing.T) {
	t.Parallel()

	got, diags := flatten(destinationForTest(t, remoteDestination), model(t).Prefix)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if want := model(t); got != want {
		t.Fatalf("flatten = %+v, want %+v", got, want)
	}
}

func TestFlattenPrefix(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		prefix types.String
		remote string
		want   string
	}{
		{"trailing slash", types.StringValue("kernel/"), "kernel", "kernel/"},
		{"multiple trailing slashes", types.StringValue("kernel///"), "kernel", "kernel///"},
		{"canonical", types.StringValue("kernel"), "kernel", "kernel"},
		{"empty", types.StringValue(""), "", ""},
		{"drift", types.StringValue("kernel/"), "other", "other"},
		{"import", types.StringNull(), "kernel", "kernel"},
		{"unknown", types.StringUnknown(), "kernel", "kernel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			remote := destinationForTest(t, remoteDestination)
			remote.Prefix = test.remote
			got, diags := flatten(remote, test.prefix)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got.Prefix.ValueString() != test.want {
				t.Fatalf("prefix = %v, want %q", got.Prefix, test.want)
			}
		})
	}
}

func TestFlattenKMSKey(t *testing.T) {
	t.Parallel()

	remote := destinationForTest(t, remoteDestination)
	remote.KmsKeyID = "alias/audit"
	got, diags := flatten(remote, model(t).Prefix)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.KMSKeyID.ValueString() != "alias/audit" {
		t.Fatalf("kms_key_id = %v, want alias/audit", got.KMSKeyID)
	}
}

func TestFlattenRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"id", "type", "format", "bucket", "region", "role_arn", "status", "external_id", "kernel_role_arn"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			var raw map[string]any
			if err := json.Unmarshal([]byte(remoteDestination), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			delete(raw, field)
			body, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			_, diags := flatten(destinationForTest(t, string(body)), model(t).Prefix)
			if !diags.HasError() {
				t.Fatalf("missing %s was accepted", field)
			}
		})
	}
}
