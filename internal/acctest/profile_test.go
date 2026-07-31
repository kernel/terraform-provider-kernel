package acctest

import (
	"context"
	"errors"
	"testing"
)

type fakeProfileCleaner struct {
	defaultProjectID string
	delete           func(context.Context, string, string) error
}

func (f fakeProfileCleaner) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeProfileCleaner) DeleteProfile(ctx context.Context, projectID, id string) error {
	return f.delete(ctx, projectID, id)
}

func TestCleanupProfile(t *testing.T) {
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
			id:          "profile_123",
			wantFailure: true,
		},
		"API key missing": {
			acceptance:  "1",
			id:          "profile_123",
			wantFailure: true,
		},
		"default project is resolved": {
			acceptance:     "1",
			apiKey:         "test-key",
			defaultProject: "project_default",
			id:             "profile_123",
			wantProjectID:  "project_default",
			wantCleanups:   1,
			wantDelete:     true,
		},
		"not found is already clean": {
			acceptance:    "1",
			apiKey:        "test-key",
			projectID:     "project_explicit",
			id:            "profile_123",
			deleteErr:     notFoundAPIError(),
			wantProjectID: "project_explicit",
			wantCleanups:  1,
			wantDelete:    true,
		},
		"delete error is reported": {
			acceptance:    "1",
			apiKey:        "test-key",
			projectID:     "project_explicit",
			id:            "profile_123",
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
			cleanupProfile(recorder, fakeProfileCleaner{
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
				t.Fatalf("cleanupProfile registered %d cleanups, want %d", got, want)
			}
			if test.wantCleanups == 1 {
				recorder.cleanups[0]()
			}
			if recorder.failed != test.wantFailure {
				t.Fatalf("cleanupProfile failure = %t, want %t", recorder.failed, test.wantFailure)
			}
			if deleteCalled != test.wantDelete {
				t.Fatalf("cleanupProfile called delete = %t, want %t", deleteCalled, test.wantDelete)
			}
			if deleteCalled && !deadlineSet {
				t.Fatal("cleanupProfile called delete without a context deadline")
			}
			if test.wantDelete && (gotID != test.id || gotProjectID != test.wantProjectID) {
				t.Fatalf("cleanup profile scope/id = %q/%q, want %q/%q", gotProjectID, gotID, test.wantProjectID, test.id)
			}
		})
	}
}
