package lifecycle

import (
	"net/url"
	"strings"
)

const registryBase = "https://registry.terraform.io"

// RegistryURL returns the page a person opens to see a module: the Terraform
// Registry page for a registry source ("ns/name/provider[//modules/sub]"), or
// the repository for a git source. version "" means the latest version. It
// returns "" when the source cannot be linked (a local path, an unknown host).
func RegistryURL(source, version string) string {
	if isGit(source) {
		return gitWebURL(source)
	}
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return "" // local module: nothing upstream to link to
	}
	base, sub, _ := strings.Cut(source, "//")
	parts := strings.Split(base, "/")
	if len(parts) != 3 || strings.Contains(base, ":") {
		return ""
	}
	if version == "" {
		version = "latest"
	}
	u := registryBase + "/modules/" + strings.Join(parts, "/") + "/" + url.PathEscape(version)
	if sub != "" {
		// "//modules/vnet" in a source is "/submodules/vnet" on the registry.
		u += "/submodules/" + strings.TrimPrefix(strings.TrimPrefix(sub, "modules/"), "/")
	}
	return u
}

// ProviderURL returns the Terraform Registry page of a provider
// ("hashicorp/azurerm"), at a version or the latest.
func ProviderURL(source, version string) string {
	parts := strings.Split(source, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	if version == "" {
		version = "latest"
	}
	return registryBase + "/providers/" + source + "/" + url.PathEscape(version)
}

func isGit(source string) bool {
	s := strings.ToLower(source)
	return strings.HasPrefix(s, "git::") || strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "github.com/") || strings.HasPrefix(s, "https://")
}

// gitWebURL turns a git module source into the repository page, at the ref
// the source pins when it pins one:
// "git::https://github.com/org/repo.git//modules/x?ref=v1" ->
// "https://github.com/org/repo/tree/v1/modules/x".
func gitWebURL(source string) string {
	s := strings.TrimPrefix(source, "git::")
	ref := ""
	if i := strings.Index(s, "?"); i >= 0 {
		if q, err := url.ParseQuery(s[i+1:]); err == nil {
			ref = q.Get("ref")
		}
		s = s[:i]
	}
	s = strings.Replace(s, "git@github.com:", "https://github.com/", 1)
	if strings.HasPrefix(s, "github.com/") {
		s = "https://" + s
	}
	if !strings.HasPrefix(s, "https://") {
		return ""
	}
	// Split off a "//subdir" that follows the host, not the scheme's "//".
	rest := strings.TrimPrefix(s, "https://")
	repo, sub, _ := strings.Cut(rest, "//")
	u := "https://" + strings.TrimSuffix(repo, ".git")
	if ref != "" {
		u += "/tree/" + url.PathEscape(ref)
		if sub != "" {
			u += "/" + sub
		}
	}
	return u
}
