package output

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// HTMLRenderer renders analysis results as standalone HTML.
type HTMLRenderer struct{}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Terraform Dependency Report — tfoutdated</title>
<style>
  *, *::before, *::after { box-sizing: border-box; }
  :root {
    --bg-primary:   #0d1117;
    --bg-secondary: #161b22;
    --bg-tertiary:  #1c2129;
    --border:       #30363d;
    --text-primary: #c9d1d9;
    --text-muted:   #8b949e;
    --accent-blue:  #58a6ff;
    --accent-blue2: #79c0ff;
    --green:        #3fb950;
    --green-bg:     #0d2818;
    --yellow:       #d29922;
    --yellow-bg:    #3d2e00;
    --red:          #f85149;
    --red-bg:       #3d1f20;
    --blue-badge:   #1f6feb;
    --purple:       #bc8cff;
    --code-bg:      #1a1e24;
  }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
    margin: 0; padding: 0;
    background: var(--bg-primary);
    color: var(--text-primary);
    line-height: 1.6;
  }
  .container { max-width: 1280px; margin: 0 auto; padding: 2rem 2.5rem; }

  /* Header */
  .report-header {
    display: flex; align-items: center; justify-content: space-between;
    border-bottom: 2px solid var(--accent-blue); padding-bottom: 1rem; margin-bottom: 2rem;
  }
  .report-header h1 { margin: 0; font-size: 1.75rem; color: var(--accent-blue); font-weight: 600; }
  .report-header .brand { font-size: 0.85rem; color: var(--text-muted); }
  .report-header .brand a { color: var(--accent-blue); text-decoration: none; }

  /* Stat Cards */
  .exec-summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
  .stat-card { background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 1.25rem 1.5rem; text-align: center; }
  .stat-card .stat-value { font-size: 2.25rem; font-weight: 700; line-height: 1.1; }
  .stat-card .stat-label { font-size: 0.85rem; color: var(--text-muted); margin-top: 0.35rem; }
  .stat-card.total   .stat-value { color: var(--accent-blue); }
  .stat-card.major-c .stat-value { color: var(--red); }
  .stat-card.minor-c .stat-value { color: var(--yellow); }
  .stat-card.patch-c .stat-value { color: var(--green); }
  .stat-card.break-c .stat-value { color: var(--red); }
  .stat-card.miss-c  .stat-value { color: var(--purple); }

  .callout-miss {
    background: linear-gradient(135deg, #1a1040 0%%, #261447 100%%);
    border: 1px solid var(--purple); border-radius: 8px; padding: 1rem 1.5rem;
    margin-bottom: 2rem; font-size: 0.95rem; color: #d2b8ff;
  }
  .callout-miss strong { color: #e8d5ff; }

  /* Section Headings */
  .section-title {
    color: var(--accent-blue2); font-size: 1.35rem; font-weight: 600;
    margin: 2.5rem 0 1rem; padding-bottom: 0.4rem; border-bottom: 1px solid var(--border);
  }
  .section-title span { font-size: 0.8rem; color: var(--text-muted); font-weight: 400; margin-left: 0.5rem; }

  /* Tables */
  table { border-collapse: collapse; width: 100%%; margin: 0 0 1.5rem; font-size: 0.9rem; }
  thead th { background: var(--bg-secondary); color: var(--accent-blue); padding: 0.65rem 0.85rem; text-align: left; border: 1px solid var(--border); font-weight: 600; position: sticky; top: 0; z-index: 1; }
  tbody td { padding: 0.6rem 0.85rem; border: 1px solid var(--border); vertical-align: top; }
  tbody tr:nth-child(even) { background: var(--bg-secondary); }
  tbody tr:hover { background: var(--bg-tertiary); }
  .major { color: var(--red); font-weight: 700; }
  .minor { color: var(--yellow); font-weight: 700; }
  .patch { color: var(--green); font-weight: 700; }
  .dep-source { font-family: 'SFMono-Regular', Consolas, monospace; font-size: 0.85rem; }
  .dep-file   { color: var(--text-muted); font-size: 0.82rem; font-family: monospace; }

  /* Breaking Change Cards — new design */
  .bc-card {
    border-radius: 8px; padding: 1.25rem 1.5rem; margin: 1rem 0;
    background: var(--bg-secondary); border: 1px solid var(--border);
  }
  .bc-card.breaking { border-left: 5px solid var(--red); }
  .bc-card.warning  { border-left: 5px solid var(--yellow); }
  .bc-card.info     { border-left: 5px solid var(--accent-blue); }
  .bc-header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; }
  .bc-badge { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px; font-size: 0.75rem; font-weight: 700; text-transform: uppercase; }
  .bc-badge.breaking { background: var(--red-bg); color: var(--red); }
  .bc-badge.warning  { background: var(--yellow-bg); color: var(--yellow); }
  .bc-badge.info     { background: #0d2640; color: var(--accent-blue); }
  .bc-effort { display: inline-block; padding: 0.15rem 0.6rem; border-radius: 4px; font-size: 0.72rem; font-weight: 500; background: var(--bg-tertiary); color: var(--text-muted); }
  .bc-resource { font-family: monospace; font-weight: 600; color: var(--accent-blue2); font-size: 0.95rem; }
  .bc-desc { margin: 0.4rem 0; font-size: 0.95rem; }
  .bc-migration { color: var(--text-muted); font-size: 0.9rem; margin-top: 0.4rem; padding-left: 1rem; border-left: 2px solid var(--border); }

  /* Code Snippets */
  .snippet-container { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; margin-top: 0.8rem; }
  .snippet-box { border-radius: 6px; overflow: hidden; }
  .snippet-label { font-size: 0.75rem; font-weight: 600; padding: 0.35rem 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
  .snippet-label.before { background: var(--red-bg); color: var(--red); }
  .snippet-label.after  { background: var(--green-bg); color: var(--green); }
  .snippet-code { background: var(--code-bg); padding: 0.75rem; font-family: 'SFMono-Regular', Consolas, monospace; font-size: 0.8rem; line-height: 1.5; white-space: pre-wrap; word-break: break-all; overflow-x: auto; color: var(--text-primary); border: 1px solid var(--border); border-top: none; }

  /* Upgrade Paths */
  .upgrade-block { margin-bottom: 1.5rem; }
  .upgrade-block h3 { color: var(--text-primary); font-size: 1rem; margin: 0 0 0.5rem; font-family: monospace; }
  .step { margin: 0.25rem 0; padding-left: 1.25rem; font-size: 0.9rem; }
  .safe   { color: var(--green); }
  .unsafe { color: var(--red); }
  .safe-target { color: var(--green); font-size: 0.9rem; margin-top: 0.35rem; }

  /* Tool Comparison Matrix */
  .comparison-table th:first-child { min-width: 240px; }
  .comparison-table th { text-align: center; white-space: nowrap; }
  .comparison-table th:first-child { text-align: left; }
  .comparison-table td { text-align: center; font-size: 0.88rem; }
  .comparison-table td:first-child { text-align: left; font-weight: 500; color: var(--text-primary); }
  .comparison-table th.tfoutdated-col { background: linear-gradient(180deg, #0e2a47 0%%, var(--bg-secondary) 100%%); color: #79c0ff; border-bottom: 2px solid var(--accent-blue); }
  .comparison-table td.tfoutdated-col { background: rgba(88,166,255,0.04); }
  .c-full    { color: var(--green);  }
  .c-partial { color: var(--yellow); }
  .c-none    { color: var(--red);    }
  .c-planned { color: var(--blue-badge); }

  /* Why Section */
  .why-section { background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 1.5rem 2rem; margin-top: 2rem; }
  .why-section h2 { color: var(--accent-blue); font-size: 1.2rem; margin: 0 0 1rem; }
  .why-section ul { margin: 0; padding-left: 1.25rem; }
  .why-section li { margin-bottom: 0.6rem; line-height: 1.5; }
  .why-section li strong { color: var(--accent-blue2); }

  /* Quick Legend */
  .legend { display: flex; gap: 1.5rem; flex-wrap: wrap; margin: 1rem 0 0.5rem; font-size: 0.85rem; }
  .legend-item { display: flex; align-items: center; gap: 0.4rem; }
  .legend-dot { width: 10px; height: 10px; border-radius: 50%%; display: inline-block; }
  .legend-dot.red { background: var(--red); }
  .legend-dot.yellow { background: var(--yellow); }
  .legend-dot.green { background: var(--green); }

  /* Footer */
  footer { margin-top: 3rem; color: var(--text-muted); font-size: 0.82rem; border-top: 1px solid var(--border); padding-top: 1rem; text-align: center; }
  footer a { color: var(--accent-blue); text-decoration: none; }

  /* Print */
  @media print {
    :root { --bg-primary:#fff; --bg-secondary:#f6f8fa; --bg-tertiary:#eef1f5; --border:#d0d7de; --text-primary:#1f2328; --text-muted:#656d76; --accent-blue:#0969da; --accent-blue2:#0550ae; --green:#1a7f37; --green-bg:#dafbe1; --yellow:#9a6700; --yellow-bg:#fff8c5; --red:#cf222e; --red-bg:#ffebe9; --blue-badge:#0969da; --purple:#8250df; --code-bg:#f6f8fa; }
    body { background: #fff; }
    .container { padding: 1rem; }
    thead th { position: static; }
    .snippet-container { break-inside: avoid; }
  }

  /* Responsive */
  @media (max-width: 900px) { .container { padding: 1rem; } .exec-summary { grid-template-columns: repeat(2, 1fr); } table { font-size: 0.82rem; } thead th, tbody td { padding: 0.45rem 0.55rem; } .snippet-container { grid-template-columns: 1fr; } }
  @media (max-width: 600px) { .exec-summary { grid-template-columns: 1fr; } .report-header { flex-direction: column; align-items: flex-start; gap: 0.5rem; } table { display: block; overflow-x: auto; } }
</style>
</head>
<body>
<div class="container">

<div class="report-header">
  <h1>Terraform Dependency Report</h1>
  <div class="brand">Generated by <a href="https://github.com/anasskartit/tfoutdated">tfoutdated</a></div>
</div>

<!-- Executive Summary -->
<div class="exec-summary">
  <div class="stat-card total"><div class="stat-value">{{.TotalDeps}}</div><div class="stat-label">Dependencies Scanned</div></div>
  <div class="stat-card major-c"><div class="stat-value">{{.MajorCount}}</div><div class="stat-label">Major Updates</div></div>
  <div class="stat-card minor-c"><div class="stat-value">{{.MinorCount}}</div><div class="stat-label">Minor Updates</div></div>
  <div class="stat-card patch-c"><div class="stat-value">{{.PatchCount}}</div><div class="stat-label">Patch Updates</div></div>
  <div class="stat-card break-c"><div class="stat-value">{{.BreakingCount}}</div><div class="stat-label">Breaking Changes</div></div>
  <div class="stat-card miss-c"><div class="stat-value">{{.BreakingCount}}</div><div class="stat-label">Missed by Renovate / Dependabot</div></div>
</div>

{{if gt .BreakingCount 0}}
<div class="callout-miss">
  <strong>Renovate and Dependabot would miss {{.BreakingCount}} breaking change{{if gt .BreakingCount 1}}s{{end}}.</strong>
  Those tools bump version numbers but have no knowledge of removed resources, renamed attributes, or behavioral changes.
  <strong>tfoutdated</strong> detects these issues <em>before</em> they reach your CI pipeline.
</div>
{{end}}

<p style="color:var(--accent-blue);font-size:1.05rem;margin:0 0 0.5rem;">{{.Summary}}</p>

<div class="legend">
  <div class="legend-item"><span class="legend-dot red"></span> Major — likely has breaking changes</div>
  <div class="legend-item"><span class="legend-dot yellow"></span> Minor — new features, usually safe</div>
  <div class="legend-item"><span class="legend-dot green"></span> Patch — bug fixes only</div>
</div>

<!-- Outdated Dependencies -->
<h2 class="section-title">Outdated Dependencies <span>({{len .Dependencies}} items)</span></h2>
{{if .Dependencies}}
<table>
<thead><tr><th>Dependency</th><th>Current</th><th>Latest</th><th>Update Type</th><th>Impact</th><th>File</th></tr></thead>
<tbody>
{{range .Dependencies}}
<tr>
  <td class="dep-source">{{.Source}}</td>
  <td>{{.CurrentVer}}</td>
  <td>{{.LatestVer}}</td>
  <td class="{{.UpdateClass}}">{{.UpdateTypeStr}}</td>
  <td>{{.ImpactStr}}</td>
  <td class="dep-file">{{.FilePath}}:{{.Line}}</td>
</tr>
{{end}}
</tbody>
</table>
{{else}}
<p style="color:var(--green);">All dependencies are up to date.</p>
{{end}}

<!-- Breaking Changes with Snippets -->
{{if .BreakingChanges}}
<h2 class="section-title">Breaking Changes <span>({{len .BreakingChanges}} detected — scroll down to see exactly what to change)</span></h2>

{{range .BreakingChanges}}
<div class="bc-card {{.CSSClass}}">
  <div class="bc-header">
    <span class="bc-badge {{.CSSClass}}">{{.SeverityStr}}</span>
    {{if .EffortLevel}}<span class="bc-effort">{{.EffortLevel}}</span>{{end}}
    {{if .ResourceType}}<span class="bc-resource">{{.ResourceType}}</span>{{end}}
    {{if .Attribute}}<span style="color:var(--text-muted);font-family:monospace;">.{{.Attribute}}</span>{{end}}
  </div>
  <div class="bc-desc">{{.Description}}</div>
  {{if .MigrationGuide}}<div class="bc-migration">{{.MigrationGuide}}</div>{{end}}

  {{if .RealSnippets}}
  {{range .RealSnippets}}
  <div style="margin-top:0.5rem;font-size:0.85rem;color:var(--accent-blue2);">
    <code>{{.ResourceName}}</code> in <code>{{.AffectedFile}}:{{.AffectedLine}}</code>
    {{if gt .LinesChanged 0}}<span style="color:var(--text-muted);"> — {{.LinesChanged}} lines to change</span>{{end}}
  </div>
  <div class="snippet-container">
    <div class="snippet-box">
      <div class="snippet-label before">Before (your code)</div>
      <div class="snippet-code">{{.Before}}</div>
    </div>
    <div class="snippet-box">
      <div class="snippet-label after">After (computed fix)</div>
      <div class="snippet-code">{{.After}}</div>
    </div>
  </div>
  {{end}}
  {{else if and .BeforeSnippet .AfterSnippet}}
  <div class="snippet-container">
    <div class="snippet-box">
      <div class="snippet-label before">Before (generic example)</div>
      <div class="snippet-code">{{.BeforeSnippet}}</div>
    </div>
    <div class="snippet-box">
      <div class="snippet-label after">After (generic example)</div>
      <div class="snippet-code">{{.AfterSnippet}}</div>
    </div>
  </div>
  {{end}}
</div>
{{end}}
{{end}}

<!-- Recommended Upgrade Paths -->
{{if .UpgradePaths}}
<h2 class="section-title">Recommended Upgrade Paths <span>({{len .UpgradePaths}} dependencies)</span></h2>
{{range .UpgradePaths}}
<div class="upgrade-block">
<h3>{{.Name}}</h3>
{{range $i, $step := .Steps}}
<div class="step">
  {{inc $i}}. <span class="{{if $step.Safe}}safe{{else}}unsafe{{end}}">{{$step.From}} &rarr; {{$step.To}}</span>
  {{if not $step.Safe}} &#9888; potential breaking changes{{end}}
</div>
{{end}}
{{if .NonBreakingTarget}}<p class="safe-target">&#10003; Safe target: {{.NonBreakingTarget}}</p>{{else if .HasBreakingSteps}}<p class="unsafe" style="font-size:0.9rem;margin-top:0.35rem;">&#9888; Breaking changes — review required before upgrading</p>{{end}}
</div>
{{end}}
{{end}}

<!-- Recommendations -->
{{if .Recommendations}}
<h2 class="section-title">Recommendations <span>({{len .Recommendations}})</span></h2>
{{range .Recommendations}}
<div class="bc-card {{.SevClass}}" style="border-left-width:4px;">
  <div class="bc-header">
    <span class="bc-badge {{.SevClass}}">{{.Severity}}</span>
    <span style="font-weight:600;">{{.Title}}</span>
  </div>
  <div class="bc-desc" style="color:var(--text-muted);font-size:0.9rem;">
    {{range .Details}}<div>{{.}}</div>{{end}}
  </div>
  {{if .Fix}}
  <div style="margin-top:0.8rem;">
    <strong style="font-size:0.85rem;color:var(--accent-blue);">Changes to make:</strong>
    {{range .Fix}}
    <div class="snippet-box" style="margin-top:0.4rem;">
      {{if .FilePart}}<div style="font-size:0.75rem;color:var(--text-muted);padding:0.3rem 0.75rem;background:var(--bg-tertiary);font-family:monospace;">{{.FilePart}}</div>{{end}}
      <div class="snippet-code" style="border-top:none;">{{if .OldValue}}<span style="color:var(--red);">- {{.OldValue}}</span>{{end}}
{{if .NewValue}}<span style="color:var(--green);">+ {{.NewValue}}</span>{{end}}</div>
    </div>
    {{end}}
  </div>
  {{end}}
</div>
{{end}}
{{end}}

<!-- Provider Impact Analysis -->
{{if .ProviderImpact}}
<h2 class="section-title">Impact Analysis <span>{{.ProviderImpact.TargetProvider}} → {{.ProviderImpact.TargetVersion}}</span></h2>

<div class="exec-summary" style="margin-bottom:1.5rem;">
  <div class="stat-card patch-c"><div class="stat-value">{{.ProviderImpact.Compatible}}</div><div class="stat-label">Compatible</div></div>
  {{if gt .ProviderImpact.NeedUpgrade 0}}<div class="stat-card minor-c"><div class="stat-value">{{.ProviderImpact.NeedUpgrade}}</div><div class="stat-label">Need Upgrade</div></div>{{end}}
  {{if gt .ProviderImpact.Incompatible 0}}<div class="stat-card major-c"><div class="stat-value">{{.ProviderImpact.Incompatible}}</div><div class="stat-label">Incompatible</div></div>{{end}}
</div>

{{range .ProviderImpact.Results}}
{{if .RequiresUpgrade}}
<div style="background:var(--bg-secondary);border-radius:8px;padding:1rem;margin-bottom:0.8rem;border:1px solid var(--border);border-left:3px solid var(--yellow);">
  <h3 style="color:var(--yellow);font-size:0.9rem;">{{.Module}}</h3>
  <div class="snippet-code" style="margin-top:0.5rem;">
<span style="color:var(--red);">- version = "{{.CurrentModuleVer}}"</span>
{{if .MinCompatibleVer}}<span style="color:var(--green);">+ version = "{{.MinCompatibleVer}}"</span>{{end}}</div>
</div>
{{end}}
{{end}}

<table>
<thead><tr><th>Module</th><th>Version</th><th>Latest</th><th>Status</th></tr></thead>
<tbody>
{{range .ProviderImpact.Results}}
<tr>
  <td class="dep-source">{{.Module}}</td>
  <td>{{.CurrentModuleVer}}</td>
  <td>{{.LatestModuleVer}}</td>
  <td><span class="bc-badge {{.StatusClass}}">{{.StatusLabel}}</span></td>
</tr>
{{end}}
</tbody>
</table>

<div style="margin-top:1.5rem;">
  <div class="callout-miss" style="{{if eq .ProviderImpact.VerdictClass "safe"}}border-color:var(--green);background:var(--green-bg);color:var(--green);{{else if eq .ProviderImpact.VerdictClass "blocked"}}border-color:var(--red);color:var(--red);{{end}}">
    <strong>{{.ProviderImpact.VerdictText}}</strong>
  </div>
</div>
{{end}}

<!-- Tool Comparison Matrix -->
<h2 class="section-title">Tool Comparison Matrix</h2>
<table class="comparison-table">
<thead>
<tr>
  <th>Feature</th>
  <th class="tfoutdated-col">tfoutdated</th>
  <th>Renovate</th>
  <th>Dependabot</th>
  <th>tfupdate</th>
  <th>tflint</th>
</tr>
</thead>
<tbody>
<tr><td>Detect outdated providers</td><td class="tfoutdated-col c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-none">&#10008; No</td></tr>
<tr><td>Detect outdated modules</td><td class="tfoutdated-col c-full">&#10004; Full</td><td class="c-partial">&#9888; Partial</td><td class="c-partial">&#9888; Partial</td><td class="c-full">&#10004; Full</td><td class="c-none">&#10008; No</td></tr>
<tr><td>Azure Verified Modules (AVM)</td><td class="tfoutdated-col c-full">&#10004; Native</td><td class="c-partial">&#9888; Generic</td><td class="c-partial">&#9888; Generic</td><td class="c-none">&#10008; No</td><td class="c-none">&#10008; No</td></tr>
<tr><td>Breaking change detection</td><td class="tfoutdated-col c-full">&#10004; Deep</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-partial">&#9888; Rules only</td></tr>
<tr><td>Code snippets (before/after)</td><td class="tfoutdated-col c-full">&#10004; Built-in</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Impact analysis</td><td class="tfoutdated-col c-full">&#10004; File-level</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Safe upgrade paths</td><td class="tfoutdated-col c-full">&#10004; Multi-step</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Version alignment check</td><td class="tfoutdated-col c-full">&#10004; Cross-repo</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Auto-fix (non-breaking)</td><td class="tfoutdated-col c-full">&#10004; HCL-aware</td><td class="c-full">&#10004; PR-based</td><td class="c-full">&#10004; PR-based</td><td class="c-full">&#10004; Rewrite</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Migration guides</td><td class="tfoutdated-col c-full">&#10004; Built-in</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td></tr>
<tr><td>Offline / local-first</td><td class="tfoutdated-col c-full">&#10004; Yes</td><td class="c-none">&#10008; SaaS</td><td class="c-none">&#10008; SaaS</td><td class="c-full">&#10004; Yes</td><td class="c-full">&#10004; Yes</td></tr>
<tr><td>CI/CD integration</td><td class="tfoutdated-col c-full">&#10004; Exit codes</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td></tr>
<tr><td>Pre-commit hook</td><td class="tfoutdated-col c-full">&#10004; Yes</td><td class="c-none">&#10008; No</td><td class="c-none">&#10008; No</td><td class="c-full">&#10004; Yes</td><td class="c-full">&#10004; Yes</td></tr>
<tr><td>AWS / GCP support</td><td class="tfoutdated-col c-planned">&#8594; Planned</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td><td class="c-full">&#10004; Full</td></tr>
<tr><td>Knowledge base (curated)</td><td class="tfoutdated-col c-full">&#10004; azurerm, azuread, azapi</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-none">&#10008; None</td><td class="c-partial">&#9888; Plugin</td></tr>
</tbody>
</table>

<!-- Why tfoutdated? -->
<div class="why-section">
  <h2>Why tfoutdated?</h2>
  <ul>
    <li><strong>Shows you exactly what to change.</strong> Each breaking change comes with side-by-side code snippets: your current code on the left, the fixed version on the right. No more guessing what "resource removed" means for your specific config.</li>
    <li><strong>Breaking-change awareness that other tools lack.</strong> Renovate and Dependabot bump version numbers but have zero insight into Terraform provider schema changes. tfoutdated ships a curated knowledge base of removed attributes, renamed resources, and behavioral changes.</li>
    <li><strong>Safe, multi-step upgrade paths.</strong> Instead of jumping straight to the latest major version, tfoutdated recommends an incremental upgrade sequence &mdash; highlighting exactly which steps carry breaking changes.</li>
    <li><strong>File-level impact analysis.</strong> Every breaking change is mapped back to the specific <code>.tf</code> files and resources it affects.</li>
    <li><strong>Offline, local-first, and CI-friendly.</strong> A single binary that returns structured exit codes, JSON, or this HTML report &mdash; perfect for pre-commit hooks and CI pipelines.</li>
  </ul>
</div>

<footer>
  Generated by <a href="https://github.com/anasskartit/tfoutdated">tfoutdated</a>
  &mdash; Terraform dependency intelligence for teams that ship infrastructure safely.
</footer>

</div>
</body>
</html>`

type htmlData struct {
	Summary         string
	Dependencies    []htmlDep
	BreakingChanges []htmlBreaking
	UpgradePaths    []analyzer.UpgradePath
	Recommendations []htmlRecommendation
	ProviderImpact  *htmlProviderImpact

	TotalDeps     int
	MajorCount    int
	MinorCount    int
	PatchCount    int
	BreakingCount int
}

type htmlRecommendation struct {
	Severity    string
	SevClass    string
	Title       string
	Details     []string
	Fix         []htmlDiffItem
}

type htmlDiffItem struct {
	FilePart string
	OldValue string
	NewValue string
}

type htmlProviderImpact struct {
	TargetProvider string
	TargetVersion  string
	Compatible     int
	NeedUpgrade    int
	Incompatible   int
	Results        []htmlProviderImpactResult
	VerdictClass   string
	VerdictText    string
}

type htmlProviderImpactResult struct {
	Module           string
	CurrentModuleVer string
	LatestModuleVer  string
	StatusClass      string
	StatusLabel      string
	RequiresUpgrade  bool
	MinCompatibleVer string
}

type htmlDep struct {
	Source        string
	CurrentVer   string
	LatestVer    string
	UpdateClass  string
	UpdateTypeStr string
	ImpactStr    string
	FilePath     string
	Line         int
}

type htmlBreaking struct {
	SeverityStr    string
	CSSClass       string
	ResourceType   string
	Attribute      string
	Description    string
	MigrationGuide string
	EffortLevel    string
	BeforeSnippet  string
	AfterSnippet   string
	RealSnippets   []htmlRealSnippet // actual user code snippets
}

type htmlRealSnippet struct {
	ResourceName string
	AffectedFile string
	AffectedLine int
	LinesChanged int
	Before       string
	After        string
}

func (r *HTMLRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	data := htmlData{
		Summary: analysis.Summary(),
	}

	data.TotalDeps = len(analysis.Dependencies)
	for _, dep := range analysis.Dependencies {
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			data.MajorCount++
		case resolver.UpdateMinor:
			data.MinorCount++
		case resolver.UpdatePatch:
			data.PatchCount++
		}
	}
	data.BreakingCount = len(analysis.BreakingChanges)

	for _, dep := range analysis.Dependencies {
		updateClass := "patch"
		switch dep.UpdateType.String() {
		case "MAJOR":
			updateClass = "major"
		case "MINOR":
			updateClass = "minor"
		}

		impact := "none"
		breakCount := 0
		for _, bc := range analysis.BreakingChanges {
			if bc.Provider == dep.Name || bc.Provider == dep.Source {
				breakCount++
			}
		}
		if breakCount > 0 {
			impact = fmt.Sprintf("%d breaking", breakCount)
		}

		data.Dependencies = append(data.Dependencies, htmlDep{
			Source:        dep.Source,
			CurrentVer:    dep.CurrentVer,
			LatestVer:     dep.LatestVer,
			UpdateClass:   updateClass,
			UpdateTypeStr: dep.UpdateType.String(),
			ImpactStr:     impact,
			FilePath:      dep.FilePath,
			Line:          dep.Line,
		})
	}

	for _, bc := range analysis.BreakingChanges {
		cssClass := "info"
		if bc.Severity >= breaking.SeverityBreaking {
			cssClass = "breaking"
		} else if bc.Severity >= breaking.SeverityWarning {
			cssClass = "warning"
		}

		effortLabel := ""
		switch bc.EffortLevel {
		case "small":
			effortLabel = "Low effort — 1-line change"
		case "medium":
			effortLabel = "Medium effort — a few attributes"
		case "large":
			effortLabel = "High effort — resource rewrite"
		}

		hb := htmlBreaking{
			SeverityStr:    bc.Severity.String(),
			CSSClass:       cssClass,
			ResourceType:   bc.ResourceType,
			Attribute:      bc.Attribute,
			Description:    bc.Description,
			MigrationGuide: bc.MigrationGuide,
			EffortLevel:    effortLabel,
			BeforeSnippet:  bc.BeforeSnippet,
			AfterSnippet:   bc.AfterSnippet,
		}

		// Attach real user code snippets from impact analysis
		for _, impact := range analysis.Impacts {
			if impact.ActualBefore == "" {
				continue
			}
			if impact.BreakingChange.ResourceType != bc.ResourceType || impact.BreakingChange.Attribute != bc.Attribute {
				continue
			}
			after := impact.ActualAfter
			if after == "" {
				after = impact.ActualBefore
			}
			hb.RealSnippets = append(hb.RealSnippets, htmlRealSnippet{
				ResourceName: impact.ResourceName,
				AffectedFile: impact.AffectedFile,
				AffectedLine: impact.AffectedLine,
				LinesChanged: impact.LinesChanged,
				Before:       impact.ActualBefore,
				After:        after,
			})
		}

		data.BreakingChanges = append(data.BreakingChanges, hb)
	}

	data.UpgradePaths = analysis.UpgradePaths

	// Recommendations
	for _, rec := range analysis.Recommendations {
		sevClass := "sev-low"
		switch rec.Severity {
		case "critical":
			sevClass = "sev-critical"
		case "high":
			sevClass = "sev-high"
		case "medium":
			sevClass = "sev-medium"
		}

		hr := htmlRecommendation{
			Severity: strings.ToUpper(rec.Severity),
			SevClass: sevClass,
			Title:    rec.Title,
			Details:  rec.Details,
		}

		for _, fix := range rec.Fix {
			parts := strings.SplitN(fix, "→", 2)
			if len(parts) != 2 {
				hr.Fix = append(hr.Fix, htmlDiffItem{OldValue: fix})
				continue
			}
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			filePart := ""
			oldValue := left
			if spaceIdx := strings.Index(left, "  "); spaceIdx > 0 {
				filePart = left[:spaceIdx]
				oldValue = strings.TrimSpace(left[spaceIdx:])
			}
			hr.Fix = append(hr.Fix, htmlDiffItem{
				FilePart: filePart,
				OldValue: oldValue,
				NewValue: right,
			})
		}

		data.Recommendations = append(data.Recommendations, hr)
	}

	// Provider Impact
	if analysis.ProviderImpact != nil {
		imp := analysis.ProviderImpact
		hi := &htmlProviderImpact{
			TargetProvider: imp.TargetProvider,
			TargetVersion:  imp.TargetVersion,
			Compatible:     imp.Compatible,
			NeedUpgrade:    imp.NeedUpgrade,
			Incompatible:   imp.Incompatible,
		}

		if imp.NeedUpgrade == 0 && imp.Incompatible == 0 {
			hi.VerdictClass = "safe"
			hi.VerdictText = fmt.Sprintf("Safe to upgrade %s to %s — all modules compatible.", imp.TargetProvider, imp.TargetVersion)
		} else if imp.Incompatible > 0 {
			hi.VerdictClass = "blocked"
			hi.VerdictText = fmt.Sprintf("Cannot upgrade %s to %s — %d module(s) have no compatible version.", imp.TargetProvider, imp.TargetVersion, imp.Incompatible)
		} else {
			hi.VerdictClass = "possible"
			hi.VerdictText = fmt.Sprintf("Upgrade possible after updating %d module(s).", imp.NeedUpgrade)
		}

		for _, r := range imp.Results {
			hr := htmlProviderImpactResult{
				Module:           r.Module,
				CurrentModuleVer: r.CurrentModuleVer,
				LatestModuleVer:  r.LatestModuleVer,
				RequiresUpgrade:  r.RequiresModuleUpgrade,
				MinCompatibleVer: r.MinCompatibleVer,
			}
			if r.Compatible && !r.RequiresModuleUpgrade {
				hr.StatusClass = "status-ok"
				hr.StatusLabel = "compatible"
			} else if r.RequiresModuleUpgrade && r.MinCompatibleVer != "" {
				hr.StatusClass = "status-minor"
				hr.StatusLabel = "upgrade to " + r.MinCompatibleVer
			} else {
				hr.StatusClass = "status-major"
				hr.StatusLabel = "incompatible"
			}
			hi.Results = append(hi.Results, hr)
		}

		data.ProviderImpact = hi
	}

	funcMap := template.FuncMap{
		"inc": func(i int) int { return i + 1 },
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing HTML template: %w", err)
	}

	return tmpl.Execute(w, data)
}
