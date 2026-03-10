package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/anasskartit/tfoutdated/internal/output"
	"github.com/anasskartit/tfoutdated/internal/recommend"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

var recommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Generate governance recommendations",
	Long: `Analyze Terraform dependencies and generate actionable governance
recommendations including version fragmentation, unpinned versions,
major version drift, deprecated modules, and stale dependencies.`,
	RunE: runRecommend,
}

func init() {
	rootCmd.AddCommand(recommendCmd)
}

func runRecommend(cmd *cobra.Command, args []string) error {
	if flagJSON {
		flagOutput = "json"
	}

	path, err := filepath.Abs(flagPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	cfg := config.Load(path)

	s := scanner.New(scanner.Options{
		Path:      path,
		Recursive: flagRecursive,
		Ignores:   cfg.Ignore,
	})
	result, err := s.Scan()
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}

	if len(result.Modules) == 0 && len(result.Providers) == 0 {
		fmt.Println("No Terraform dependencies found.")
		return nil
	}

	res := resolver.New(resolver.Options{
		ProviderFilter: flagProviderFilter,
	})
	resolved, err := res.Resolve(result)
	if err != nil {
		return fmt.Errorf("resolving versions: %w", err)
	}

	a := analyzer.New()
	analysis := a.Analyze(resolved)

	analysis.BreakingChanges = detectAllBreakingChanges(resolved)

	// Generate recommendations
	recs := recommend.Recommend(analysis)
	analysis.Recommendations = recs

	if len(recs) == 0 {
		fmt.Println("No recommendations — looking good!")
		return nil
	}

	out, err := output.New(flagOutput, output.Options{
		NoColor: flagNoColor,
	})
	if err != nil {
		return err
	}
	return out.Render(os.Stdout, analysis)
}
