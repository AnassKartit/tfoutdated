// Package lifecycle detects modules whose upstream has stopped: a GitHub
// repository that is archived, a README that announces the deprecation, or an
// Azure Verified Module marked Deprecated in Microsoft's module index.
//
// The Terraform Registry's own "deprecated" flag is not enough on its own: AVM
// modules are archived and deprecated upstream while the registry still reports
// them as current.
package lifecycle

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anasskartit/tfoutdated/internal/httpx"
)

const (
	defaultGitHubAPI = "https://api.github.com"
	defaultRawGitHub = "https://raw.githubusercontent.com"
	defaultAVMIndex  = "https://raw.githubusercontent.com/Azure/Azure-Verified-Modules/main/docs/static/module-indexes"

	maxBodyBytes = 4 << 20
	// A deprecation notice sits at the top of a README; reading further only
	// finds unrelated mentions of "deprecated" in variable descriptions.
	readmeHeadBytes = 4000
)

// avmIndexFiles are the Terraform module indexes Microsoft publishes, with the
// human-readable page that shows each one.
var avmIndexFiles = []struct{ file, page string }{
	{"TerraformResourceModules.csv", "https://azure.github.io/Azure-Verified-Modules/indexes/terraform/tf-resource-modules/"},
	{"TerraformPatternModules.csv", "https://azure.github.io/Azure-Verified-Modules/indexes/terraform/tf-pattern-modules/"},
	{"TerraformUtilityModules.csv", "https://azure.github.io/Azure-Verified-Modules/indexes/terraform/tf-utility-modules/"},
}

type avmEntry struct {
	status string
	page   string
}

// Status is what we learned about one module's upstream. Each signal is kept
// separately so a report can say which source said what.
type Status struct {
	RepoURL       string
	Archived      bool
	ArchivedAt    time.Time // last push to an archived repo, close to the archive date
	AVMStatus     string    // Available, Orphaned, Deprecated, Proposed; empty when not an AVM module
	ReadmeNotice  bool      // the latest version's README announces deprecation
	ReleaseNotice bool      // the latest GitHub release notes announce deprecation
	ReleaseTag    string    // tag of that release
	ReplacedBy    string    // module source the notice points to, when it names one
	MovedTo       string    // "owner/repo" GitHub now serves the repository under, when renamed or transferred

	// Evidence links each signal to the page that shows it, so a reader can
	// verify every claim instead of taking the tool's word for it.
	Evidence []Evidence

	links []string // registry links found after a notice on GitHub, before self-links are dropped
}

// Evidence is one verifiable signal: what was found and where to see it.
type Evidence struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Stopped reports whether the module should be treated as deprecated.
func (s Status) Stopped() bool {
	return s.Archived || s.ReadmeNotice || s.ReleaseNotice || strings.EqualFold(s.AVMStatus, "Deprecated")
}

// NeedsAttention reports any signal worth a look, including a moved repository,
// which still works through GitHub's redirect but should be updated.
func (s Status) NeedsAttention() bool {
	return s.Stopped() || s.MovedTo != ""
}

// Message explains which signals fired, so a reader can check each one.
func (s Status) Message() string {
	var parts []string
	if s.ReadmeNotice {
		parts = append(parts, "README of latest version announces deprecation")
	}
	if s.ReleaseNotice {
		parts = append(parts, "release notes "+s.ReleaseTag+" announce deprecation")
	}
	if strings.EqualFold(s.AVMStatus, "Deprecated") {
		parts = append(parts, "AVM index: Deprecated")
	}
	if s.Archived {
		msg := "GitHub repository archived"
		if !s.ArchivedAt.IsZero() {
			msg += " (last push " + s.ArchivedAt.Format("2006-01-02") + ")"
		}
		parts = append(parts, msg)
	}
	if s.MovedTo != "" {
		parts = append(parts, "GitHub repository moved to "+s.MovedTo)
	}
	return strings.Join(parts, "; ")
}

