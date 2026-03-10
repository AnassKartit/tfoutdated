package resolver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
)

// RegistryClient interacts with the Terraform Registry API.
type RegistryClient struct {
	baseURL    string
	httpClient *http.Client
}

// moduleVersionsResponse is the API response for module versions.
type moduleVersionsResponse struct {
	Modules []struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	} `json:"modules"`
}

// providerVersionsResponse is the API response for provider versions.
type providerVersionsResponse struct {
	Versions []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

// NewRegistryClient creates a new Terraform Registry HTTP client.
func NewRegistryClient(baseURL string) *RegistryClient {
	return &RegistryClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetModuleVersions fetches all versions for a module from the registry.
// source should be in format "namespace/name/provider" (e.g., "Azure/avm-res-compute-virtualmachine/azurerm").
func (c *RegistryClient) GetModuleVersions(source string) ([]*goversion.Version, error) {
	parts := strings.Split(source, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid module source: %s (expected namespace/name/provider)", source)
	}

	// Use the last 3 parts: namespace/name/provider
	ns := parts[len(parts)-3]
	name := parts[len(parts)-2]
	provider := parts[len(parts)-1]

	url := fmt.Sprintf("%s/v1/modules/%s/%s/%s/versions", c.baseURL, ns, name, provider)

	body, err := c.doGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetching module versions for %s: %w", source, err)
	}

	var resp moduleVersionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing module versions response: %w", err)
	}

	if len(resp.Modules) == 0 {
		return nil, fmt.Errorf("no module data returned for %s", source)
	}

	var versions []*goversion.Version
	for _, v := range resp.Modules[0].Versions {
		ver, err := goversion.NewVersion(v.Version)
		if err != nil {
			continue
		}
		// Skip prerelease versions
		if ver.Prerelease() != "" {
			continue
		}
		versions = append(versions, ver)
	}

	sort.Sort(goversion.Collection(versions))
	return versions, nil
}

// GetProviderVersions fetches all versions for a provider from the registry.
func (c *RegistryClient) GetProviderVersions(namespace, name string) ([]*goversion.Version, error) {
	url := fmt.Sprintf("%s/v1/providers/%s/%s/versions", c.baseURL, namespace, name)

	body, err := c.doGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetching provider versions for %s/%s: %w", namespace, name, err)
	}

	var resp providerVersionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing provider versions response: %w", err)
	}

	var versions []*goversion.Version
	for _, v := range resp.Versions {
		ver, err := goversion.NewVersion(v.Version)
		if err != nil {
			continue
		}
		if ver.Prerelease() != "" {
			continue
		}
		versions = append(versions, ver)
	}

	sort.Sort(goversion.Collection(versions))
	return versions, nil
}

// ModuleDetail contains lifecycle metadata from the Terraform Registry.
type ModuleDetail struct {
	Deprecated bool   `json:"deprecated"`
	ReplacedBy string `json:"replaced_by,omitempty"`
}

// ProviderDep represents a provider dependency constraint from a module version.
type ProviderDep struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
}

// moduleDetailResponse is the API response for module detail.
type moduleDetailResponse struct {
	Deprecated bool   `json:"deprecated"`
	ReplacedBy string `json:"replaced_by"`
}

// moduleVersionDetailResponse is the API response for a specific module version.
type moduleVersionDetailResponse struct {
	Root struct {
		ProviderDependencies []struct {
			Name      string `json:"name"`
			Source    string `json:"source"`
			Namespace string `json:"namespace"`
			Version   string `json:"version"`
		} `json:"provider_dependencies"`
	} `json:"root"`
}

// FetchModuleDetail fetches deprecation status and replacement info for a module.
func (c *RegistryClient) FetchModuleDetail(source string) (*ModuleDetail, error) {
	parts := strings.Split(source, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid module source: %s", source)
	}

	ns := parts[len(parts)-3]
	name := parts[len(parts)-2]
	provider := parts[len(parts)-1]

	url := fmt.Sprintf("%s/v1/modules/%s/%s/%s", c.baseURL, ns, name, provider)

	body, err := c.doGet(url)
	if err != nil {
		return nil, err
	}

	var resp moduleDetailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing module detail: %w", err)
	}

	return &ModuleDetail{
		Deprecated: resp.Deprecated,
		ReplacedBy: resp.ReplacedBy,
	}, nil
}

// GetModuleProviderDeps fetches provider dependencies for a specific module version.
func (c *RegistryClient) GetModuleProviderDeps(source, version string) ([]ProviderDep, error) {
	parts := strings.Split(source, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid module source: %s", source)
	}

	ns := parts[len(parts)-3]
	name := parts[len(parts)-2]
	provider := parts[len(parts)-1]

	url := fmt.Sprintf("%s/v1/modules/%s/%s/%s/%s", c.baseURL, ns, name, provider, version)

	body, err := c.doGet(url)
	if err != nil {
		return nil, err
	}

	var resp moduleVersionDetailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing module version detail: %w", err)
	}

	var deps []ProviderDep
	for _, d := range resp.Root.ProviderDependencies {
		deps = append(deps, ProviderDep{
			Name:      d.Name,
			Source:    d.Source,
			Namespace: d.Namespace,
			Version:   d.Version,
		})
	}

	return deps, nil
}

func (c *RegistryClient) doGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return body, nil
}
