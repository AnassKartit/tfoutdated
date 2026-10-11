package output

import (
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/lifecycle"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// HTMLRenderer renders analysis results as standalone HTML.
type HTMLRenderer struct{}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Dependency plan — {{if .Target}}{{.Target}}{{else}}tfoutdated{{end}}</title>
<style>
  /* Layout: a terraform-plan summary on top, then one code-review style block per action. */
  :root {
    --ground: #f3f5f8; --paper: #ffffff; --rule: #d5dbe3; --rule-soft: #e7ebf0;
    --ink: #17202c; --ink-2: #4a5666; --ink-3: #6c7787;
    --accent: #275d8f;
    --del: #b3261e; --del-bg: #fdeceb; --del-line: #f4c7c3;
    --add: #17723a; --add-bg: #e7f6ec; --add-line: #b5e2c3;
    --warn: #8a5a00; --warn-bg: #fdf3dc;
    --code: #f7f9fb;
    --font-ui: "Segoe UI Variable Text", "Segoe UI", system-ui, -apple-system, "Helvetica Neue", Arial, sans-serif;
    --font-head: "Segoe UI Variable Display", "Segoe UI", system-ui, -apple-system, "Helvetica Neue", Arial, sans-serif;
    --font-code: "Cascadia Code", "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  @media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
      --ground: #0f141a; --paper: #161d26; --rule: #2a3442; --rule-soft: #212a36;
      --ink: #e6ebf1; --ink-2: #b2bccb; --ink-3: #8592a5;
      --accent: #7fb2e5;
      --del: #ff8a80; --del-bg: #2c1618; --del-line: #5a2a2d;
      --add: #6fd394; --add-bg: #10261a; --add-line: #1f4a2f;
      --warn: #e8b75c; --warn-bg: #2a2212;
      --code: #121820; color-scheme: dark;
    }
  }
  :root[data-theme="dark"] {
    --ground: #0f141a; --paper: #161d26; --rule: #2a3442; --rule-soft: #212a36;
    --ink: #e6ebf1; --ink-2: #b2bccb; --ink-3: #8592a5;
    --accent: #7fb2e5;
    --del: #ff8a80; --del-bg: #2c1618; --del-line: #5a2a2d;
    --add: #6fd394; --add-bg: #10261a; --add-line: #1f4a2f;
    --warn: #e8b75c; --warn-bg: #2a2212;
    --code: #121820; color-scheme: dark;
  }

  *, *::before, *::after { box-sizing: border-box; }
  html { -webkit-text-size-adjust: 100%; }
  body { margin: 0; background: var(--ground); color: var(--ink); font: 15px/1.6 var(--font-ui); }
  a { color: var(--accent); text-decoration: none; }
  a:hover { text-decoration: underline; }
  a:focus-visible, summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 3px; }
  code { font-family: var(--font-code); font-size: 0.88em; }
  .page { max-width: 1040px; margin: 0 auto; padding-inline: 20px; padding-block: 36px 56px; display: grid; gap: 40px; }

  /* Masthead */
  .mast { display: grid; gap: 18px; }
  .mast-top { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; flex-wrap: wrap; color: var(--ink-3); font-size: 13px; }
  .mast-top .tool { font-family: var(--font-code); letter-spacing: .02em; }
  h1 { font-family: var(--font-head); font-size: 30px; line-height: 1.15; font-weight: 650; margin: 0; text-wrap: balance; }
  .target { font-family: var(--font-code); font-size: 13px; color: var(--ink-2); overflow-wrap: anywhere; }

  .plan { background: var(--paper); border: 1px solid var(--rule); border-radius: 8px; padding: 18px 20px; display: grid; gap: 14px; }
  .plan-line { font-family: var(--font-code); font-size: 16px; font-weight: 600; margin: 0; }
  .plan-line .n-del { color: var(--del); } .plan-line .n-warn { color: var(--warn); } .plan-line .n-add { color: var(--add); }
  .bar { display: flex; height: 10px; border-radius: 5px; overflow: hidden; background: var(--rule-soft); }
  .bar span { display: block; min-width: 3px; }
  .bar .s-replace { background: var(--del); } .bar .s-major { background: color-mix(in srgb, var(--del) 55%, var(--warn)); }
  .bar .s-minor { background: var(--warn); } .bar .s-patch { background: color-mix(in srgb, var(--add) 70%, var(--paper)); } .bar .s-ok { background: var(--add); }
  .legend { display: flex; flex-wrap: wrap; gap: 6px 18px; font-size: 13px; color: var(--ink-2); font-variant-numeric: tabular-nums; }
  .legend i { display: inline-block; width: 9px; height: 9px; border-radius: 2px; margin-right: 6px; vertical-align: 0; }
  .plan-hint { margin: 0; font-size: 14px; color: var(--ink-2); }

  /* Sections */
  section { display: grid; gap: 14px; }
  .sec-head { display: grid; gap: 2px; }
  h2 { font-family: var(--font-head); font-size: 20px; font-weight: 650; margin: 0; display: flex; align-items: baseline; gap: 10px; }
  h2 .n { font-family: var(--font-code); font-size: 14px; color: var(--ink-3); font-weight: 500; }
  .sec-head p { margin: 0; color: var(--ink-2); font-size: 14px; max-width: 70ch; }

  /* One action: a file header, three answers, the diff */
  .item { background: var(--paper); border: 1px solid var(--rule); border-radius: 8px; overflow: hidden; }
  .item-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; padding: 10px 16px; background: var(--code); border-bottom: 1px solid var(--rule); }
  .item-head .file { font-family: var(--font-code); font-size: 13px; color: var(--ink-2); min-width: 0; overflow-wrap: anywhere; }
  .item-head .file b { color: var(--ink); font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .btn { font-size: 13px; border: 1px solid var(--rule); background: var(--paper); color: var(--ink); border-radius: 6px; padding: 3px 10px; white-space: nowrap; }
  .btn:hover { border-color: var(--accent); color: var(--accent); text-decoration: none; }
  .btn.go { border-color: var(--add-line); color: var(--add); }

  .answers { display: grid; grid-template-columns: 7.5rem 1fr; gap: 6px 16px; padding: 14px 16px 4px; font-size: 14px; }
  .answers dt { color: var(--ink-3); font-size: 12px; text-transform: uppercase; letter-spacing: .06em; padding-top: 2px; }
  .answers dd { margin: 0; min-width: 0; }
  .tag { display: inline-block; font-size: 11.5px; font-weight: 700; letter-spacing: .04em; padding: 1px 7px; border-radius: 4px; margin-right: 6px; vertical-align: 1px; }
  .tag.del { background: var(--del-bg); color: var(--del); } .tag.warn { background: var(--warn-bg); color: var(--warn); } .tag.add { background: var(--add-bg); color: var(--add); }

  .diff { margin: 10px 16px 16px; border: 1px solid var(--rule); border-radius: 6px; background: var(--code); overflow-x: auto; }
  .diff pre { margin: 0; padding: 10px 0; font-family: var(--font-code); font-size: 13px; line-height: 1.65; }
  .diff .l { display: block; padding: 0 14px; white-space: pre; }
  .diff .l.del { background: var(--del-bg); color: var(--del); }
  .diff .l.add { background: var(--add-bg); color: var(--add); }
  .diff .l.ctx { color: var(--ink-3); }
  .diff .l.note { color: var(--ink-3); font-style: italic; }

  .evidence { border-top: 1px solid var(--rule-soft); padding: 12px 16px 14px; display: grid; gap: 6px; }
  .evidence h3 { margin: 0; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--ink-3); font-weight: 600; }
  .evidence ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; font-size: 14px; }
  .evidence li { display: flex; gap: 8px; align-items: baseline; min-width: 0; }
  .evidence svg { flex: none; width: 14px; height: 14px; position: relative; top: 2px; }
  .evidence .src { color: var(--ink-3); font-family: var(--font-code); font-size: 12px; overflow-wrap: anywhere; }

  /* Breaking changes and paths reuse the item frame */
  .bc-body { padding: 14px 16px; display: grid; gap: 8px; font-size: 14px; }
  .bc-body p { margin: 0; }
  .muted { color: var(--ink-2); }
  .twocol { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .twocol .diff { margin: 0; }
  .twocol .cap { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--ink-3); padding: 6px 14px 0; }
  .path { display: flex; flex-wrap: wrap; gap: 6px; font-family: var(--font-code); font-size: 13px; padding: 12px 16px 14px; align-items: center; }
  .path .ok { color: var(--add); } .path .bad { color: var(--del); }

  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th { text-align: left; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--ink-3); font-weight: 600; padding: 8px 16px; border-bottom: 1px solid var(--rule); }
  td { padding: 8px 16px; border-bottom: 1px solid var(--rule-soft); font-variant-numeric: tabular-nums; }
  tr:last-child td { border-bottom: 0; }

  footer { color: var(--ink-3); font-size: 13px; border-top: 1px solid var(--rule); padding-top: 16px; display: flex; flex-wrap: wrap; gap: 8px 20px; justify-content: space-between; }

  @media (max-width: 700px) {
    h1 { font-size: 24px; }
    .answers { grid-template-columns: 1fr; gap: 2px; }
    .answers dd { margin-bottom: 8px; }
    .twocol { grid-template-columns: 1fr; }
  }
  @media print { body { background: #fff; } .item { break-inside: avoid; } }
</style>
</head>
<body>
<div class="page">

<header class="mast">
  <div class="mast-top"><span class="tool">tfoutdated</span><span>{{.GeneratedAt}}</span></div>
  <h1>Dependency plan</h1>
  {{if .Target}}<div class="target">{{if .TargetLink}}<a href="{{.TargetLink}}" target="_blank" rel="noopener">{{.Target}}</a>{{else}}{{.Target}}{{end}}</div>{{end}}

  <div class="plan">
    <p class="plan-line">Plan: <span class="{{if .Deprecated}}n-del{{end}}">{{len .Deprecated}} to replace</span>, <span class="{{if gt .MajorCount 0}}n-del{{end}}">{{.MajorCount}} major</span>, <span class="{{if gt .MinorCount 0}}n-warn{{end}}">{{.MinorCount}} minor</span>, <span class="{{if gt .PatchCount 0}}n-add{{end}}">{{.PatchCount}} patch</span>{{if gt .BreakingCount 0}}, <span class="n-del">{{.BreakingCount}} breaking</span>{{end}}.</p>
    {{if gt .Total 0}}
    <div class="bar" role="img" aria-label="{{len .Deprecated}} to replace, {{.MajorCount}} major, {{.MinorCount}} minor, {{.PatchCount}} patch, {{.Current}} current out of {{.Total}}">
      {{if .Deprecated}}<span class="s-replace" style="flex-grow: {{len .Deprecated}}"></span>{{end}}
      {{if gt .MajorCount 0}}<span class="s-major" style="flex-grow: {{.MajorCount}}"></span>{{end}}
      {{if gt .MinorCount 0}}<span class="s-minor" style="flex-grow: {{.MinorCount}}"></span>{{end}}
      {{if gt .PatchCount 0}}<span class="s-patch" style="flex-grow: {{.PatchCount}}"></span>{{end}}
      {{if gt .Current 0}}<span class="s-ok" style="flex-grow: {{.Current}}"></span>{{end}}
    </div>
    <div class="legend">
      <span><i class="s-replace" style="background: var(--del)"></i>{{len .Deprecated}} replace</span>
      <span><i style="background: color-mix(in srgb, var(--del) 55%, var(--warn))"></i>{{.MajorCount}} major</span>
      <span><i style="background: var(--warn)"></i>{{.MinorCount}} minor</span>
      <span><i style="background: color-mix(in srgb, var(--add) 70%, var(--paper))"></i>{{.PatchCount}} patch</span>
      <span><i style="background: var(--add)"></i>{{.Current}} current</span>
      <span>of {{.Total}} dependencies</span>
    </div>
    {{end}}
    {{if .Deprecated}}<p class="plan-hint">Start with the replacements: a newer version of a deprecated module still gets no fixes.</p>
    {{else if .Upgrades}}<p class="plan-hint">Preview every change with <code>tfoutdated plan</code>, then apply with <code>tfoutdated apply</code>.</p>
    {{else}}<p class="plan-hint">Nothing to do. Every dependency is current and maintained.</p>{{end}}
  </div>
</header>

{{if .Deprecated}}
<section id="replace">
  <div class="sec-head">
    <h2>Replace <span class="n">{{len .Deprecated}}</span></h2>
    <p>These modules are deprecated, archived or moved upstream. Each block shows the line to change and links to the proof.</p>
  </div>
  {{range .Deprecated}}
  <article class="item">
    <div class="item-head">
      <div class="file">{{if .FileLink}}<a href="{{.FileLink}}">{{.FilePath}}:{{.Line}}</a>{{else}}{{.FilePath}}:{{.Line}}{{end}} · <b>module "{{.Name}}"</b></div>
      <div class="actions">
        {{if .OldLink}}<a class="btn" href="{{.OldLink}}" target="_blank" rel="noopener">Current module</a>{{end}}
        {{if .OldRepo}}<a class="btn" href="{{.OldRepo}}/releases" target="_blank" rel="noopener">Release notes</a>{{end}}
        {{if .NewLink}}<a class="btn go" href="{{.NewLink}}" target="_blank" rel="noopener">Replacement</a>{{end}}
      </div>
    </div>
    <dl class="answers">
      <dt>Changed</dt><dd><span class="tag del">DEPRECATED</span>The module no longer receives updates{{if .CurrentVer}}; you are on {{.CurrentVer}}{{end}}.</dd>
      <dt>Impact</dt><dd>No bug or security fixes from now on.{{if .ReplacedBy}} The replacement can take different inputs, so compare them on its module page before switching.{{end}}</dd>
      <dt>Do this</dt><dd>{{if .ReplacedBy}}Change the source in <code>{{.FilePath}}</code>, then run <code>terraform init -upgrade</code> and <code>terraform plan</code>.{{else}}No replacement is named upstream. Pick a maintained module, or fork this one if you depend on it.{{end}}</dd>
    </dl>
    <div class="diff"><pre><span class="l ctx">  module "{{.Name}}" {</span><span class="l del">-   source  = "{{.Source}}"</span>{{if .OldVersion}}<span class="l del">-   version = "{{.OldVersion}}"</span>{{end}}{{if .ReplacedBy}}<span class="l add">+   source  = "{{.ReplacedBy}}"</span>{{if .NewVersion}}<span class="l add">+   version = "{{.NewVersion}}"</span>{{end}}{{else}}<span class="l note">    # no replacement named upstream</span>{{end}}<span class="l ctx">  }</span></pre></div>
    {{if .Evidence}}
    <div class="evidence">
      <h3>Check it yourself</h3>
      <ul>{{range .Evidence}}<li><svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="var(--del-bg)" stroke="var(--del)" stroke-width="1.2"/><path d="M5 8.2l2 2 4-4.4" fill="none" stroke="var(--del)" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg><span>{{if .URL}}<a href="{{.URL}}" target="_blank" rel="noopener">{{.Label}}</a> <span class="src">{{.URL}}</span>{{else}}{{.Label}}{{end}}</span></li>{{end}}</ul>
    </div>
    {{end}}
  </article>
  {{end}}
</section>
{{end}}

{{if .Upgrades}}
<section id="upgrade">
  <div class="sec-head">
    <h2>Upgrade <span class="n">{{len .Upgrades}}</span></h2>
    <p>Newer versions are published. <code>tfoutdated apply</code> makes these edits for you; <code>-safe</code> stops before breaking versions.</p>
  </div>
  {{range .Upgrades}}
  <article class="item">
    <div class="item-head">
      <div class="file">{{if .FileLink}}<a href="{{.FileLink}}">{{.FilePath}}:{{.Line}}</a>{{else}}{{.FilePath}}:{{.Line}}{{end}} · <b>{{if .IsModule}}module "{{.Name}}"{{else}}provider "{{.Name}}"{{end}}</b></div>
      <div class="actions">
        {{if .Link}}<a class="btn" href="{{.Link}}" target="_blank" rel="noopener">{{if .IsModule}}Module{{else}}Provider{{end}} page</a>{{end}}
        {{if .LatestLink}}<a class="btn go" href="{{.LatestLink}}" target="_blank" rel="noopener">{{.LatestVer}}</a>{{end}}
      </div>
    </div>
    <dl class="answers">
      <dt>Changed</dt><dd><span class="tag {{if eq .UpdateClass "major"}}del{{else if eq .UpdateClass "minor"}}warn{{else}}add{{end}}">{{.UpdateTypeStr}}</span>{{.CurrentVer}} → {{.LatestVer}} <span class="muted">· {{.Source}}</span></dd>
      <dt>Impact</dt><dd>{{if .BreakCount}}{{.BreakCount}} breaking change{{if gt .BreakCount 1}}s{{end}}, listed under <a href="#breaking">Breaking changes</a>.{{else if eq .UpdateClass "major"}}Major version: read the release notes before upgrading.{{else}}No known breaking change.{{end}}</dd>
      <dt>Do this</dt><dd>{{if .SafeTarget}}Upgrade to {{.SafeTarget}} first, which has no known breaking change, then review the rest.{{else}}Change the version in <code>{{.FilePath}}</code>, then run <code>terraform init -upgrade</code>.{{end}}</dd>
    </dl>
    <div class="diff"><pre><span class="l del">-   version = "{{.OldConstraint}}"</span><span class="l add">+   version = "{{.NewConstraint}}"</span></pre></div>
  </article>
  {{end}}
</section>
{{end}}

{{if .BreakingChanges}}
<section id="breaking">
  <div class="sec-head">
    <h2>Breaking changes <span class="n">{{len .BreakingChanges}}</span></h2>
    <p>What changes in your code when you upgrade.</p>
  </div>
  {{range .BreakingChanges}}
  <article class="item">
    <div class="item-head"><div class="file"><span class="tag {{if eq .CSSClass "breaking"}}del{{else if eq .CSSClass "warning"}}warn{{else}}add{{end}}">{{.SeverityStr}}</span>{{if .ResourceType}}<b>{{.ResourceType}}{{if .Attribute}}.{{.Attribute}}{{end}}</b>{{end}}</div>{{if .EffortLevel}}<div class="muted" style="font-size: 13px">{{.EffortLevel}}</div>{{end}}</div>
    <div class="bc-body">
      <p>{{.Description}}</p>
      {{if .MigrationGuide}}<p class="muted">{{.MigrationGuide}}</p>{{end}}
      {{if .RealSnippets}}{{range .RealSnippets}}
      <p class="muted"><code>{{.ResourceName}}</code> in <code>{{.AffectedFile}}:{{.AffectedLine}}</code>{{if gt .LinesChanged 0}} · {{.LinesChanged}} lines{{end}}</p>
      <div class="twocol"><div class="diff"><div class="cap">Your code</div><pre>{{.Before}}</pre></div><div class="diff"><div class="cap">After the fix</div><pre>{{.After}}</pre></div></div>
      {{end}}{{else if and .BeforeSnippet .AfterSnippet}}
      <div class="twocol"><div class="diff"><div class="cap">Before</div><pre>{{.BeforeSnippet}}</pre></div><div class="diff"><div class="cap">After</div><pre>{{.AfterSnippet}}</pre></div></div>
      {{end}}
    </div>
  </article>
  {{end}}
</section>
{{end}}

{{if .Recommendations}}
<section id="recommendations">
  <div class="sec-head"><h2>Recommendations <span class="n">{{len .Recommendations}}</span></h2></div>
  {{range .Recommendations}}
  <article class="item">
    <div class="item-head"><div class="file"><span class="tag {{if or (eq .SevClass "critical") (eq .SevClass "high")}}del{{else if eq .SevClass "medium"}}warn{{else}}add{{end}}">{{.Severity}}</span><b>{{.Title}}</b></div></div>
    <div class="bc-body">
      {{range .Details}}<p class="muted">{{.}}</p>{{end}}
      {{if .Fix}}<div class="diff" style="margin: 0"><pre>{{range .Fix}}{{if .FilePart}}<span class="l ctx">  {{.FilePart}}</span>{{end}}{{if .OldValue}}<span class="l del">-   {{.OldValue}}</span>{{end}}{{if .NewValue}}<span class="l add">+   {{.NewValue}}</span>{{end}}{{end}}</pre></div>{{end}}
    </div>
  </article>
  {{end}}
</section>
{{end}}

{{if .ProviderImpact}}
<section id="impact">
  <div class="sec-head">
    <h2>Provider upgrade <span class="n">{{.ProviderImpact.TargetProvider}} → {{.ProviderImpact.TargetVersion}}</span></h2>
    <p>{{.ProviderImpact.VerdictText}}</p>
  </div>
  <article class="item">
    <table>
      <thead><tr><th>Module</th><th>Version</th><th>Status</th></tr></thead>
      <tbody>{{range .ProviderImpact.Results}}<tr><td><code>{{.Module}}</code></td><td><code>{{.CurrentModuleVer}}{{if .LatestModuleVer}} → {{.LatestModuleVer}}{{end}}</code></td><td><span class="tag {{if eq .StatusClass "ok"}}add{{else if eq .StatusClass "possible"}}warn{{else}}del{{end}}">{{.StatusLabel}}</span></td></tr>{{end}}</tbody>
    </table>
  </article>
</section>
{{end}}

{{if .UpgradePaths}}
<section id="paths">
  <div class="sec-head">
    <h2>Upgrade paths <span class="n">{{len .UpgradePaths}}</span></h2>
    <p>Step by step. Red steps carry breaking changes.</p>
  </div>
  {{range .UpgradePaths}}
  <article class="item">
    <div class="item-head"><div class="file"><b>{{.Name}}</b></div>{{if .NonBreakingTarget}}<div class="muted" style="font-size: 13px">safe target {{.NonBreakingTarget}}</div>{{end}}</div>
    <div class="path">{{range $i, $s := .Steps}}{{if eq $i 0}}<span>{{$s.From}}</span>{{end}}<span class="{{if $s.Safe}}ok{{else}}bad{{end}}">→ {{$s.To}}</span>{{end}}</div>
  </article>
  {{end}}
</section>
{{end}}

<footer>
  <span>Generated by <a href="https://github.com/anasskartit/tfoutdated">tfoutdated</a></span>
  <span>Re-run: <code>tfoutdated check</code></span>
</footer>

</div>
</body>
</html>`

type htmlData struct {
	GeneratedAt string
	Target      string
	TargetLink  string

	// Plan line and distribution bar, all counts of dependencies.
	Total   int
	Current int

	Deprecated      []htmlDeprecated
	Upgrades        []htmlUpgrade
	BreakingChanges []htmlBreaking
	UpgradePaths    []analyzer.UpgradePath
	Recommendations []htmlRecommendation
	ProviderImpact  *htmlProviderImpact

	MajorCount    int
	MinorCount    int
	PatchCount    int
	BreakingCount int
}

// htmlDeprecated is one "stop using this, use that" card. Every claim on it
// links to a page the reader can open to check it.
type htmlDeprecated struct {
	Name       string
	IsModule   bool
	Source     string
	CurrentVer string
	FilePath   string
	Line       int
	FileLink   template.URL
	OldLink    string // registry page or repository of the deprecated module
	OldRepo    string // repository, for release notes
	ReplacedBy string
	NewVersion string // version to pin on the replacement
	OldVersion string // version line as written today, if any
	NewLink    string
	Evidence   []lifecycle.Evidence
}

type htmlUpgrade struct {
	Name          string
	IsModule      bool
	Source        string
	OldConstraint string // the version line as written
	NewConstraint string // the same line pointing at the target version
	SafeTarget    string // newest version without known breaking changes, when the latest breaks
	Link          string // registry page of the module or provider
	CurrentVer    string
	LatestVer     string
	LatestLink    string // registry page at the latest version
	UpdateClass   string
	UpdateTypeStr string
	BreakCount    int
	FilePath      string
	Line          int
	FileLink      template.URL
}

type htmlRecommendation struct {
	Severity string
	SevClass string
	Title    string
	Details  []string
	Fix      []htmlDiffItem
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

// fileLink opens a file at a line in VS Code; "" when the path cannot be made
// absolute. Readers on other editors still see the path itself.
//
// html/template rejects the vscode: scheme as unsafe; the URL is built here
// from a local path, so it is marked trusted rather than interpolated raw.
func fileLink(path string, line int) template.URL {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	u := url.URL{Scheme: "vscode", Path: "file/" + strings.TrimPrefix(filepath.ToSlash(abs), "/") + fmt.Sprintf(":%d", line)}
	return template.URL(u.String())
}

// constraintChange returns the version line as written and the same line
// pointing at the target, keeping the operator the author chose ("~> 0.14.0"
// becomes "~> 0.14.1"). A range ("> 1, < 2") is replaced by the bare target.
func constraintChange(raw, current, target string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = current
	}
	if strings.Contains(raw, ",") {
		return raw, target
	}
	op := strings.TrimRight(strings.TrimRight(raw, "0123456789.-+abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"), " ")
	if op == "" {
		return raw, target
	}
	return raw, op + " " + target
}

// displayPath shows a file relative to the scanned directory, which is how
// the reader knows it; the link keeps the full path.
func displayPath(base, path string) string {
	if base == "" || !filepath.IsAbs(path) {
		return path
	}
	if rel, err := filepath.Rel(base, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

// depLinks returns the page of a dependency and the page of a given version.
func depLinks(dep analyzer.DependencyAnalysis) (page, latest string) {
	if dep.IsModule {
		return lifecycle.RegistryURL(dep.Source, ""), lifecycle.RegistryURL(dep.Source, dep.LatestVer)
	}
	src := dep.Source
	if !strings.Contains(src, "/") && dep.Namespace != "" {
		src = dep.Namespace + "/" + dep.Name
	}
	return lifecycle.ProviderURL(src, ""), lifecycle.ProviderURL(src, dep.LatestVer)
}

func (r *HTMLRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	data := htmlData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04"),
		Target:      analysis.Target,
	}
	if strings.HasPrefix(analysis.Target, "http") {
		data.TargetLink = analysis.Target
	}

	// Newest non-breaking version per dependency, from the upgrade paths.
	safeTargets := map[string]string{}
	for _, up := range analysis.UpgradePaths {
		if up.HasBreakingSteps() && up.NonBreakingTarget != "" {
			safeTargets[up.Name] = up.NonBreakingTarget
		}
	}

	needsAction := 0
	for _, dep := range analysis.Dependencies {
		if dep.Deprecated || dep.UpdateType != resolver.UpdateNone {
			needsAction++
		}
	}
	data.Total = analysis.TotalDeps
	if data.Total < needsAction {
		data.Total = needsAction
	}
	data.Current = data.Total - needsAction

	for _, dep := range analysis.Dependencies {
		breakCount := 0
		for _, bc := range analysis.BreakingChanges {
			if (bc.Provider == dep.Name || bc.Provider == dep.Source) && bc.IsBreaking() {
				breakCount++
			}
		}

		if dep.Deprecated {
			old, _ := depLinks(dep)
			d := htmlDeprecated{
				Name:       dep.Name,
				IsModule:   dep.IsModule,
				Source:     dep.Source,
				CurrentVer: dep.CurrentVer,
				FilePath:   displayPath(analysis.Target, dep.FilePath),
				Line:       dep.Line,
				FileLink:   fileLink(dep.FilePath, dep.Line),
				OldLink:    old,
				ReplacedBy: dep.ReplacedBy,
				NewVersion: dep.ReplacedByVersion,
				OldVersion: dep.Constraint,
				NewLink:    lifecycle.RegistryURL(dep.ReplacedBy, dep.ReplacedByVersion),
				Evidence:   dep.Evidence,
			}
			if len(d.Evidence) == 0 && dep.DeprecationReason != "" {
				// No page to link to: still say why, as plain labels.
				for _, reason := range strings.Split(dep.DeprecationReason, "; ") {
					d.Evidence = append(d.Evidence, lifecycle.Evidence{Label: reason})
				}
			}
			for _, e := range dep.Evidence {
				if strings.HasPrefix(e.URL, "https://github.com/") && d.OldRepo == "" {
					d.OldRepo = repoRoot(e.URL)
				}
			}
			data.Deprecated = append(data.Deprecated, d)
		}

		// A deprecated module is counted and shown once, under Replace:
		// upgrading it does not help.
		if dep.Deprecated || dep.UpdateType == resolver.UpdateNone {
			continue
		}
		class := "patch"
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			class = "major"
			data.MajorCount++
		case resolver.UpdateMinor:
			class = "minor"
			data.MinorCount++
		default:
			data.PatchCount++
		}
		page, latest := depLinks(dep)
		oldC, newC := constraintChange(dep.Constraint, dep.CurrentVer, dep.LatestVer)
		data.Upgrades = append(data.Upgrades, htmlUpgrade{
			Name:          dep.Name,
			IsModule:      dep.IsModule,
			OldConstraint: oldC,
			NewConstraint: newC,
			SafeTarget:    safeTargets[dep.Name],
			Source:        dep.Source,
			Link:          page,
			CurrentVer:    dep.CurrentVer,
			LatestVer:     dep.LatestVer,
			LatestLink:    latest,
			UpdateClass:   class,
			UpdateTypeStr: dep.UpdateType.String(),
			BreakCount:    breakCount,
			FilePath:      displayPath(analysis.Target, dep.FilePath),
			Line:          dep.Line,
			FileLink:      fileLink(dep.FilePath, dep.Line),
		})
	}

	for _, bc := range analysis.BreakingChanges {
		cssClass := "info"
		if bc.Severity >= breaking.SeverityBreaking {
			cssClass = "breaking"
			data.BreakingCount++
		} else if bc.Severity >= breaking.SeverityWarning {
			cssClass = "warning"
		}

		effortLabel := ""
		switch bc.EffortLevel {
		case "small":
			effortLabel = "low effort, one-line change"
		case "medium":
			effortLabel = "medium effort, a few attributes"
		case "large":
			effortLabel = "high effort, resource rewrite"
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

	for _, rec := range analysis.Recommendations {
		hr := htmlRecommendation{
			Severity: strings.ToUpper(rec.Severity),
			SevClass: strings.ToLower(rec.Severity), // critical, high, medium, low: styled in the template
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
			hi.VerdictText = fmt.Sprintf("Safe to upgrade %s to %s: all modules are compatible.", imp.TargetProvider, imp.TargetVersion)
		} else if imp.Incompatible > 0 {
			hi.VerdictClass = "blocked"
			hi.VerdictText = fmt.Sprintf("Cannot upgrade %s to %s yet: %d module(s) have no compatible version.", imp.TargetProvider, imp.TargetVersion, imp.Incompatible)
		} else {
			hi.VerdictClass = "possible"
			hi.VerdictText = fmt.Sprintf("Upgrade possible after updating %d module(s).", imp.NeedUpgrade)
		}

		for _, res := range imp.Results {
			hr := htmlProviderImpactResult{
				Module:           res.Module,
				CurrentModuleVer: res.CurrentModuleVer,
				LatestModuleVer:  res.LatestModuleVer,
				RequiresUpgrade:  res.RequiresModuleUpgrade,
				MinCompatibleVer: res.MinCompatibleVer,
			}
			switch {
			case res.Compatible && !res.RequiresModuleUpgrade:
				hr.StatusClass, hr.StatusLabel = "ok", "compatible"
			case res.RequiresModuleUpgrade && res.MinCompatibleVer != "":
				hr.StatusClass, hr.StatusLabel = "possible", "upgrade to "+res.MinCompatibleVer
			default:
				hr.StatusClass, hr.StatusLabel = "blocked", "incompatible"
			}
			hi.Results = append(hi.Results, hr)
		}

		data.ProviderImpact = hi
	}

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing HTML template: %w", err)
	}
	return tmpl.Execute(w, data)
}

// repoRoot trims a GitHub URL to "https://github.com/owner/repo".
func repoRoot(u string) string {
	rest := strings.TrimPrefix(u, "https://github.com/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return "https://github.com/" + parts[0] + "/" + strings.SplitN(parts[1], "#", 2)[0]
}
