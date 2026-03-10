package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/anasskartit/tfoutdated/internal/multirepo"
	"github.com/anasskartit/tfoutdated/internal/output"
	"github.com/anasskartit/tfoutdated/internal/recommend"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

var (
	flagImpact        string
	flagTargetVersion string
	flagFull          bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for outdated Terraform dependencies",
	Long: `Scan Terraform configurations for outdated modules and providers,
analyze impact of upgrades, and detect breaking changes.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringVar(&flagImpact, "impact", "", "Provider impact analysis (e.g., hashicorp/azurerm)")
	scanCmd.Flags().StringVar(&flagTargetVersion, "target-version", "", "Target provider version for impact analysis")
	scanCmd.Flags().BoolVar(&flagFull, "full", false, "Full report: scan + breaking + recommendations + impact")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	if flagJSON {
		flagOutput = "json"
	}

	// CI auto-detection
	if flagOutput == "table" || flagOutput == "" {
		if os.Getenv("TF_BUILD") != "" {
			flagOutput = "azdevops"
		}
		if os.Getenv("GITHUB_ACTIONS") != "" {
			flagOutput = "github"
		}
	}

	// Multi-repo support
	if flagReposFile != "" {
		return runMultiRepoScan()
	}

	// Comma-separated multi-path support
	if strings.Contains(flagPath, ",") {
		return runMultiPathScan(strings.Split(flagPath, ","))
	}

	path, err := filepath.Abs(flagPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	analysis, result, res, err := runScanPipeline(path)
	if err != nil {
		return err
	}
	if analysis == nil {
		fmt.Println("No Terraform dependencies found.")
		return nil
	}

	// Provider impact analysis
	if flagImpact != "" || flagFull {
		provider := flagImpact
		if provider == "" {
			provider = autoDetectProvider(analysis)
		}
		targetVer := flagTargetVersion
		if targetVer == "" {
			targetVer = findLatestProviderVersion(analysis, provider)
		}
		if provider != "" && targetVer != "" {
			impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, targetVer)
			if err == nil {
				analysis.ProviderImpact = impact
			}
		}
	}

	// Recommendations (for --full mode)
	if flagFull {
		recs := recommend.Recommend(analysis)
		analysis.Recommendations = recs
	}

	// Filter by severity
	analysis = analysis.FilterBySeverity(flagSeverity)

	if len(analysis.Dependencies) == 0 && len(analysis.Recommendations) == 0 && analysis.ProviderImpact == nil {
		fmt.Println("All dependencies are up to date.")
		return nil
	}

	// Output
	outputFormat := flagOutput
	if flagOutputFile != "" {
		if detected := formatFromFilename(flagOutputFile); detected != "" {
			outputFormat = detected
		}
	}

	out, err := output.New(outputFormat, output.Options{
		NoColor: flagNoColor,
	})
	if err != nil {
		return err
	}

	writer := os.Stdout
	if flagOutputFile != "" {
		f, err := os.Create(flagOutputFile)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		writer = f
		// Show table summary on stdout when writing non-table format to file
		if outputFormat != "table" {
			tableOut, _ := output.New("table", output.Options{NoColor: flagNoColor})
			if tableOut != nil {
				tableOut.Render(os.Stdout, analysis)
			}
		}
	}

	if err := out.Render(writer, analysis); err != nil {
		return fmt.Errorf("rendering output: %w", err)
	}

	if flagOutputFile != "" {
		fmt.Fprintf(os.Stderr, "\nReport written to %s\n", flagOutputFile)
	}

	// Show warnings for provider mismatches and unpinned versions (table output only)
	if outputFormat == "table" {
		printProviderWarnings(analysis)
	}

	// Exit code
	if analysis.HasBreakingChanges() {
		os.Exit(ExitBreakingChanges)
	}
	if len(analysis.Dependencies) > 0 {
		os.Exit(ExitUpdatesAvail)
	}
	return nil
}

func runScanPipeline(path string) (*analyzer.Analysis, *scanner.ScanResult, *resolver.Resolver, error) {
	start := time.Now()
	cfg := config.Load(path)

	s := scanner.New(scanner.Options{
		Path:      path,
		Recursive: flagRecursive,
		Ignores:   cfg.Ignore,
	})
	result, err := s.Scan()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scanning: %w", err)
	}

	totalDeps := len(result.Modules) + len(result.Providers)
	if totalDeps == 0 {
		return nil, nil, nil, nil
	}

	res := resolver.New(resolver.Options{
		ProviderFilter: flagProviderFilter,
	})
	resolved, err := res.Resolve(result)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving versions: %w", err)
	}

	a := analyzer.New()
	analysis := a.Analyze(resolved)

	// Populate scan metadata
	analysis.ScannedFiles = len(result.Files)
	analysis.TotalDeps = totalDeps
	analysis.UpToDate = totalDeps - len(analysis.Dependencies)
	analysis.DurationMs = time.Since(start).Milliseconds()

	breakingChanges := detectAllBreakingChanges(resolved)
	analysis.BreakingChanges = breakingChanges

	impacts := analyzer.AnalyzeImpact(analysis, result)
	analysis.Impacts = impacts

	analysis.UpgradePaths = analyzer.ComputeUpgradePaths(resolved, analysis.BreakingChanges)

	return analysis, result, res, nil
}

func runMultiRepoScan() error {
	tempDir, err := os.MkdirTemp("", "tfoutdated-repos-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	entries, err := multirepo.ResolveRepos(flagReposFile, tempDir)
	if err != nil {
		return err
	}
	defer multirepo.CleanupCloned(entries)

	if len(entries) == 0 {
		return fmt.Errorf("no valid repos found in %s", flagReposFile)
	}

	for i, repo := range entries {
		if len(entries) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("=== %s ===\n", repo.Name)
		}

		analysis, result, res, err := runScanPipeline(repo.LocalPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: error (%s): %v\n", repo.Name, err)
			continue
		}
		if analysis == nil {
			fmt.Printf("No Terraform dependencies found in %s.\n", repo.Name)
			continue
		}

		if flagImpact != "" || flagFull {
			provider := flagImpact
			if provider == "" {
				provider = autoDetectProvider(analysis)
			}
			targetVer := flagTargetVersion
			if targetVer == "" {
				targetVer = findLatestProviderVersion(analysis, provider)
			}
			if provider != "" && targetVer != "" {
				impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, targetVer)
				if err == nil {
					analysis.ProviderImpact = impact
				}
			}
		}

		if flagFull {
			recs := recommend.Recommend(analysis)
			analysis.Recommendations = recs
		}

		analysis = analysis.FilterBySeverity(flagSeverity)

		out, err := output.New(flagOutput, output.Options{
			NoColor: flagNoColor,
		})
		if err != nil {
			return err
		}
		if err := out.Render(os.Stdout, analysis); err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: render error (%s): %v\n", repo.Name, err)
		}
	}

	return nil
}

func runMultiPathScan(paths []string) error {
	for i, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		absPath, err := filepath.Abs(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: skipping %s: %v\n", p, err)
			continue
		}

		if len(paths) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("=== %s ===\n", filepath.Base(absPath))
		}

		analysis, result, res, err := runScanPipeline(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: error (%s): %v\n", p, err)
			continue
		}
		if analysis == nil {
			fmt.Printf("No Terraform dependencies found in %s.\n", p)
			continue
		}

		if flagImpact != "" || flagFull {
			provider := flagImpact
			if provider == "" {
				provider = autoDetectProvider(analysis)
			}
			targetVer := flagTargetVersion
			if targetVer == "" {
				targetVer = findLatestProviderVersion(analysis, provider)
			}
			if provider != "" && targetVer != "" {
				impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, targetVer)
				if err == nil {
					analysis.ProviderImpact = impact
				}
			}
		}

		if flagFull {
			recs := recommend.Recommend(analysis)
			analysis.Recommendations = recs
		}

		analysis = analysis.FilterBySeverity(flagSeverity)

		out, err := output.New(flagOutput, output.Options{NoColor: flagNoColor})
		if err != nil {
			return err
		}
		if err := out.Render(os.Stdout, analysis); err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: render error (%s): %v\n", p, err)
		}
	}
	return nil
}

func printProviderWarnings(analysis *analyzer.Analysis) {
	// Flag unpinned or loosely pinned versions
	var unpinned []struct {
		Name       string
		Constraint string
		File       string
		Line       int
	}
	for _, dep := range analysis.Dependencies {
		c := strings.TrimSpace(dep.Constraint)
		if c == "" || c == "latest" || c == "*" ||
			(strings.HasPrefix(c, ">=") && !strings.Contains(c, "<") && !strings.Contains(c, ",")) {
			unpinned = append(unpinned, struct {
				Name       string
				Constraint string
				File       string
				Line       int
			}{dep.Name, c, dep.FilePath, dep.Line})
		}
	}
	if len(unpinned) > 0 {
		fmt.Fprintf(os.Stderr, "\nWarning: %d dependency(s) not pinned to a specific version:\n", len(unpinned))
		for _, u := range unpinned {
			c := u.Constraint
			if c == "" {
				c = "(none)"
			}
			fmt.Fprintf(os.Stderr, "  %s:%d  %s  version = \"%s\"\n", u.File, u.Line, u.Name, c)
		}
	}
}

func autoDetectProvider(analysis *analyzer.Analysis) string {
	for _, dep := range analysis.Dependencies {
		if !dep.IsModule {
			src := strings.ToLower(dep.Source)
			if strings.Contains(src, "azurerm") {
				return "hashicorp/azurerm"
			}
		}
	}
	// Default to azurerm for Azure-focused repos
	return "hashicorp/azurerm"
}

func findLatestProviderVersion(analysis *analyzer.Analysis, provider string) string {
	for _, dep := range analysis.Dependencies {
		if !dep.IsModule && strings.EqualFold(dep.Source, provider) {
			return dep.LatestVer
		}
	}
	return ""
}
