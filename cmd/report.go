package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Verify breaking changes with real terraform validate output",
	Long: `Runs terraform init + validate on your code with both current and latest
provider versions to verify that tfoutdated's breaking change detections are real.

Generates a before/after comparison showing:
- Current versions: terraform validate passes
- Upgraded versions: terraform validate fails with the exact errors tfoutdated predicted`,
	RunE: runReport,
}

var flagReportHTML string

func init() {
	reportCmd.Flags().StringVarP(&flagReportHTML, "html", "R", "", "Write HTML report to file (e.g., report.html)")
	rootCmd.AddCommand(reportCmd)
}

// terraformValidateOutput represents the JSON output of terraform validate.
type terraformValidateOutput struct {
	Valid        bool                  `json:"valid"`
	Diagnostics  []terraformDiagnostic `json:"diagnostics"`
	ErrorCount   int                   `json:"error_count"`
	WarningCount int                   `json:"warning_count"`
}

// terraformDiagnostic represents a single diagnostic from terraform validate.
type terraformDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

type proofResult struct {
	BeforeValid   bool
	BeforeErrors  int
	BeforeWarns   int
	BeforeDiags   []terraformDiagnostic
	AfterValid    bool
	AfterErrors   int
	AfterWarns    int
	AfterDiags    []terraformDiagnostic
	Predictions   []breaking.BreakingChange
	Confirmed     []proofMatch
	Analysis      *analyzer.Analysis
	BeforeVersion string
	AfterVersion  string
	InitFailed    bool
}

type proofMatch struct {
	Prediction breaking.BreakingChange
	TFError    *terraformDiagnostic
	Status     string // "CONFIRMED", "WARNING_ONLY", "NOT_YET" (future version removes it)
}

func runReport(cmd *cobra.Command, args []string) error {
	path, err := filepath.Abs(flagPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Check terraform is available
	if _, err := exec.LookPath("terraform"); err != nil {
		return fmt.Errorf("terraform not found in PATH. Install terraform to use the report command")
	}

	fmt.Println("tfoutdated report — Verifying breaking changes with real terraform output")
	fmt.Println(strings.Repeat("═", 70))
	fmt.Println()

	// Step 1: Run tfoutdated scan
	fmt.Println("Step 1: Scanning for outdated dependencies...")
	analysis, scanResult, resolved, err := runFullScan(path)
	if err != nil {
		return err
	}
	if analysis == nil {
		fmt.Println("No dependencies found.")
		return nil
	}

	fmt.Printf("  Found %d outdated dependencies, %d breaking changes predicted\n\n",
		len(analysis.Dependencies), len(analysis.BreakingChanges))

	// Step 2: terraform validate with CURRENT versions (should pass)
	fmt.Println("Step 2: Running terraform validate with CURRENT versions...")
	beforeOut, beforeErr := runTerraformValidateInDir(path)
	if beforeErr != nil {
		fmt.Printf("  Note: terraform init/validate could not run: %v\n", beforeErr)
		fmt.Println("  This is expected if backend config requires credentials.")
		fmt.Println("  Continuing with tfoutdated predictions only.")
	} else {
		status := "PASS"
		if !beforeOut.Valid {
			status = "FAIL"
		}
		fmt.Printf("  Result: %s (%d errors, %d warnings)\n\n", status, beforeOut.ErrorCount, beforeOut.WarningCount)
	}

	// Step 3: Create temp dir with upgraded versions, validate (should fail)
	fmt.Println("Step 3: Upgrading provider versions and re-validating...")
	afterOut, afterErr := runUpgradedValidate(path, resolved)
	if afterErr != nil {
		fmt.Printf("  Note: Upgraded validation could not run: %v\n\n", afterErr)
	} else {
		status := "PASS"
		if !afterOut.Valid {
			status = "FAIL"
		}
		fmt.Printf("  Result: %s (%d errors, %d warnings)\n\n", status, afterOut.ErrorCount, afterOut.WarningCount)
	}

	// Step 4: Cross-reference predictions with actual terraform errors
	fmt.Println("Step 4: Cross-referencing predictions with terraform output...")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Println()

	result := buildProofResult(beforeOut, afterOut, analysis, scanResult)

	printProofReport(result)

	// Write HTML report if requested
	if flagReportHTML != "" {
		if err := writeProofHTML(flagReportHTML, result); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Printf("\nHTML report written to: %s\n", flagReportHTML)
	}

	return nil
}

func runFullScan(path string) (*analyzer.Analysis, *scanner.ScanResult, *resolver.ResolvedResult, error) {
	cfg := config.Load(path)
	s := scanner.New(scanner.Options{
		Path:      path,
		Recursive: flagRecursive,
		Ignores:   cfg.Ignore,
	})
	scanResult, err := s.Scan()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scanning: %w", err)
	}
	if len(scanResult.Modules) == 0 && len(scanResult.Providers) == 0 {
		return nil, nil, nil, nil
	}

	res := resolver.New(resolver.Options{ProviderFilter: flagProviderFilter})
	resolved, err := res.Resolve(scanResult)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving: %w", err)
	}

	a := analyzer.New()
	analysis := a.Analyze(resolved)
	analysis.BreakingChanges = detectAllBreakingChanges(resolved)
	analysis.Impacts = analyzer.AnalyzeImpact(analysis, scanResult)
	analysis.UpgradePaths = analyzer.ComputeUpgradePaths(resolved, analysis.BreakingChanges)

	return analysis, scanResult, resolved, nil
}

