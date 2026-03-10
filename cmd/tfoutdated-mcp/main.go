package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/anasskartit/tfoutdated/internal/output"
	"github.com/anasskartit/tfoutdated/internal/recommend"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/schemadiff"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

var version = "dev"

func main() {
	s := server.NewMCPServer(
		"tfoutdated",
		version,
		server.WithToolCapabilities(false),
	)

	// Tool 1: scan (read-only)
	scanTool := mcp.NewTool("tfoutdated_scan",
		mcp.WithDescription("Scan Terraform dependencies for outdated modules and providers. Returns structured report with current vs latest versions, breaking changes, and upgrade paths."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the Terraform root directory to scan"),
		),
	)
	s.AddTool(scanTool, handleScan)

	// Tool 2: recommend (read-only)
	recommendTool := mcp.NewTool("tfoutdated_recommend",
		mcp.WithDescription("Generate actionable governance recommendations for Terraform dependencies. Detects version fragmentation, unpinned versions, major version drift, deprecated modules, and stale dependencies."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the Terraform root directory to scan"),
		),
	)
	s.AddTool(recommendTool, handleRecommend)

	// Tool 3: impact (read-only)
	impactTool := mcp.NewTool("tfoutdated_impact",
		mcp.WithDescription("Analyze the impact of upgrading a Terraform provider. Checks each module's provider_dependencies against the target version to determine compatibility."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the Terraform root directory to scan"),
		),
		mcp.WithString("provider",
			mcp.Required(),
			mcp.Description("Provider to upgrade (e.g., hashicorp/azurerm)"),
		),
		mcp.WithString("target_version",
			mcp.Required(),
			mcp.Description("Target provider version (e.g., 4.61.0)"),
		),
	)
	s.AddTool(impactTool, handleImpact)

	// Tool 4: full report (read-only)
	fullReportTool := mcp.NewTool("tfoutdated_full_report",
		mcp.WithDescription("Generate a comprehensive Terraform dependency report including: dependency scan, breaking changes, governance recommendations, and provider upgrade impact analysis."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the Terraform root directory to scan"),
		),
		mcp.WithString("provider",
			mcp.Description("Provider for impact analysis (default: hashicorp/azurerm)"),
		),
		mcp.WithString("target_version",
			mcp.Description("Target provider version for impact analysis"),
		),
	)
	s.AddTool(fullReportTool, handleFullReport)

	// Tool 5: HTML report (writes a file)
	htmlReportTool := mcp.NewTool("tfoutdated_html_report",
		mcp.WithDescription("Generate a self-contained HTML report file with scan results, breaking changes, recommendations, and impact analysis."),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the Terraform root directory to scan"),
		),
		mcp.WithString("output",
			mcp.Required(),
			mcp.Description("Output file path for the HTML report"),
		),
		mcp.WithString("provider",
			mcp.Description("Provider for impact analysis (default: hashicorp/azurerm)"),
		),
		mcp.WithString("target_version",
			mcp.Description("Target provider version for impact analysis"),
		),
	)
	s.AddTool(htmlReportTool, handleHTMLReport)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "tfoutdated-mcp server error: %v\n", err)
		os.Exit(1)
	}
}

func runPipeline(path string) (*analyzer.Analysis, *scanner.ScanResult, *resolver.Resolver, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving path: %w", err)
	}

	cfg := config.Load(absPath)

	s := scanner.New(scanner.Options{
		Path:      absPath,
		Recursive: true,
		Ignores:   cfg.Ignore,
	})
	result, err := s.Scan()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scanning: %w", err)
	}

	if len(result.Modules) == 0 && len(result.Providers) == 0 {
		return nil, nil, nil, fmt.Errorf("no Terraform dependencies found in %s", absPath)
	}

	res := resolver.New(resolver.Options{})
	resolved, err := res.Resolve(result)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving versions: %w", err)
	}

	a := analyzer.New()
	analysis := a.Analyze(resolved)

	analysis.BreakingChanges = detectAllBreakingChanges(resolved)

	impacts := analyzer.AnalyzeImpact(analysis, result)
	analysis.Impacts = impacts

	analysis.UpgradePaths = analyzer.ComputeUpgradePaths(resolved, analysis.BreakingChanges)

	return analysis, result, res, nil
}

