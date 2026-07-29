package acctest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	kernel "github.com/kernel/kernel-go-sdk"
	"github.com/kernel/terraform-provider-kernel/internal/kernelclient"
	"github.com/kernel/terraform-provider-kernel/internal/provider"
)

const (
	EnvAcceptance = "KERNEL_ACC"
	EnvAPIKey     = "KERNEL_API_KEY"
	EnvBaseURL    = "KERNEL_BASE_URL"
	EnvProjectID  = "KERNEL_PROJECT_ID"
	// EnvAltProjectID names a second project for cross-project override tests.
	EnvAltProjectID = "KERNEL_ALT_PROJECT_ID"

	cleanupTimeout = 2 * time.Minute
)

func PreCheck(t testing.TB) {
	t.Helper()

	if !AcceptanceEnabled() {
		t.Skipf("set %s=1 to run Kernel acceptance tests", EnvAcceptance)
	}
	if os.Getenv(EnvAPIKey) == "" {
		t.Fatalf("%s must be set when %s=1", EnvAPIKey, EnvAcceptance)
	}
}

func AcceptanceEnabled() bool {
	return os.Getenv(EnvAcceptance) == "1"
}

func ProviderConfig() string {
	return `provider "kernel" {}` + "\n"
}

func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"kernel": providerserver.NewProtocol6WithError(provider.New("acceptance")()),
	}
}

func UniqueName(t testing.TB, prefix string) string {
	t.Helper()

	stem := sanitizeNamePart(prefix)
	if stem == "" {
		stem = "test"
	}

	timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	suffix := randomHex(t, 4)
	name := "kernel-tf-" + stem + "-" + timestamp + "-" + suffix
	if len(name) <= 63 {
		return name
	}

	over := len(name) - 63
	if over >= len(stem) {
		stem = "test"
	} else {
		stem = strings.Trim(stem[:len(stem)-over], "-")
	}
	return "kernel-tf-" + stem + "-" + timestamp + "-" + suffix
}

// CleanupBrowserPool registers a cleanup that deletes the pool from
// projectID; empty means the env-configured default project.
func CleanupBrowserPool(t testing.TB, projectID, id string) {
	t.Helper()

	cleanupBrowserPool(t, ClientFromEnv(), projectID, id)
}

type browserPoolCleaner interface {
	DefaultProjectID() string
	DeleteBrowserPool(context.Context, string, string) error
}

func cleanupBrowserPool(t testing.TB, client browserPoolCleaner, projectID, id string) {
	t.Helper()

	if id == "" {
		return
	}
	// The returns after Fatalf look dead for a real *testing.T (Fatalf never
	// returns) but are load-bearing for the testRecorder fake in the gate
	// tests, whose Fatalf only records the failure and returns.
	if !AcceptanceEnabled() {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAcceptance)
		return
	}
	if os.Getenv(EnvAPIKey) == "" {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAPIKey)
		return
	}

	if projectID == "" {
		projectID = client.DefaultProjectID()
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := client.DeleteBrowserPool(ctx, projectID, id); err != nil && !IsNotFound(err) {
			t.Errorf("cleanup Kernel browser pool %s: %v", id, err)
		}
	})
}

func CleanupProject(t testing.TB, id string) {
	t.Helper()

	cleanupProject(t, ClientFromEnv(), id)
}

type projectCleaner interface {
	DeleteProject(context.Context, string) error
}

func cleanupProject(t testing.TB, client projectCleaner, id string) {
	t.Helper()

	if id == "" {
		return
	}
	if !AcceptanceEnabled() {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAcceptance)
		return
	}
	if os.Getenv(EnvAPIKey) == "" {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAPIKey)
		return
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := client.DeleteProject(ctx, id); err != nil && !IsNotFound(err) {
			t.Errorf("cleanup Kernel project %s: %v", id, err)
		}
	})
}

// CleanupProfile registers a cleanup that deletes the profile from projectID;
// empty means the env-configured default project.
func CleanupProfile(t testing.TB, projectID, id string) {
	t.Helper()

	cleanupProfile(t, ClientFromEnv(), projectID, id)
}

type profileCleaner interface {
	DefaultProjectID() string
	DeleteProfile(context.Context, string, string) error
}

func cleanupProfile(t testing.TB, client profileCleaner, projectID, id string) {
	t.Helper()

	if id == "" {
		return
	}
	if !AcceptanceEnabled() {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAcceptance)
		return
	}
	if os.Getenv(EnvAPIKey) == "" {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAPIKey)
		return
	}

	if projectID == "" {
		projectID = client.DefaultProjectID()
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := client.DeleteProfile(ctx, projectID, id); err != nil && !IsNotFound(err) {
			t.Errorf("cleanup Kernel profile %s: %v", id, err)
		}
	})
}

// CleanupProxy registers a cleanup that deletes the proxy from projectID;
// empty means the env-configured default project.
func CleanupProxy(t testing.TB, projectID, id string) {
	t.Helper()

	cleanupProxy(t, ClientFromEnv(), projectID, id)
}

type proxyCleaner interface {
	DefaultProjectID() string
	DeleteProxy(context.Context, string, string) error
}

func cleanupProxy(t testing.TB, client proxyCleaner, projectID, id string) {
	t.Helper()

	if id == "" {
		return
	}
	if !AcceptanceEnabled() {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAcceptance)
		return
	}
	if os.Getenv(EnvAPIKey) == "" {
		t.Fatalf("%s must be set to clean up Kernel acceptance test resources", EnvAPIKey)
		return
	}

	if projectID == "" {
		projectID = client.DefaultProjectID()
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := client.DeleteProxy(ctx, projectID, id); err != nil && !IsNotFound(err) {
			t.Errorf("cleanup Kernel proxy %s: %v", id, err)
		}
	})
}

func ClientFromEnv() kernelclient.Clients {
	return kernelclient.New(kernelclient.Config{
		APIKey:    os.Getenv(EnvAPIKey),
		BaseURL:   os.Getenv(EnvBaseURL),
		ProjectID: os.Getenv(EnvProjectID),
	})
}

// IsNotFound reports whether err is a Kernel API 404, which acceptance
// checks treat as "resource gone" rather than a failure.
func IsNotFound(err error) bool {
	var kernelErr *kernel.Error
	return errors.As(err, &kernelErr) && kernelErr.StatusCode == http.StatusNotFound
}

func sanitizeNamePart(value string) string {
	value = strings.ToLower(value)

	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if lastHyphen {
			continue
		}
		b.WriteByte('-')
		lastHyphen = true
	}

	return strings.Trim(b.String(), "-")
}

func randomHex(t testing.TB, bytes int) string {
	t.Helper()

	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("generate acceptance test suffix: %v", err)
	}
	return hex.EncodeToString(buf)
}
