package resolver

import (
	"fmt"
	"strings"
	"sync"

	goversion "github.com/hashicorp/go-version"

	"github.com/anasskartit/tfoutdated/internal/scanner"
)

// UpdateType classifies the kind of version update available.
type UpdateType int

const (
	UpdateNone  UpdateType = iota
	UpdatePatch            // 3.75.0 → 3.75.1
	UpdateMinor            // 3.75.0 → 3.76.0
	UpdateMajor            // 3.75.0 → 4.0.0
)

func (u UpdateType) String() string {
	switch u {
	case UpdatePatch:
		return "PATCH"
	case UpdateMinor:
		return "MINOR"
	case UpdateMajor:
		return "MAJOR"
	default:
		return "NONE"
	}
}

// ResolvedDependency contains version resolution information for a dependency.
type ResolvedDependency struct {
	// Original dependency info
	Name       string
	Source     string
	Version    string // original constraint string
	FilePath   string
	Line       int
	IsModule   bool
	IsAVM      bool
	SourceType string
	Namespace  string // provider namespace

	// Resolved versions
	Current            *goversion.Version
	Latest             *goversion.Version
	AllVersions        []*goversion.Version
	UpdateType         UpdateType
	Constraint         goversion.Constraints
	LatestInConstraint *goversion.Version // latest satisfying current constraint

	// Deprecation info (modules only)
	Deprecated         bool
	ReplacedBy         string
	DeprecationMessage string
	ConstraintRaw      string // raw constraint string (e.g., "~> 3.75.0")
}

// ResolvedResult contains all resolved dependencies.
type ResolvedResult struct {
	Dependencies []ResolvedDependency
}

// Options configures the resolver.
type Options struct {
	ProviderFilter []string
	RegistryURL    string
	MaxConcurrency int
}

// Resolver resolves latest versions for dependencies.
type Resolver struct {
	opts     Options
	registry *RegistryClient
	cache    *Cache
}

// New creates a new Resolver.
func New(opts Options) *Resolver {
	if opts.RegistryURL == "" {
		opts.RegistryURL = "https://registry.terraform.io"
	}
	if opts.MaxConcurrency == 0 {
		opts.MaxConcurrency = 10
	}

	return &Resolver{
		opts:     opts,
		registry: NewRegistryClient(opts.RegistryURL),
		cache:    NewCache(),
	}
}

// Resolve fetches latest versions for all scanned dependencies.
func (r *Resolver) Resolve(scan *scanner.ScanResult) (*ResolvedResult, error) {
	result := &ResolvedResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.opts.MaxConcurrency)
	var errs []error

	// Resolve modules
	for _, mod := range scan.Modules {
		wg.Add(1)
		go func(m scanner.ModuleDependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resolved, err := r.resolveModule(m)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("module %s: %w", m.Name, err))
				mu.Unlock()
				return
			}
			if resolved != nil {
				mu.Lock()
				result.Dependencies = append(result.Dependencies, *resolved)
				mu.Unlock()
			}
		}(mod)
	}

	// Resolve providers
	for _, prov := range scan.Providers {
		if r.shouldFilterProvider(prov.Name) {
			continue
		}
		wg.Add(1)
		go func(p scanner.ProviderDependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resolved, err := r.resolveProvider(p)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("provider %s: %w", p.Name, err))
				mu.Unlock()
				return
			}
			if resolved != nil {
				mu.Lock()
				result.Dependencies = append(result.Dependencies, *resolved)
				mu.Unlock()
			}
		}(prov)
	}

	wg.Wait()

	if len(errs) > 0 && len(result.Dependencies) == 0 {
		return nil, fmt.Errorf("all resolutions failed: %v", errs)
	}

	return result, nil
}

func (r *Resolver) resolveModule(mod scanner.ModuleDependency) (*ResolvedDependency, error) {
	cacheKey := "module:" + mod.Source
	var versions []*goversion.Version
	if cached, ok := r.cache.GetVersions(cacheKey); ok {
		versions = cached
	} else {
		fetched, err := r.registry.GetModuleVersions(mod.Source)
		if err != nil {
			return nil, err
		}
		versions = fetched
		r.cache.SetVersions(cacheKey, versions)
	}

	resolved, err := r.buildResolved(mod.Name, mod.Source, mod.Version, mod.FilePath, mod.Line, true, mod.IsAVM, mod.SourceType, "", versions)
	if err != nil {
		return nil, err
	}

	// Fetch module detail for deprecation info
	detail, detailErr := r.registry.FetchModuleDetail(mod.Source)
	if detailErr == nil && detail != nil && detail.Deprecated {
		if resolved == nil {
			// Module is up-to-date but deprecated — still return it
			current := parseCurrentVersion(mod.Version)
			if current != nil && len(versions) > 0 {
				latest := versions[len(versions)-1]
				resolved = &ResolvedDependency{
					Name:       mod.Name,
					Source:     mod.Source,
					Version:    mod.Version,
					FilePath:   mod.FilePath,
					Line:       mod.Line,
					IsModule:   true,
					IsAVM:      mod.IsAVM,
					SourceType: mod.SourceType,
					Current:    current,
					Latest:     latest,
					AllVersions: versions,
					UpdateType: UpdateNone,
				}
			}
		}
		if resolved != nil {
			resolved.Deprecated = true
			resolved.ReplacedBy = detail.ReplacedBy
		}
	}

	if resolved != nil {
		resolved.ConstraintRaw = mod.Version
	}

	return resolved, nil
}