// Checker looks up module lifecycle signals. It is safe for concurrent use and
// queries each repository once per run.
type Checker struct {
	http      *httpx.Client
	githubAPI string
	rawGitHub string
	avmIndex  string

	avmOnce sync.Once
	avm     map[string]avmEntry // module name -> status and index page
	avmErr  error

	mu    sync.Mutex
	repos map[string]repoResult
}

type repoResult struct {
	status Status
	err    error
}

// Options overrides the endpoints, for tests.
type Options struct {
	GitHubAPI string
	RawGitHub string
	AVMIndex  string
}

// New returns a Checker against the public endpoints unless overridden.
func New(opts Options) *Checker {
	c := &Checker{
		http:      httpx.New(),
		githubAPI: defaultGitHubAPI,
		rawGitHub: defaultRawGitHub,
		avmIndex:  defaultAVMIndex,
		repos:     map[string]repoResult{},
	}
	if opts.GitHubAPI != "" {
		c.githubAPI = strings.TrimRight(opts.GitHubAPI, "/")
	}
	if opts.RawGitHub != "" {
		c.rawGitHub = strings.TrimRight(opts.RawGitHub, "/")
	}
	if opts.AVMIndex != "" {
		c.avmIndex = strings.TrimRight(opts.AVMIndex, "/")
	}
	return c
}

// Check returns the lifecycle status of a registry module.
//
// source is the module source as written ("Azure/avm-ptn-virtualwan/azurerm");
// repoURL is the GitHub URL the registry reports for it; readme is the README
// the registry serves for the latest version. Any of them may be empty.
//
// The README comes from the published module package itself, so it is the most
// reliable signal and costs no GitHub call. GitHub failures (rate limits, a
// private repository) are returned but never hide the signals already found.
func (c *Checker) Check(ctx context.Context, source, repoURL, readme string) (Status, error) {
	var st Status
	var links []string // replacement candidates, most authoritative first

	readmeFromRegistry := readme != ""
	if readmeFromRegistry {
		st.ReadmeNotice, links = parseNotice(readme)
	}

	owner, repo, ok := parseGitHub(repoURL)

	name := moduleName(source)
	if !strings.HasPrefix(name, "avm-") && ok {
		// A git source names the repository, not the module.
		name = avmNameFromRepo(repo)
	}
	var avmPage string
	if strings.HasPrefix(name, "avm-") {
		if entry, found := c.avmEntry(ctx, name); found {
			st.AVMStatus = entry.status
			avmPage = entry.page
		}
	}

	var ghErr error
	if ok {
		key := owner + "/" + repo
		c.mu.Lock()
		cached, seen := c.repos[key]
		c.mu.Unlock()
		if !seen {
			cached.status, cached.err = c.checkRepo(ctx, owner, repo)
			c.mu.Lock()
			c.repos[key] = cached
			c.mu.Unlock()
		}

		gh := cached.status
		st.RepoURL = gh.RepoURL
		st.Archived = gh.Archived
		st.ArchivedAt = gh.ArchivedAt
		st.ReleaseNotice = gh.ReleaseNotice
		st.ReleaseTag = gh.ReleaseTag
		st.MovedTo = gh.MovedTo
		// No README from the registry: fall back to the one on the default branch.
		if !readmeFromRegistry {
			st.ReadmeNotice = gh.ReadmeNotice
		}
		links = append(links, gh.links...)
		ghErr = cached.err
	}
	st.ReplacedBy = replacementFor(source, links)
	st.Evidence = st.evidence(source, readmeFromRegistry, avmPage)
	return st, ghErr
}

