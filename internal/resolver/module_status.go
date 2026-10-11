package resolver

import (
	"context"
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"
)

// ModuleStatus answers "can I use this module?" for one source, without a
// Terraform configuration around it. It is what an editor or a coding agent
// needs before it writes a module block.
type ModuleStatus struct {
	Source        string `json:"source"`
	LatestVersion string `json:"latest_version,omitempty"`
	RepoURL       string `json:"repo_url,omitempty"`

	Deprecated bool   `json:"deprecated"`
	Archived   bool   `json:"archived"`
	MovedTo    string `json:"moved_to,omitempty"`
	AVMStatus  string `json:"avm_status,omitempty"`
	ReplacedBy string `json:"replaced_by,omitempty"`
	Reason     string `json:"reason,omitempty"`

	// Advice is one sentence a person or an agent can act on.
	Advice string `json:"advice"`
}

// ModuleStatus looks up the lifecycle of a registry or git module source.
func (r *Resolver) ModuleStatus(ctx context.Context, source string) (*ModuleStatus, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("empty module source")
	}
	st := &ModuleStatus{Source: source}

	var versions []*goversion.Version
	var detail *ModuleDetail
	var err error
	if IsGitSource(source) {
		detail = &ModuleDetail{RepoURL: GitRemoteURL(source)}
		versions, err = GitTagVersions(ctx, GitRemoteURL(source))
	} else {
		if detail, err = r.registry.FetchModuleDetail(source); err != nil {
			return nil, fmt.Errorf("registry lookup %s: %w", source, err)
		}
		versions, err = r.registry.GetModuleVersions(source)
	}
	if err == nil && len(versions) > 0 {
		st.LatestVersion = versions[len(versions)-1].String()
	}
	st.RepoURL = detail.RepoURL
	st.Deprecated = detail.Deprecated
	st.ReplacedBy = detail.ReplacedBy

	var lcErr error
	if r.lifecycle != nil {
		lc, e := r.lifecycle.Check(ctx, source, detail.RepoURL, detail.Readme)
		lcErr = e
		st.Archived = lc.Archived
		st.MovedTo = lc.MovedTo
		st.AVMStatus = lc.AVMStatus
		st.Reason = lc.Message()
		if lc.Stopped() {
			st.Deprecated = true
		}
		if lc.ReplacedBy != "" {
			st.ReplacedBy = lc.ReplacedBy
		}
		if lc.RepoURL != "" {
			st.RepoURL = lc.RepoURL
		}
	}

	switch {
	case st.Deprecated && st.ReplacedBy != "":
		st.Advice = "Do not use for new code; use " + st.ReplacedBy + " instead and plan a migration for existing code."
	case st.Deprecated:
		st.Advice = "Do not use for new code; it no longer receives updates. Look for a maintained alternative."
	case st.MovedTo != "":
		st.Advice = "Still usable, but the repository moved to " + st.MovedTo + "; update references."
	default:
		st.Advice = "No lifecycle issue found."
		if st.LatestVersion != "" {
			st.Advice += " Pin to " + st.LatestVersion + "."
		}
	}
	// A failed upstream check is reported, never mistaken for a clean bill of health.
	if lcErr != nil {
		st.Advice += " (upstream check incomplete: " + lcErr.Error() + ")"
	}
	return st, nil
}
