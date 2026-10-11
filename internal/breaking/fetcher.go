package breaking

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/httpx"
)

// Known GitHub changelog URLs for major providers.
var providerChangelogURLs = map[string]string{
	"azurerm": "https://raw.githubusercontent.com/hashicorp/terraform-provider-azurerm/main/CHANGELOG.md",
	"azuread": "https://raw.githubusercontent.com/hashicorp/terraform-provider-azuread/main/CHANGELOG.md",
	"azapi":   "https://raw.githubusercontent.com/Azure/terraform-provider-azapi/main/CHANGELOG.md",
	"aws":     "https://raw.githubusercontent.com/hashicorp/terraform-provider-aws/main/CHANGELOG.md",
	"google":  "https://raw.githubusercontent.com/hashicorp/terraform-provider-google/main/CHANGELOG.md",
}

// maxChangelogBytes caps a changelog read. The azurerm CHANGELOG is the largest
// we fetch at roughly 1 MB; this leaves headroom without letting a bad response
// pull an unbounded amount into memory.
const maxChangelogBytes = 16 << 20

// Fetcher retrieves changelogs from GitHub.
type Fetcher struct {
	client *httpx.Client
}

// NewFetcher creates a new changelog fetcher.
//
// It goes through httpx so that the GitHub token the user exported actually
// applies here too. Previously this fetcher authenticated to nothing, so half
// the tool's GitHub traffic stayed anonymous and burned the 60/hour budget.
func NewFetcher() *Fetcher {
	return &Fetcher{client: httpx.New()}
}

// FetchChangelog downloads the CHANGELOG for a provider.
func (f *Fetcher) FetchChangelog(provider string) (string, error) {
	url, ok := providerChangelogURLs[provider]
	if !ok {
		return "", fmt.Errorf("no changelog URL known for provider %q", provider)
	}

	return f.fetchURL(url)
}

// FetchChangelogURL downloads a changelog from a specific URL.
func (f *Fetcher) FetchChangelogURL(url string) (string, error) {
	return f.fetchURL(url)
}

// ModuleChangelogURL builds a GitHub raw changelog URL from a Terraform registry module source.
// e.g., "Azure/avm-res-network-bastionhost/azurerm" → "https://raw.githubusercontent.com/Azure/terraform-azurerm-avm-res-network-bastionhost/main/CHANGELOG.md"
func ModuleChangelogURL(source string) string {
	// Parse source: "namespace/name/provider" format
	parts := strings.Split(source, "/")
	if len(parts) < 3 {
		return ""
	}

	namespace := parts[0]
	name := parts[1]
	provider := parts[2]

	// Terraform convention: GitHub repo is "terraform-{provider}-{name}"
	repo := fmt.Sprintf("terraform-%s-%s", provider, name)

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/CHANGELOG.md", namespace, repo)
}

// FetchModuleChangelog downloads the CHANGELOG for a module given its registry source string.
func (f *Fetcher) FetchModuleChangelog(source string) (string, error) {
	url := ModuleChangelogURL(source)
	if url == "" {
		return "", fmt.Errorf("cannot build changelog URL from module source %q", source)
	}

	return f.fetchURL(url)
}

func (f *Fetcher) fetchURL(url string) (string, error) {
	body, err := f.get(url)
	if err == nil {
		return body, nil
	}
	// The default branch is not always "main". Rather than fail on projects that
	// still use "master", try the other name once before giving up.
	if alt := swapDefaultBranch(url); alt != "" {
		if body, altErr := f.get(alt); altErr == nil {
			return body, nil
		}
	}
	return "", err
}

// swapDefaultBranch returns the same raw.githubusercontent URL pointed at the
// other conventional default branch, or "" if the URL uses neither.
func swapDefaultBranch(url string) string {
	switch {
	case strings.Contains(url, "/main/"):
		return strings.Replace(url, "/main/", "/master/", 1)
	case strings.Contains(url, "/master/"):
		return strings.Replace(url, "/master/", "/main/", 1)
	}
	return ""
}

func (f *Fetcher) get(url string) (string, error) {
	resp, err := f.client.Get(context.Background(), url, nil)
	if err != nil {
		return "", fmt.Errorf("fetching changelog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("changelog fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChangelogBytes))
	if err != nil {
		return "", fmt.Errorf("reading changelog body: %w", err)
	}

	return string(body), nil
}
