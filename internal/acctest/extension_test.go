package acctest

import (
	"context"
	"errors"
	"testing"
)

type fakeExtensionCleaner struct {
	defaultProjectID string
	delete           func(context.Context, string, string) error
}

func (f fakeExtensionCleaner) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeExtensionCleaner) DeleteExtension(ctx context.Context, projectID, id string) error {
	return f.delete(ctx, projectID, id)
}

func TestCleanupExtension(t *testing.T) {
	tests := map[string]struct {
		acceptance     string
		apiKey         string
		projectID      string
		defaultProject string
		id             string
		deleteErr      error
		wantProjectID  string
		wantCleanups   int
		wantDelete     bool
		wantFailure    bool
	}{
		"empty ID is ignored": {},
		"acceptance disabled": {
			apiKey:      "test-key",
			id:          "extension_123",
			wantFailure: true,
		},
		"API key missing": {
			acceptance:  "1",
			id:          "extension_123",
			wantFailure: true,
		},
		"default project is resolved": {
			acceptance:     "1",
			apiKey:         "test-key",
			defaultProject: "project_default",
			id:             "extension_123",
			wantProjectID:  "project_default",
			wantCleanups:   1,
			wantDelete:     true,
		},
		"not found is already clean": {
			acceptance:    "1",
			apiKey:        "test-key",
			projectID:     "project_explicit",
			id:            "extension_123",
			deleteErr:     notFoundAPIError(),
			wantProjectID: "project_explicit",
			wantCleanups:  1,
			wantDelete:    true,
		},
		"delete error is reported": {
			acceptance:    "1",
			apiKey:        "test-key",
			projectID:     "project_explicit",
			id:            "extension_123",
			deleteErr:     errors.New("connection reset"),
			wantProjectID: "project_explicit",
			wantCleanups:  1,
			wantDelete:    true,
			wantFailure:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvAcceptance, test.acceptance)
			t.Setenv(EnvAPIKey, test.apiKey)

			var gotID, gotProjectID string
			deleteCalled := false
			deadlineSet := false
			recorder := &testRecorder{TB: t}
			cleanupExtension(recorder, fakeExtensionCleaner{
				defaultProjectID: test.defaultProject,
				delete: func(ctx context.Context, projectID, id string) error {
					deleteCalled = true
					_, deadlineSet = ctx.Deadline()
					gotProjectID = projectID
					gotID = id
					return test.deleteErr
				},
			}, test.projectID, test.id)

			if got, want := len(recorder.cleanups), test.wantCleanups; got != want {
				t.Fatalf("cleanupExtension registered %d cleanups, want %d", got, want)
			}
			if test.wantCleanups == 1 {
				recorder.cleanups[0]()
			}
			if recorder.failed != test.wantFailure {
				t.Fatalf("cleanupExtension failure = %t, want %t", recorder.failed, test.wantFailure)
			}
			if deleteCalled != test.wantDelete {
				t.Fatalf("cleanupExtension called delete = %t, want %t", deleteCalled, test.wantDelete)
			}
			if deleteCalled && !deadlineSet {
				t.Fatal("cleanupExtension called delete without a context deadline")
			}
			if test.wantDelete && (gotID != test.id || gotProjectID != test.wantProjectID) {
				t.Fatalf("cleanup extension scope/id = %q/%q, want %q/%q", gotProjectID, gotID, test.wantProjectID, test.id)
			}
		})
	}
}
