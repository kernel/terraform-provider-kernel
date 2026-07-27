package acctest

import (
	"context"
	"errors"
	"testing"
)

type fakeProxyCleaner struct {
	defaultProjectID string
	delete           func(context.Context, string, string) error
}

func (f fakeProxyCleaner) DefaultProjectID() string {
	return f.defaultProjectID
}

func (f fakeProxyCleaner) DeleteProxy(ctx context.Context, projectID, id string) error {
	return f.delete(ctx, projectID, id)
}

func TestCleanupProxy(t *testing.T) {
	tests := map[string]struct {
		acceptance   string
		apiKey       string
		projectID    string
		id           string
		deleteErr    error
		wantProject  string
		wantCleanups int
		wantDelete   bool
		wantFailure  bool
	}{
		"empty ID is ignored": {},
		"acceptance disabled": {
			apiKey:      "test-key",
			id:          "proxy_123",
			wantFailure: true,
		},
		"API key missing": {
			acceptance:  "1",
			id:          "proxy_123",
			wantFailure: true,
		},
		"default project is used": {
			acceptance:   "1",
			apiKey:       "test-key",
			id:           "proxy_123",
			wantProject:  "proj_env",
			wantCleanups: 1,
			wantDelete:   true,
		},
		"explicit project is used": {
			acceptance:   "1",
			apiKey:       "test-key",
			projectID:    "proj_explicit",
			id:           "proxy_123",
			wantProject:  "proj_explicit",
			wantCleanups: 1,
			wantDelete:   true,
		},
		"not found is already clean": {
			acceptance:   "1",
			apiKey:       "test-key",
			id:           "proxy_123",
			deleteErr:    notFoundAPIError(),
			wantProject:  "proj_env",
			wantCleanups: 1,
			wantDelete:   true,
		},
		"delete error is reported": {
			acceptance:   "1",
			apiKey:       "test-key",
			id:           "proxy_123",
			deleteErr:    errors.New("connection reset"),
			wantProject:  "proj_env",
			wantCleanups: 1,
			wantDelete:   true,
			wantFailure:  true,
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
			cleanupProxy(recorder, fakeProxyCleaner{
				defaultProjectID: "proj_env",
				delete: func(ctx context.Context, projectID, id string) error {
					deleteCalled = true
					_, deadlineSet = ctx.Deadline()
					gotID = id
					gotProjectID = projectID
					return test.deleteErr
				},
			}, test.projectID, test.id)

			if got, want := len(recorder.cleanups), test.wantCleanups; got != want {
				t.Fatalf("cleanupProxy registered %d cleanups, want %d", got, want)
			}
			if test.wantCleanups == 1 {
				recorder.cleanups[0]()
			}
			if recorder.failed != test.wantFailure {
				t.Fatalf("cleanupProxy failure = %t, want %t", recorder.failed, test.wantFailure)
			}
			if deleteCalled != test.wantDelete {
				t.Fatalf("cleanupProxy called delete = %t, want %t", deleteCalled, test.wantDelete)
			}
			if deleteCalled && !deadlineSet {
				t.Fatal("cleanupProxy called delete without a context deadline")
			}
			if test.wantDelete && gotID != test.id {
				t.Fatalf("cleanup id = %q, want %q", gotID, test.id)
			}
			if test.wantDelete && gotProjectID != test.wantProject {
				t.Fatalf("cleanup project = %q, want %q", gotProjectID, test.wantProject)
			}
		})
	}
}