// evidence lists, for every signal that fired, the page a reader can open to
// see it for themselves. The order matches Message.
func (s Status) evidence(source string, readmeFromRegistry bool, avmPage string) []Evidence {
	var ev []Evidence
	if s.ReadmeNotice {
		url := RegistryURL(source, "")
		if !readmeFromRegistry || url == "" {
			url = s.RepoURL + "#readme"
		}
		ev = append(ev, Evidence{"README of latest version announces deprecation", url})
	}
	if s.ReleaseNotice && s.RepoURL != "" {
		ev = append(ev, Evidence{"Release notes " + s.ReleaseTag + " announce deprecation", s.RepoURL + "/releases/tag/" + s.ReleaseTag})
	}
	if strings.EqualFold(s.AVMStatus, "Deprecated") && avmPage != "" {
		ev = append(ev, Evidence{"AVM index: Deprecated", avmPage})
	}
	if s.Archived && s.RepoURL != "" {
		label := "GitHub repository archived"
		if !s.ArchivedAt.IsZero() {
			label += " (last push " + s.ArchivedAt.Format("2006-01-02") + ")"
		}
		ev = append(ev, Evidence{label, s.RepoURL})
	}
	if s.MovedTo != "" {
		ev = append(ev, Evidence{"Repository moved to " + s.MovedTo, "https://github.com/" + s.MovedTo})
	}
	return ev
}

var githubJSON = map[string]string{"Accept": "application/vnd.github+json"}

