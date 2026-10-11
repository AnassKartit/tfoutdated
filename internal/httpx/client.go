// Package httpx provides the one HTTP client the rest of the tool should use.
//
// It exists because three separate call sites had drifted apart: two had a 30s
// timeout, one used http.DefaultClient (no timeout at all, so a stalled registry
// hung the CLI forever), and only one of them ever sent the GitHub token the user
// had exported. None retried, so a single 429 from the GitHub API — easy to hit,
// the unauthenticated budget is 60 requests per hour — ended the run with a bare
// "github returned 403".
package httpx

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds a single attempt, not the whole retry sequence.
	DefaultTimeout = 30 * time.Second
	// MaxAttempts counts the first try plus retries.
	MaxAttempts = 4
	baseBackoff = 500 * time.Millisecond
	maxBackoff  = 8 * time.Second
)

// Client wraps http.Client with retry and GitHub authentication.
type Client struct {
	HTTP        *http.Client
	MaxAttempts int
	// Sleep is swappable so tests do not actually wait.
	Sleep func(time.Duration)
}

// New returns a client with sane defaults.
func New() *Client {
	return &Client{
		HTTP:        &http.Client{Timeout: DefaultTimeout},
		MaxAttempts: MaxAttempts,
		Sleep:       func(d time.Duration) { time.Sleep(d) },
	}
}

// GitHubToken returns the token to authenticate GitHub calls with, checking the
// variables both the GitHub CLI and Actions set. Callers must not read the
// environment themselves, or the token stops applying uniformly.
func GitHubToken() string {
	for _, k := range []string{"TFOUTDATED_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// isGitHub reports whether a host is one we should authenticate to.
func isGitHub(host string) bool {
	return host == "api.github.com" ||
		host == "github.com" ||
		strings.HasSuffix(host, ".githubusercontent.com")
}

// Get performs a GET with retries on transient failures.
//
// The caller owns the response body on success. On failure every intermediate
// body has already been drained and closed.
func (c *Client) Get(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	attempts := c.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if isGitHub(req.URL.Host) {
			if tok := GitHubToken(); tok != "" {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			// A cancelled context is the user's decision, not a transient fault.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
		} else if shouldRetry(resp.StatusCode) {
			wait := retryAfter(resp, attempt)
			drain(resp)
			lastErr = fmt.Errorf("%s returned %d", req.URL.Host, resp.StatusCode)
			if attempt < attempts {
				if !c.sleepCtx(ctx, wait) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, annotate(lastErr, req.URL.Host)
		} else {
			return resp, nil
		}

		if attempt < attempts {
			if !c.sleepCtx(ctx, backoff(attempt)) {
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", attempts, lastErr)
}

// annotate turns a rate-limit failure into advice the user can act on.
func annotate(err error, host string) error {
	if isGitHub(host) && GitHubToken() == "" {
		return fmt.Errorf("%w — GitHub allows only 60 unauthenticated requests per hour; "+
			"set GITHUB_TOKEN to raise the limit to 5000", err)
	}
	return err
}

// shouldRetry covers rate limiting and the transient 5xx family. A 403 from
// GitHub is almost always the rate limiter rather than a real permission problem,
// so it is worth one more try once the backoff has elapsed.
func shouldRetry(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusForbidden,
		http.StatusRequestTimeout, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryAfter honours the server's own pacing when it gives one, in either the
// delay-seconds or the GitHub X-RateLimit-Reset epoch form.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
			return capWait(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return capWait(time.Until(t))
		}
	}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return capWait(time.Until(time.Unix(epoch, 0)))
		}
	}
	return backoff(attempt)
}

func capWait(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// backoff grows exponentially with jitter, so parallel workers that trip the
// rate limit together do not all come back at the same instant.
func backoff(attempt int) time.Duration {
	d := baseBackoff << (attempt - 1)
	if d > maxBackoff {
		d = maxBackoff
	}
	return d + time.Duration(rand.Int63n(int64(d/2+1)))
}

// sleepCtx waits, reporting false if the context ended first. Going through
// Client.Sleep keeps tests instant instead of making them wait out the backoff.
func (c *Client) sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	if c.Sleep != nil {
		c.Sleep(d)
		return ctx.Err() == nil
	}
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// drain lets the connection be reused instead of being torn down.
func drain(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
