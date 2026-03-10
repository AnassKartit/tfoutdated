package schemadiff

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// ModuleFetcher fetches and parses Terraform module schemas from the registry.
type ModuleFetcher struct {
	cacheDir string
	mu       sync.Mutex
}

// NewModuleFetcher creates a fetcher with a cache directory.
func NewModuleFetcher() *ModuleFetcher {
	cacheDir := filepath.Join(os.TempDir(), "tfoutdated-schema-cache")
	os.MkdirAll(cacheDir, 0755)
	return &ModuleFetcher{cacheDir: cacheDir}
}

// registryVersionInfo holds the response from the registry version metadata endpoint.
type registryVersionInfo struct {
	Source string `json:"source"`
	Tag    string `json:"tag"`
}

// FetchModuleSchema fetches and parses a module at a specific version.
// source is the registry source like "Azure/avm-res-network-virtualnetwork/azurerm"
func (f *ModuleFetcher) FetchModuleSchema(source, version string) (*tfconfig.Module, error) {
	// Check cache first
	cacheKey := strings.ReplaceAll(source, "/", "_") + "_" + version
	cacheDir := filepath.Join(f.cacheDir, cacheKey)

	f.mu.Lock()
	defer f.mu.Unlock()

	if mod, err := f.loadCached(cacheDir); err == nil {
		return mod, nil
	}

	// Get GitHub repo info from registry
	owner, repo, tag, err := f.resolveGitHubSource(source, version)
	if err != nil {
		return nil, fmt.Errorf("resolving source: %w", err)
	}

	// Download and extract tarball
	if err := f.downloadAndExtract(owner, repo, tag, cacheDir); err != nil {
		return nil, fmt.Errorf("downloading module: %w", err)
	}

	return f.loadCached(cacheDir)
}

func (f *ModuleFetcher) loadCached(dir string) (*tfconfig.Module, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("not cached")
	}

	mod, diags := tfconfig.LoadModule(dir)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing module: %s", diags.Error())
	}
	return mod, nil
}

// resolveGitHubSource gets the GitHub owner, repo, and tag for a registry module version.
func (f *ModuleFetcher) resolveGitHubSource(source, version string) (owner, repo, tag string, err error) {
	parts := strings.Split(source, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid module source: %s", source)
	}

	// First try the registry metadata endpoint
	url := fmt.Sprintf("https://registry.terraform.io/v1/modules/%s/%s/%s/%s",
		parts[0], parts[1], parts[2], version)

	resp, err := http.Get(url)
	if err != nil {
		return "", "", "", fmt.Errorf("registry request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Fall back to convention-based resolution
		return f.resolveByConvention(parts, version)
	}

	var info registryVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return f.resolveByConvention(parts, version)
	}

	// Parse GitHub URL from source field
	// e.g., "https://github.com/Azure/terraform-azurerm-avm-res-network-virtualnetwork"
	ghSource := info.Source
	if ghSource == "" {
		return f.resolveByConvention(parts, version)
	}

	ghSource = strings.TrimPrefix(ghSource, "https://github.com/")
	ghSource = strings.TrimPrefix(ghSource, "http://github.com/")
	ghParts := strings.Split(ghSource, "/")
	if len(ghParts) < 2 {
		return f.resolveByConvention(parts, version)
	}

	tag = info.Tag
	if tag == "" {
		tag = "v" + version
	}

	return ghParts[0], ghParts[1], tag, nil
}

func (f *ModuleFetcher) resolveByConvention(parts []string, version string) (string, string, string, error) {
	// Convention: Azure/avm-res-foo-bar/azurerm → github.com/Azure/terraform-azurerm-avm-res-foo-bar
	owner := parts[0]
	repo := fmt.Sprintf("terraform-%s-%s", parts[2], parts[1])
	tag := "v" + version
	return owner, repo, tag, nil
}

// downloadAndExtract downloads a GitHub tarball and extracts .tf files.
func (f *ModuleFetcher) downloadAndExtract(owner, repo, tag, destDir string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tarball/%s", owner, repo, tag)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	// Use GITHUB_TOKEN if available
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("github returned %d for %s/%s@%s", resp.StatusCode, owner, repo, tag)
	}

	os.MkdirAll(destDir, 0755)

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Only extract root-level .tf files (skip subdirectories/modules)
		// Tarball has a prefix dir like "Azure-terraform-azurerm-...-abc1234/"
		name := header.Name
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue
		}
		relPath := parts[1]

		// Only root-level .tf files
		if strings.Contains(relPath, "/") {
			continue
		}
		if !strings.HasSuffix(relPath, ".tf") {
			continue
		}

		destFile := filepath.Join(destDir, relPath)
		outFile, err := os.Create(destFile)
		if err != nil {
			return fmt.Errorf("creating %s: %w", relPath, err)
		}
		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			return fmt.Errorf("writing %s: %w", relPath, err)
		}
		outFile.Close()
	}

	return nil
}