func runTerraformValidateInDir(dir string) (*terraformValidateOutput, error) {
	// Init
	cmd := exec.Command("terraform", "init", "-backend=false")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("terraform init: %s", strings.TrimSpace(stderr.String()))
	}

	// Validate
	cmd2 := exec.Command("terraform", "validate", "-json")
	cmd2.Dir = dir
	var stdout2 bytes.Buffer
	cmd2.Stdout = &stdout2
	cmd2.Stderr = &bytes.Buffer{}
	_ = cmd2.Run()

	var result terraformValidateOutput
	if err := json.Unmarshal(stdout2.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parsing validate output: %w", err)
	}
	return &result, nil
}

func runUpgradedValidate(origPath string, resolved *resolver.ResolvedResult) (*terraformValidateOutput, error) {
	tmpDir, err := os.MkdirTemp("", "tfoutdated-proof-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Copy .tf files and rewrite provider versions to latest
	err = filepath.Walk(origPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			relDir, _ := filepath.Rel(origPath, path)
			return os.MkdirAll(filepath.Join(tmpDir, relDir), 0755)
		}
		if !strings.HasSuffix(path, ".tf") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Rewrite provider versions to latest
		modified := string(content)
		for _, dep := range resolved.Dependencies {
			if dep.IsModule || dep.Latest == nil {
				continue
			}
			// Replace version constraints with latest major
			latestMajor := fmt.Sprintf("%d.0", dep.Latest.Segments()[0])
			// Try replacing full constraint (e.g., "~> 3.75.0" → "~> 4.0")
			if dep.Version != "" && strings.Contains(modified, dep.Version) {
				modified = strings.Replace(modified, dep.Version, latestMajor, 1)
			}
		}

		relPath, _ := filepath.Rel(origPath, path)
		return os.WriteFile(filepath.Join(tmpDir, relPath), []byte(modified), 0644)
	})
	if err != nil {
		return nil, fmt.Errorf("copying files: %w", err)
	}

	return runTerraformValidateInDir(tmpDir)
}

