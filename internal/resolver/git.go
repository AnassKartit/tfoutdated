package resolver

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
)

// gitLsRemoteTimeout bounds a single ls-remote. Private hosts that want
// credentials we do not have should fail fast, not stall the scan.
const gitLsRemoteTimeout = 20 * time.Second

// IsGitSource reports whether a module source is fetched by git rather than
// from the Terraform registry.
//
// This matters well beyond cosmetics: these sources used to be handed to the
// registry API, which of course could not resolve them, and the resulting error
// silently dropped the dependency. Internal modules — the ones an organisation
// actually owns — are exactly the ones served from a private git host, so the
// tool reported a clean bill of health on the dependencies that matter most.
func IsGitSource(source string) bool {
	s := strings.ToLower(source)
	switch {
	case strings.HasPrefix(s, "git::"),
		strings.HasPrefix(s, "git@"),
		strings.HasPrefix(s, "ssh://"):
		return true
	case strings.Contains(s, "dev.azure.com"),
		strings.Contains(s, "visualstudio.com"),
		strings.Contains(s, "gitlab.com"),
		strings.Contains(s, "bitbucket.org"):
		return true
	case strings.HasPrefix(s, "github.com/") && strings.Count(s, "/") >= 2:
		return true
	}
	return strings.Contains(s, ".git")
}

// GitRef pulls the pinned ref out of a source string, i.e. the "v1" of
// "git::https://host/org/_git/repo?ref=v1".
func GitRef(source string) string {
	idx := strings.Index(source, "?")
	if idx < 0 {
		return ""
	}
	q, err := url.ParseQuery(source[idx+1:])
	if err != nil {
		return ""
	}
	return q.Get("ref")
}

// GitRemoteURL turns a Terraform module source into something git can clone.
func GitRemoteURL(source string) string {
	s := strings.TrimPrefix(source, "git::")
	if idx := strings.Index(s, "?"); idx >= 0 {
		s = s[:idx]
	}
	// Terraform marks a subdirectory inside a repository with "//", as in
	// "github.com/org/repo//modules/vnet". That separator must be looked for
	// past the scheme, otherwise the "//" of "https://" is mistaken for it and
	// the URL is truncated to "https:".
	searchFrom := 0
	if i := strings.Index(s, "://"); i >= 0 {
		searchFrom = i + 3
	}
	if i := strings.Index(s[searchFrom:], "//"); i >= 0 {
		s = s[:searchFrom+i]
	}
	if strings.HasPrefix(s, "github.com/") {
		s = "https://" + s
	}
	return s
}

// GitTagVersions lists semver tags on a remote, newest last.
//
// It never prompts: a host that wants credentials we do not have must fail
// quickly so the scan can report the dependency as unresolved rather than hang.
func GitTagVersions(ctx context.Context, remote string) ([]*goversion.Version, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, gitLsRemoteTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", remote)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // never block on a username prompt
		"GIT_ASKPASS=true",
		"GCM_INTERACTIVE=never",
	)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git ls-remote timed out after %s", gitLsRemoteTimeout)
		}
		return nil, fmt.Errorf("git ls-remote %s: %w", remote, err)
	}

	var versions []*goversion.Version
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		tag := strings.TrimPrefix(parts[1], "refs/tags/")
		v, err := goversion.NewVersion(strings.TrimPrefix(tag, "v"))
		if err != nil {
			continue // non-semver tags such as "latest" are not upgrade targets
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no semver tags on %s", remote)
	}
	sort.Sort(goversion.Collection(versions))
	return versions, nil
}