func (c *Checker) checkRepo(ctx context.Context, owner, repo string) (Status, error) {
	st := Status{RepoURL: "https://github.com/" + owner + "/" + repo}

	body, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/%s", c.githubAPI, owner, repo), githubJSON)
	if err != nil {
		return st, fmt.Errorf("github %s/%s: %w", owner, repo, err)
	}
	var meta struct {
		FullName string    `json:"full_name"`
		Archived bool      `json:"archived"`
		PushedAt time.Time `json:"pushed_at"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return st, fmt.Errorf("parsing github %s/%s: %w", owner, repo, err)
	}
	st.Archived = meta.Archived
	if meta.Archived {
		st.ArchivedAt = meta.PushedAt
	}
	// GitHub answers a renamed or transferred repository through a redirect, so
	// a different full_name is how a move shows up.
	if meta.FullName != "" && !strings.EqualFold(meta.FullName, owner+"/"+repo) {
		st.MovedTo = meta.FullName
		owner, repo, _ = strings.Cut(meta.FullName, "/")
	}

	// Release notes are where maintainers announce a deprecation first, often
	// months before archiving. A repository without releases is not an error.
	if body, err := c.get(ctx, fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.githubAPI, owner, repo), githubJSON); err == nil {
		var rel struct {
			TagName string `json:"tag_name"`
			Body    string `json:"body"`
		}
		if json.Unmarshal(body, &rel) == nil {
			var links []string
			st.ReleaseNotice, links = parseNotice(rel.Body)
			if st.ReleaseNotice {
				st.ReleaseTag = rel.TagName
				st.links = append(st.links, links...)
			}
		}
	}

	if readme, err := c.get(ctx, fmt.Sprintf("%s/%s/%s/HEAD/README.md", c.rawGitHub, owner, repo), nil); err == nil {
		notice, links := parseNotice(string(readme))
		st.ReadmeNotice = notice
		// The README is maintained after the release, so its links come first.
		st.links = append(links, st.links...)
	}
	return st, nil
}

func (c *Checker) avmEntry(ctx context.Context, name string) (avmEntry, bool) {
	c.avmOnce.Do(func() {
		c.avm = map[string]avmEntry{}
		for _, idx := range avmIndexFiles {
			body, err := c.get(ctx, c.avmIndex+"/"+idx.file, nil)
			if err != nil {
				c.avmErr = err
				continue
			}
			if err := readAVMIndex(string(body), idx.page, c.avm); err != nil {
				c.avmErr = err
			}
		}
	})
	entry, ok := c.avm[name]
	return entry, ok
}

func (c *Checker) get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	resp, err := c.http.Get(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// readAVMIndex adds ModuleName -> ModuleStatus rows from one index CSV, noting
// the page that lists them.
func readAVMIndex(data, page string, into map[string]avmEntry) error {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(data, "\uFEFF")))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parsing AVM index: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	nameCol, statusCol := -1, -1
	for i, h := range rows[0] {
		switch strings.TrimSpace(h) {
		case "ModuleName":
			nameCol = i
		case "ModuleStatus":
			statusCol = i
		}
	}
	if nameCol < 0 || statusCol < 0 {
		return fmt.Errorf("AVM index: ModuleName/ModuleStatus columns not found")
	}
	for _, row := range rows[1:] {
		if nameCol < len(row) && statusCol < len(row) {
			into[strings.TrimSpace(row[nameCol])] = avmEntry{status: strings.TrimSpace(row[statusCol]), page: page}
		}
	}
	return nil
}

var (
	// Only module-level wording counts. An inline "Deprecation notice:" or "no
	// longer receive" also appears in notes about a single attribute or setting
	// (seen in avm-ptn-alz-connectivity-hub-and-spoke-vnet and avm-res-web-site),
	// so "deprecation notice" only counts as a heading.
	noticeRe = regexp.MustCompile(`(?im)(this (module|repository|repo) (is|has been)( now)? (deprecated|archived)` +
		`|this (module|repository|repo) (is now in extended support|will be archived)` +
		`|will no longer receive (any )?(updates|contributions)` +
		`|is no longer (being )?maintained` +
		`|^#{1,3}[^\n]*deprecation notice)`)
	// Registry links: registry.terraform.io/modules/<ns>/<name>/<provider>[/<version>][/submodules/<sub>]
	registryLinkRe = regexp.MustCompile(`registry\.terraform\.io/modules/([\w.-]+)/([\w.-]+)/([\w.-]+)(?:/[\w.-]+)?(?:/submodules/([\w.-]+))?`)
)

// parseNotice reports whether the head of a README or release notes announces
// deprecation and, if so, the registry modules it links to after the notice,
// in order. The caller drops links back to the module itself.
func parseNotice(text string) (notice bool, links []string) {
	head := text
	if len(head) > readmeHeadBytes {
		head = head[:readmeHeadBytes]
	}
	loc := noticeRe.FindStringIndex(head)
	if loc == nil {
		return false, nil
	}
	// The replacement is linked after the notice, not in badges above it.
	for _, m := range registryLinkRe.FindAllStringSubmatch(head[loc[0]:], -1) {
		link := m[1] + "/" + m[2] + "/" + m[3]
		if m[4] != "" {
			link += "//modules/" + m[4]
		}
		links = append(links, link)
	}
	return true, links
}

// replacementFor returns the first link that is not the module itself.
func replacementFor(source string, links []string) string {
	self := strings.ToLower(baseSource(source))
	for _, l := range links {
		if strings.ToLower(baseSource(l)) != self {
			return l
		}
	}
	return ""
}

// baseSource drops any "//submodule" path from a module source.
func baseSource(source string) string {
	base, _, _ := strings.Cut(source, "//")
	return base
}

// moduleName returns the name segment of a registry source
// "<ns>/<name>/<provider>[//sub]", or "" for anything else.
func moduleName(source string) string {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git") {
		return ""
	}
	source, _, _ = strings.Cut(source, "//")
	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

// avmNameFromRepo maps "terraform-azurerm-avm-res-x" to "avm-res-x", the name
// AVM modules carry in the index; "" for repositories that are not AVM.
func avmNameFromRepo(repo string) string {
	parts := strings.SplitN(repo, "-", 3)
	if len(parts) == 3 && parts[0] == "terraform" && strings.HasPrefix(parts[2], "avm-") {
		return parts[2]
	}
	return ""
}

// parseGitHub extracts owner and repository from a github.com URL.
func parseGitHub(raw string) (owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	for _, p := range []string{"https://", "http://", "git::", "git@"} {
		raw = strings.TrimPrefix(raw, p)
	}
	raw = strings.Replace(raw, "github.com:", "github.com/", 1)
	rest, found := strings.CutPrefix(raw, "github.com/")
	if !found {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSuffix(rest, ".git"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}
