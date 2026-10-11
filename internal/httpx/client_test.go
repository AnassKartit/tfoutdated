package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client that never really sleeps, so the backoff logic
// can be exercised without the test taking seconds.
func newTestClient() (*Client, *[]time.Duration) {
	var slept []time.Duration
	c := New()
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	return c, &slept
}

func TestGetRetriesOnRateLimitThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	c, slept := newTestClient()
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("want 3 attempts, got %d", got)
	}
	if len(*slept) != 2 {
		t.Errorf("want 2 backoffs, got %d", len(*slept))
	}
	// Retry-After: 1 must be honoured rather than replaced by our own backoff.
	if len(*slept) > 0 && (*slept)[0] != time.Second {
		t.Errorf("want Retry-After honoured (1s), got %v", (*slept)[0])
	}
}

func TestGetGivesUpAndExplainsTheRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c, _ := newTestClient()
	_, err := c.Get(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("want an error once attempts are exhausted")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should name the status, got %v", err)
	}
}

func TestGetDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := newTestClient()
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("a 404 is a real answer, not an error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 passed through, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("404 must not be retried, got %d attempts", got)
	}
}

func TestGetStopsWhenContextIsCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := New()
	c.Sleep = func(time.Duration) { cancel() } // user hits Ctrl+C during the backoff

	if _, err := c.Get(ctx, srv.URL, nil); err == nil {
		t.Fatal("want an error once the context is cancelled")
	}
}

func TestGitHubTokenPrefersMostSpecific(t *testing.T) {
	t.Setenv("GH_TOKEN", "gh")
	t.Setenv("GITHUB_TOKEN", "github")
	if got := GitHubToken(); got != "github" {
		t.Errorf("GITHUB_TOKEN should win over GH_TOKEN, got %q", got)
	}
	t.Setenv("TFOUTDATED_GITHUB_TOKEN", "specific")
	if got := GitHubToken(); got != "specific" {
		t.Errorf("tool-specific token should win, got %q", got)
	}
}

func TestGetSendsTokenOnlyToGitHub(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret")
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	c, _ := newTestClient()
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// The test server is not GitHub, so the credential must not leak to it.
	if auth != "" {
		t.Errorf("token leaked to a non-GitHub host: %q", auth)
	}
}