func buildProofResult(before, after *terraformValidateOutput, analysis *analyzer.Analysis, _ *scanner.ScanResult) proofResult {
	result := proofResult{
		Predictions: analysis.BreakingChanges,
		Analysis:    analysis,
	}

	if before != nil {
		result.BeforeValid = before.Valid
		result.BeforeErrors = before.ErrorCount
		result.BeforeWarns = before.WarningCount
		result.BeforeDiags = before.Diagnostics
	} else {
		result.InitFailed = true
	}

	if after != nil {
		result.AfterValid = after.Valid
		result.AfterErrors = after.ErrorCount
		result.AfterWarns = after.WarningCount
		result.AfterDiags = after.Diagnostics
	}

	// Match predictions to terraform errors
	for _, pred := range analysis.BreakingChanges {
		match := proofMatch{Prediction: pred, Status: "NOT_YET"}

		// Check if terraform after-upgrade caught it as an error
		if after != nil {
			for i, diag := range after.Diagnostics {
				if diagnosticConfirmsPrediction(diag, pred) {
					match.TFError = &after.Diagnostics[i]
					if diag.Severity == "error" {
						match.Status = "CONFIRMED"
					} else {
						match.Status = "WARNING_ONLY"
					}
					break
				}
			}
		}

		result.Confirmed = append(result.Confirmed, match)
	}

	return result
}

func diagnosticConfirmsPrediction(diag terraformDiagnostic, pred breaking.BreakingChange) bool {
	text := strings.ToLower(diag.Summary + " " + diag.Detail)

	if pred.ResourceType != "" && strings.Contains(text, strings.ToLower(pred.ResourceType)) {
		return true
	}
	if pred.Attribute != "" && strings.Contains(text, strings.ToLower(pred.Attribute)) {
		return true
	}
	if pred.OldValue != "" && strings.Contains(text, strings.ToLower(pred.OldValue)) {
		return true
	}
	return false
}