// GetRegistry returns the registry client for external use.
func (r *Resolver) GetRegistry() *RegistryClient {
	return r.registry
}

func (r *Resolver) resolveProvider(prov scanner.ProviderDependency) (*ResolvedDependency, error) {
	ns := prov.Namespace
	if ns == "" {
		ns = "hashicorp"
	}

	cacheKey := "provider:" + ns + "/" + prov.Name
	var versions []*goversion.Version
	if cached, ok := r.cache.GetVersions(cacheKey); ok {
		versions = cached
	} else {
		fetched, err := r.registry.GetProviderVersions(ns, prov.Name)
		if err != nil {
			return nil, err
		}
		versions = fetched
		r.cache.SetVersions(cacheKey, versions)
	}

	// Use locked version (from .terraform.lock.hcl) as the actual current version.
	// When no lock file exists and constraint is a range (e.g., ">= 3.80.0, < 5.0.0"),
	// use the latest version that satisfies the constraint as the effective current version.
	versionStr := prov.Version
	if prov.LockedVersion != "" {
		versionStr = prov.LockedVersion
	} else if isRangeConstraint(prov.Version) {
		// No lock file — find latest version satisfying the constraint
		constraint, err := goversion.NewConstraint(prov.Version)
		if err == nil && len(versions) > 0 {
			for i := len(versions) - 1; i >= 0; i-- {
				if constraint.Check(versions[i]) {
					versionStr = versions[i].Original()
					break
				}
			}
		}
	}

	resolved, err := r.buildResolved(prov.Name, ns+"/"+prov.Name, versionStr, prov.FilePath, prov.Line, false, false, "", ns, versions)
	if err != nil {
		return nil, err
	}
	if resolved != nil {
		resolved.ConstraintRaw = prov.Version
	}
	return resolved, nil
}

// isRangeConstraint returns true if a version string is a compound range constraint
// (e.g., ">= 3.80.0, < 5.0.0") rather than a simple version or pessimistic constraint.
func isRangeConstraint(v string) bool {
	return strings.Contains(v, ",") || strings.Contains(v, ">=") || strings.Contains(v, "~>")
}

func (r *Resolver) buildResolved(name, source, versionStr, filePath string, line int, isModule, isAVM bool, sourceType, namespace string, allVersions []*goversion.Version) (*ResolvedDependency, error) {
	if len(allVersions) == 0 {
		return nil, nil
	}

	// Parse current version from constraint
	current := parseCurrentVersion(versionStr)
	if current == nil {
		return nil, nil
	}

	// Find latest version
	latest := allVersions[len(allVersions)-1]

	// Parse constraint
	constraint, err := goversion.NewConstraint(versionStr)
	if err != nil {
		// Try adding a ">= " prefix for bare versions
		constraint, err = goversion.NewConstraint(">= " + versionStr)
		if err != nil {
			return nil, nil
		}
	}

	// Find latest version satisfying current constraint
	var latestInConstraint *goversion.Version
	for i := len(allVersions) - 1; i >= 0; i-- {
		if constraint.Check(allVersions[i]) {
			latestInConstraint = allVersions[i]
			break
		}
	}

	// Determine update type
	updateType := classifyUpdate(current, latest)

	if updateType == UpdateNone {
		return nil, nil
	}

	return &ResolvedDependency{
		Name:               name,
		Source:             source,
		Version:            versionStr,
		FilePath:           filePath,
		Line:               line,
		IsModule:           isModule,
		IsAVM:              isAVM,
		SourceType:         sourceType,
		Namespace:          namespace,
		Current:            current,
		Latest:             latest,
		AllVersions:        allVersions,
		UpdateType:         updateType,
		Constraint:         constraint,
		LatestInConstraint: latestInConstraint,
	}, nil
}

func (r *Resolver) shouldFilterProvider(name string) bool {
	if len(r.opts.ProviderFilter) == 0 {
		return false
	}
	for _, f := range r.opts.ProviderFilter {
		if strings.EqualFold(f, name) {
			return false
		}
	}
	return true
}

// parseCurrentVersion extracts the base version from a constraint string.
func parseCurrentVersion(constraint string) *goversion.Version {
	// Clean the constraint string to extract a version
	s := constraint
	// Remove common constraint operators
	for _, prefix := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
		s = strings.TrimPrefix(strings.TrimSpace(s), prefix)
	}
	s = strings.TrimSpace(s)

	// Handle compound constraints (take the first version)
	if idx := strings.Index(s, ","); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}

	v, err := goversion.NewVersion(s)
	if err != nil {
		return nil
	}
	return v
}

// classifyUpdate determines if an update is patch, minor, or major.
func classifyUpdate(current, latest *goversion.Version) UpdateType {
	if current.Equal(latest) || latest.LessThan(current) {
		return UpdateNone
	}

	cs := current.Segments()
	ls := latest.Segments()

	// Pad to at least 3 segments
	for len(cs) < 3 {
		cs = append(cs, 0)
	}
	for len(ls) < 3 {
		ls = append(ls, 0)
	}

	if cs[0] != ls[0] {
		return UpdateMajor
	}
	if cs[1] != ls[1] {
		return UpdateMinor
	}
	return UpdatePatch
}