func handleScan(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requiredString(request, "path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	analysis, _, _, err := runPipeline(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("scan error: %v", err)), nil
	}

	var buf bytes.Buffer
	renderer := &output.JSONRenderer{}
	if err := renderer.Render(&buf, analysis); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("render error: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func handleRecommend(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requiredString(request, "path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	analysis, _, _, err := runPipeline(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("scan error: %v", err)), nil
	}

	recs := recommend.Recommend(analysis)

	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("json error: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func handleImpact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requiredString(request, "path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	provider, err := requiredString(request, "provider")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetVersion, err := requiredString(request, "target_version")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	_, result, res, err := runPipeline(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("scan error: %v", err)), nil
	}

	impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, targetVersion)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("impact error: %v", err)), nil
	}

	data, err := json.MarshalIndent(impact, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("json error: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func handleFullReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requiredString(request, "path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	provider := optionalString(request, "provider", "hashicorp/azurerm")
	targetVersion := optionalString(request, "target_version", "")

	analysis, result, res, err := runPipeline(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("scan error: %v", err)), nil
	}

	// Recommendations
	recs := recommend.Recommend(analysis)
	analysis.Recommendations = recs

	// Impact analysis
	if provider != "" {
		tv := targetVersion
		if tv == "" {
			tv = findLatestProvider(analysis, provider)
		}
		if tv != "" {
			impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, tv)
			if err == nil {
				analysis.ProviderImpact = impact
			}
		}
	}

	var buf bytes.Buffer
	renderer := &output.MarkdownRenderer{}
	if err := renderer.Render(&buf, analysis); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("render error: %v", err)), nil
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func handleHTMLReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := requiredString(request, "path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	outputPath, err := requiredString(request, "output")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	provider := optionalString(request, "provider", "hashicorp/azurerm")
	targetVersion := optionalString(request, "target_version", "")

	analysis, result, res, err := runPipeline(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("scan error: %v", err)), nil
	}

	recs := recommend.Recommend(analysis)
	analysis.Recommendations = recs

	if provider != "" {
		tv := targetVersion
		if tv == "" {
			tv = findLatestProvider(analysis, provider)
		}
		if tv != "" {
			impact, err := analyzer.AnalyzeProviderImpact(result, res.GetRegistry(), provider, tv)
			if err == nil {
				analysis.ProviderImpact = impact
			}
		}
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create file error: %v", err)), nil
	}
	defer f.Close()

	renderer := &output.HTMLRenderer{}
	if err := renderer.Render(f, analysis); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("render error: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("HTML report written to %s", outputPath)), nil
}

func findLatestProvider(analysis *analyzer.Analysis, provider string) string {
	for _, dep := range analysis.Dependencies {
		if !dep.IsModule && strings.EqualFold(dep.Source, provider) {
			return dep.LatestVer
		}
	}
	return ""
}

func requiredString(request mcp.CallToolRequest, key string) (string, error) {
	val, ok := request.GetArguments()[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	s, ok := val.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("parameter %s must be a non-empty string", key)
	}
	return s, nil
}

func optionalString(request mcp.CallToolRequest, key, defaultVal string) string {
	val, ok := request.GetArguments()[key]
	if !ok {
		return defaultVal
	}
	s, ok := val.(string)
	if !ok || s == "" {
		return defaultVal
	}
	return s
}

func detectAllBreakingChanges(resolved *resolver.ResolvedResult) []breaking.BreakingChange {
	bd := breaking.NewDetector()
	breakingChanges := bd.Detect(resolved)

	var inputs []schemadiff.DetectInput
	for _, dep := range resolved.Dependencies {
		if dep.IsModule {
			inputs = append(inputs, schemadiff.DetectInput{
				Source:         dep.Source,
				CurrentVersion: dep.Current.String(),
				LatestVersion:  dep.Latest.String(),
			})
		}
	}
	if len(inputs) == 0 {
		return breakingChanges
	}

	sd := schemadiff.NewDetector()
	diffResults := sd.Detect(inputs)
	for _, r := range diffResults {
		if r.Error != nil {
			continue
		}
		latestVer := ""
		for _, dep := range resolved.Dependencies {
			if dep.IsModule && dep.Source == r.Source {
				latestVer = dep.Latest.String()
				break
			}
		}
		dynamicChanges := schemadiff.ToBreakingChanges([]schemadiff.DetectResult{r}, latestVer)
		for _, dc := range dynamicChanges {
			isDuplicate := false
			for _, existing := range breakingChanges {
				if existing.Provider == dc.Provider && existing.Attribute == dc.Attribute && existing.Kind == dc.Kind {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				breakingChanges = append(breakingChanges, dc)
			}
		}
	}

	return breakingChanges
}