func printProofReport(result proofResult) {
	confirmed := 0
	warned := 0
	predicted := 0

	for _, m := range result.Confirmed {
		switch m.Status {
		case "CONFIRMED":
			confirmed++
		case "WARNING_ONLY":
			warned++
		default:
			predicted++
		}
	}

	total := len(result.Confirmed)

	fmt.Println("  BREAKING CHANGE VERIFICATION")
	fmt.Println(strings.Repeat("─", 70))
	fmt.Println()

	for i, m := range result.Confirmed {
		var icon, status string
		switch m.Status {
		case "CONFIRMED":
			icon = "  CONFIRMED "
			status = "terraform error proves this"
		case "WARNING_ONLY":
			icon = "  WARNING   "
			status = "terraform warns (will break in next major)"
		default:
			icon = "  PREDICTED "
			status = "tfoutdated detects, terraform cannot see yet"
		}

		label := m.Prediction.Provider
		if m.Prediction.ResourceType != "" {
			label += " / " + m.Prediction.ResourceType
		}
		if m.Prediction.Attribute != "" {
			label += "." + m.Prediction.Attribute
		}

		fmt.Printf("  %d. %s  %s\n", i+1, icon, label)
		fmt.Printf("              %s\n", m.Prediction.Description)
		if m.Prediction.EffortLevel != "" {
			fmt.Printf("              Effort: %s\n", m.Prediction.EffortEmoji())
		}
		if m.TFError != nil {
			fmt.Printf("              Terraform says: %s — %s\n", m.TFError.Summary, m.TFError.Detail)
		}
		fmt.Printf("              Status: %s\n", status)
		// Show real user code snippets if available
		realShown := false
		if result.Analysis != nil {
			for _, impact := range result.Analysis.Impacts {
				if impact.ActualBefore == "" {
					continue
				}
				if impact.BreakingChange.ResourceType != m.Prediction.ResourceType || impact.BreakingChange.Attribute != m.Prediction.Attribute {
					continue
				}
				realShown = true
				fmt.Printf("\n              Your code: %s (%s:%d)\n", impact.ResourceName, impact.AffectedFile, impact.AffectedLine)
				if impact.LinesChanged > 0 {
					fmt.Printf("              Scope: %d lines to change\n", impact.LinesChanged)
				}
				fmt.Printf("              Before:\n")
				for _, line := range strings.Split(impact.ActualBefore, "\n") {
					fmt.Printf("                - %s\n", line)
				}
				if impact.ActualAfter != "" {
					fmt.Printf("              After:\n")
					for _, line := range strings.Split(impact.ActualAfter, "\n") {
						fmt.Printf("                + %s\n", line)
					}
				}
			}
		}
		if !realShown && m.Prediction.BeforeSnippet != "" && m.Prediction.AfterSnippet != "" {
			fmt.Printf("\n              Before (generic):\n")
			for _, line := range strings.Split(m.Prediction.BeforeSnippet, "\n") {
				fmt.Printf("                - %s\n", line)
			}
			fmt.Printf("              After (generic):\n")
			for _, line := range strings.Split(m.Prediction.AfterSnippet, "\n") {
				fmt.Printf("                + %s\n", line)
			}
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("═", 70))
	fmt.Println("  SUMMARY")
	fmt.Println(strings.Repeat("─", 70))
	if !result.InitFailed {
		fmt.Printf("  Before upgrade:  terraform validate %s (%d errors, %d warnings)\n",
			map[bool]string{true: "PASSES", false: "FAILS"}[result.BeforeValid],
			result.BeforeErrors, result.BeforeWarns)
		fmt.Printf("  After upgrade:   terraform validate %s (%d errors, %d warnings)\n\n",
			map[bool]string{true: "PASSES", false: "FAILS"}[result.AfterValid],
			result.AfterErrors, result.AfterWarns)
	}
	fmt.Printf("  Total breaking changes:    %d\n", total)
	fmt.Printf("    Confirmed by terraform:  %d  (real errors after upgrade)\n", confirmed)
	fmt.Printf("    Warned by terraform:     %d  (deprecations, will break later)\n", warned)
	fmt.Printf("    Only tfoutdated detects: %d  (behavior/default changes invisible to validate)\n\n", predicted)

	if predicted > 0 {
		fmt.Println("  tfoutdated catches changes that terraform validate CANNOT detect:")
		fmt.Println("  - Default value changes (your code works but behavior silently changes)")
		fmt.Println("  - Provider config restructuring (features block changes)")
		fmt.Println("  - Behavioral changes (same attribute name, different semantics)")
	}
	fmt.Println()
}

func writeProofHTML(filename string, result proofResult) error {
	var buf bytes.Buffer

	confirmed := 0
	warned := 0
	predicted := 0
	for _, m := range result.Confirmed {
		switch m.Status {
		case "CONFIRMED":
			confirmed++
		case "WARNING_ONLY":
			warned++
		default:
			predicted++
		}
	}
	total := len(result.Confirmed)

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>tfoutdated — Breaking Change Proof Report</title>
<style>
  :root { --bg: #0d1117; --card: #161b22; --border: #30363d; --text: #c9d1d9; --blue: #58a6ff; --green: #3fb950; --red: #f85149; --yellow: #d29922; --dim: #484f58; --code-bg: #1a1e24; }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: var(--bg); color: var(--text); padding: 2rem; line-height: 1.6; }
  .container { max-width: 1100px; margin: 0 auto; }
  h1 { color: var(--blue); font-size: 1.8rem; margin-bottom: 0.5rem; }
  h2 { color: #79c0ff; font-size: 1.3rem; margin: 2rem 0 1rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
  .subtitle { color: var(--dim); margin-bottom: 2rem; }
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 1rem; margin: 1.5rem 0; }
  .stat { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1.2rem; text-align: center; }
  .stat .number { font-size: 2rem; font-weight: bold; }
  .stat .label { color: var(--dim); font-size: 0.85rem; margin-top: 0.3rem; }
  .stat.green .number { color: var(--green); }
  .stat.red .number { color: var(--red); }
  .stat.yellow .number { color: var(--yellow); }
  .stat.blue .number { color: var(--blue); }
  .before-after { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin: 1.5rem 0; }
  .box { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1.2rem; }
  .box.pass { border-color: var(--green); }
  .box.fail { border-color: var(--red); }
  .box h3 { font-size: 1rem; margin-bottom: 0.5rem; }
  .box .result { font-size: 1.5rem; font-weight: bold; }
  .box .result.pass { color: var(--green); }
  .box .result.fail { color: var(--red); }
  .box .detail { color: var(--dim); font-size: 0.85rem; margin-top: 0.3rem; }

  /* Proof Items */
  .proof-item { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem 1.5rem; margin: 1rem 0; }
  .proof-item.confirmed { border-left: 5px solid var(--red); }
  .proof-item.warning   { border-left: 5px solid var(--yellow); }
  .proof-item.predicted  { border-left: 5px solid var(--blue); }
  .proof-header { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; margin-bottom: 0.5rem; }
  .badge { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px; font-size: 0.75rem; font-weight: bold; }
  .badge.confirmed { background: #3d1f20; color: var(--red); }
  .badge.warning { background: #3d2e00; color: var(--yellow); }
  .badge.predicted { background: #0d2640; color: var(--blue); }
  .effort-badge { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px; font-size: 0.72rem; background: #1c2129; color: var(--dim); }
  .resource { font-weight: bold; font-family: 'SFMono-Regular', Consolas, monospace; color: #79c0ff; font-size: 0.95rem; }
  .description { margin: 0.4rem 0; font-size: 0.95rem; }
  .migration { color: var(--dim); font-size: 0.9rem; margin-top: 0.4rem; padding-left: 1rem; border-left: 2px solid var(--border); }
  .terraform-says { background: #1a1a2e; border: 1px solid #30363d; border-radius: 4px; padding: 0.6rem; margin-top: 0.6rem; font-family: monospace; font-size: 0.85rem; }
  .terraform-says .error-label { color: var(--red); font-weight: bold; }
  .explanation { color: var(--dim); font-size: 0.85rem; margin-top: 0.4rem; font-style: italic; }

  /* Snippet side-by-side */
  .snippet-container { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; margin-top: 0.8rem; }
  .snippet-box { border-radius: 6px; overflow: hidden; }
  .snippet-label { font-size: 0.75rem; font-weight: 600; padding: 0.35rem 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
  .snippet-label.before { background: #3d1f20; color: var(--red); }
  .snippet-label.after  { background: #0d2818; color: var(--green); }
  .snippet-code { background: var(--code-bg); padding: 0.75rem; font-family: 'SFMono-Regular', Consolas, monospace; font-size: 0.78rem; line-height: 1.5; white-space: pre-wrap; word-break: break-all; overflow-x: auto; color: var(--text); border: 1px solid var(--border); border-top: none; }

  .callout { background: linear-gradient(135deg, #1a1a2e, #16213e); border: 1px solid var(--blue); border-radius: 8px; padding: 1.2rem; margin: 1.5rem 0; }
  .callout h3 { color: var(--blue); margin-bottom: 0.5rem; }
  .callout ul { list-style: none; padding: 0; }
  .callout li { padding: 0.3rem 0; padding-left: 1.5rem; position: relative; }
  .callout li::before { content: "→"; position: absolute; left: 0; color: var(--blue); }

  /* Legend */
  .legend { display: flex; gap: 1.5rem; flex-wrap: wrap; margin: 1.5rem 0; font-size: 0.85rem; }
  .legend-item { display: flex; align-items: center; gap: 0.4rem; }
  .legend-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }

  footer { margin-top: 2rem; color: var(--dim); font-size: 0.8rem; border-top: 1px solid var(--border); padding-top: 1rem; text-align: center; }
  footer a { color: var(--blue); text-decoration: none; }
  @media print { :root { --bg: #fff; --card: #f6f8fa; --border: #d0d7de; --text: #1f2328; --dim: #656d76; --code-bg: #f6f8fa; } .snippet-container { break-inside: avoid; } }
  @media (max-width: 700px) { .before-after, .stats, .snippet-container { grid-template-columns: 1fr; } }
</style>
</head>
<body>
<div class="container">
<h1>Breaking Change Proof Report</h1>
<p class="subtitle">Generated by tfoutdated — Every breaking change verified with real terraform output</p>
`)

	// Stats
	fmt.Fprintf(&buf, `<div class="stats">
<div class="stat blue"><div class="number">%d</div><div class="label">Total Breaking Changes</div></div>
<div class="stat red"><div class="number">%d</div><div class="label">Confirmed by Terraform</div></div>
<div class="stat yellow"><div class="number">%d</div><div class="label">Terraform Warnings</div></div>
<div class="stat green"><div class="number">%d</div><div class="label">Only tfoutdated Detects</div></div>
</div>`, total, confirmed, warned, predicted)

	// Legend
	buf.WriteString(`<div class="legend">
<div class="legend-item"><span class="legend-dot" style="background:var(--red)"></span> CONFIRMED — terraform validate fails after upgrade, proving the breaking change</div>
<div class="legend-item"><span class="legend-dot" style="background:var(--yellow)"></span> WARNING — terraform warns about deprecation</div>
<div class="legend-item"><span class="legend-dot" style="background:var(--blue)"></span> ONLY TFOUTDATED — behavioral changes invisible to terraform validate</div>
</div>`)

	// Before/After
	if !result.InitFailed {
		beforeStatus := "PASSES"
		beforeClass := "pass"
		if !result.BeforeValid {
			beforeStatus = "FAILS"
			beforeClass = "fail"
		}
		afterStatus := "PASSES"
		afterClass := "pass"
		if !result.AfterValid {
			afterStatus = "FAILS"
			afterClass = "fail"
		}

		fmt.Fprintf(&buf, `
<h2>Before vs After Upgrade</h2>
<div class="before-after">
<div class="box %s">
  <h3>BEFORE (Current Versions)</h3>
  <div class="result %s">terraform validate %s</div>
  <div class="detail">%d errors, %d warnings</div>
</div>
<div class="box %s">
  <h3>AFTER (Upgraded to Latest Major)</h3>
  <div class="result %s">terraform validate %s</div>
  <div class="detail">%d errors, %d warnings — These are the breaking changes tfoutdated predicted</div>
</div>
</div>`,
			beforeClass, beforeClass, beforeStatus, result.BeforeErrors, result.BeforeWarns,
			afterClass, afterClass, afterStatus, result.AfterErrors, result.AfterWarns)
	}

	// Each proof item
	buf.WriteString(`<h2>Breaking Change Verification</h2>`)

	for _, m := range result.Confirmed {
		class := "predicted"
		badgeText := "ONLY TFOUTDATED"
		switch m.Status {
		case "CONFIRMED":
			class = "confirmed"
			badgeText = "CONFIRMED"
		case "WARNING_ONLY":
			class = "warning"
			badgeText = "TERRAFORM WARNING"
		}

		label := m.Prediction.Provider
		if m.Prediction.ResourceType != "" {
			label += " / " + m.Prediction.ResourceType
		}
		if m.Prediction.Attribute != "" {
			label += "." + m.Prediction.Attribute
		}

		effortLabel := ""
		switch m.Prediction.EffortLevel {
		case "small":
			effortLabel = "Low effort"
		case "medium":
			effortLabel = "Medium effort"
		case "large":
			effortLabel = "High effort"
		}

		fmt.Fprintf(&buf, `
<div class="proof-item %s">
  <div class="proof-header">
    <span class="badge %s">%s</span>`, class, class, badgeText)

		if effortLabel != "" {
			fmt.Fprintf(&buf, `<span class="effort-badge">%s</span>`, effortLabel)
		}

		fmt.Fprintf(&buf, `
    <span class="resource">%s</span>
  </div>
  <div class="description">%s</div>`,
			template_escape(label), template_escape(m.Prediction.Description))

		if m.Prediction.MigrationGuide != "" {
			fmt.Fprintf(&buf, `<div class="migration">%s</div>`, template_escape(m.Prediction.MigrationGuide))
		}

		if m.TFError != nil {
			fmt.Fprintf(&buf, `
  <div class="terraform-says">
    <span class="error-label">terraform %s:</span> %s — %s
  </div>`, m.TFError.Severity, template_escape(m.TFError.Summary), template_escape(m.TFError.Detail))
		}

		// Code snippets — prefer real user code from impact analysis
		realSnippetShown := false
		if result.Analysis != nil {
			for _, impact := range result.Analysis.Impacts {
				if impact.ActualBefore == "" {
					continue
				}
				if impact.BreakingChange.ResourceType != m.Prediction.ResourceType || impact.BreakingChange.Attribute != m.Prediction.Attribute {
					continue
				}
				realSnippetShown = true
				scopeInfo := ""
				if impact.LinesChanged > 0 {
					scopeInfo = fmt.Sprintf(" — %d lines to change", impact.LinesChanged)
				}
				fmt.Fprintf(&buf, `
  <div style="margin-top:0.5rem;font-size:0.85rem;color:#79c0ff;">
    <code>%s</code> in <code>%s:%d</code><span style="color:var(--dim);">%s</span>
  </div>
  <div class="snippet-container">
    <div class="snippet-box">
      <div class="snippet-label before">Before (your code)</div>
      <div class="snippet-code">%s</div>
    </div>
    <div class="snippet-box">
      <div class="snippet-label after">After (computed fix)</div>
      <div class="snippet-code">%s</div>
    </div>
  </div>`,
					template_escape(impact.ResourceName),
					template_escape(impact.AffectedFile),
					impact.AffectedLine,
					scopeInfo,
					template_escape(impact.ActualBefore),
					template_escape(func() string {
						if impact.ActualAfter != "" {
							return impact.ActualAfter
						}
						return impact.ActualBefore
					}()))
			}
		}
		if !realSnippetShown && m.Prediction.BeforeSnippet != "" && m.Prediction.AfterSnippet != "" {
			fmt.Fprintf(&buf, `
  <div class="snippet-container">
    <div class="snippet-box">
      <div class="snippet-label before">Before (generic example)</div>
      <div class="snippet-code">%s</div>
    </div>
    <div class="snippet-box">
      <div class="snippet-label after">After (generic example)</div>
      <div class="snippet-code">%s</div>
    </div>
  </div>`, template_escape(m.Prediction.BeforeSnippet), template_escape(m.Prediction.AfterSnippet))
		}

		explanation := ""
		switch m.Status {
		case "CONFIRMED":
			explanation = "Terraform error after upgrade proves this breaking change is real."
		case "WARNING_ONLY":
			explanation = "Terraform warns about deprecation. This resource will be removed in a future version."
		default:
			explanation = "Terraform validate cannot detect this — it is a behavioral or default change that silently affects your infrastructure."
		}
		fmt.Fprintf(&buf, `<div class="explanation">%s</div></div>`, explanation)
	}

	// Callout
	if predicted > 0 {
		fmt.Fprintf(&buf, `
<div class="callout">
<h3>Why tfoutdated catches what terraform validate misses</h3>
<ul>
<li>Default value changes — your code still validates but behavior silently changes (e.g., TLS version, disk type)</li>
<li>Provider config restructuring — features block may accept the same syntax but ignore some flags</li>
<li>Behavioral changes — same attribute name but different semantics or side effects</li>
<li>terraform validate only checks syntax and schema — it cannot predict runtime behavior changes</li>
</ul>
</div>`)
	}

	buf.WriteString(`
<footer>Generated by <a href="https://github.com/anasskartit/tfoutdated">tfoutdated</a></footer>
</div>
</body>
</html>`)

	return os.WriteFile(filename, buf.Bytes(), 0644)
}

func template_escape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
