package breaking

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Known GitHub changelog URLs for major providers.
var providerChangelogURLs = map[string]string{
	"azurerm": "https://raw.githubusercontent.com/hashicorp/terraform-provider-azurerm/main/CHANGELOG.md",
	"azuread": "https://raw.githubusercontent.com/hashicorp/terraform-provider-azuread/main/CHANGELOG.md",
	"azapi":   "https://raw.githubusercontent.com/Azure/terraform-provider-azapi/main/CHANGELOG.md",
	"aws":     "https://raw.githubusercontent.com/hashicorp/terraform-provider-aws/main/CHANGELOG.md",
	"google":  "https://raw.githubusercontent.com/hashicorp/terraform-provider-google/main/CHANGELOG.md",
}

// Fetcher retrieves changelogs from GitHub.
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new changelog fetcher.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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
	resp, err := f.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching changelog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("changelog fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading changelog body: %w", err)
	}

	return string(body), nil
}
