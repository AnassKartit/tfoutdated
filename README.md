<p align="center">
  <h1 align="center">tfoutdated</h1>
  <p align="center">
    <strong>Keep your Terraform dependencies up to date across AWS, Azure, and GCP</strong>
  </p>
  <p align="center">
    Scan, detect breaking changes, and auto-fix outdated modules &amp; providers across AWS, Azure, and GCP.
  </p>
</p>

<p align="center">
  <a href="https://github.com/AnassKartit/tfoutdated/releases"><img src="https://img.shields.io/github/v/release/AnassKartit/tfoutdated?style=flat-square&color=blue" alt="Release"></a>
  <a href="https://github.com/AnassKartit/tfoutdated/actions"><img src="https://img.shields.io/github/actions/workflow/status/AnassKartit/tfoutdated/ci.yml?style=flat-square&label=CI" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/anasskartit/tfoutdated"><img src="https://goreportcard.com/badge/github.com/anasskartit/tfoutdated?style=flat-square" alt="Go Report Card"></a>
  <a href="https://github.com/AnassKartit/tfoutdated/blob/main/LICENSE"><img src="https://img.shields.io/github/license/AnassKartit/tfoutdated?style=flat-square" alt="License"></a>
  <a href="https://github.com/AnassKartit/tfoutdated/stargazers"><img src="https://img.shields.io/github/stars/AnassKartit/tfoutdated?style=flat-square" alt="Stars"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#auto-fix-in-action">Auto-Fix Demo</a> &middot;
  <a href="#cicd-integration">CI/CD</a> &middot;
  <a href="#mcp-server">MCP Server</a> &middot;
  <a href="CHANGELOG.md">Changelog</a>
</p>

---

<p align="center">
  <img src="demo/demo.gif" alt="tfoutdated demo" width="900">
</p>

---

## Why tfoutdated?

Other tools bump the version number in your `.tf` file. **tfoutdated also fixes your code.**

It downloads both module versions, diffs their variable schemas, detects renames and removals, and rewrites your module calls to match the new API.

| Feature | tfoutdated | [tfupdate](https://github.com/minamijoyo/tfupdate) | [Renovate](https://docs.renovatebot.com/modules/manager/terraform/) | [Dependabot](https://docs.github.com/en/code-security/dependabot) |
|---------|:----------:|:--------:|:--------:|:----------:|
| Bump version constraints | &check; | &check; | &check; | &check; |
| **Detect breaking changes between versions** | &check; | &cross; | &cross; | &cross; |
| **Auto-rename variables in module calls** | &check; | &cross; | &cross; | &cross; |
| **Auto-update provider constraints from module deps** | &check; | &cross; | &cross; | &cross; |
| **Schema diff (download &amp; compare both versions)** | &check; | &cross; | &cross; | &cross; |
| Upgrade path recommendations | &check; | &cross; | &cross; | &cross; |
| Creates PRs automatically | &cross; | &cross;\* | &check; | &check; |
| MCP server (AI editor integration) | &check; | &cross; | &cross; | &cross; |
| Multi-cloud (AWS, Azure, GCP) | &check; | &check; | &check; | &check; |

\* tfupdate can be combined with CI to create PRs, but doesn't do it natively.

## Auto-Fix in Action

```diff
 # Before: tfoutdated fix -p ./terraform
 module "eks" {
   source  = "terraform-aws-modules/eks/aws"
-  version = "~> 19.0.0"
+  version = "~> 21.15.1"

-  cluster_name                   = "prod-cluster"
-  cluster_version                = "1.27"
-  cluster_endpoint_public_access = true
+  name                   = "prod-cluster"
+  kubernetes_version     = "1.27"
+  endpoint_public_access = true

 terraform {
   required_providers {
-    aws = { source = "hashicorp/aws", version = "~> 5.30" }
+    aws = { source = "hashicorp/aws", version = "~> 6.28" }
   }
 }
```

```
$ tfoutdated fix -p ./terraform

  main.tf
    ✓ eks  19.0.0 → 21.15.1
    ✓ s3_bucket  3.0.0 → 5.10.0
    ↻ eks  rename cluster_name → name
    ↻ eks  rename cluster_version → kubernetes_version
    ↻ eks  rename cluster_endpoint_public_access → endpoint_public_access
    ↻ eks  rename cluster_addons → addons
    ⚡ aws  ~> 5.30 → ~> 6.28

  7 changes applied:  2 upgraded · 4 renamed · 1 constraints
```

### Tested with real-world modules

| Cloud | Modules Tested |
|-------|---------------|
| **AWS** | EKS, VPC, S3, Lambda, RDS, ALB, ECS |
| **Azure** | VNet, ACR, Key Vault, Storage, Service Bus, NSG |
| **GCP** | GKE, Cloud NAT, Network, Cloud Run, Cloud SQL |

## Quick Start

```bash
# Install
brew install anasskartit/tap/tfoutdated

# Scan for outdated dependencies
tfoutdated scan -p /path/to/terraform

# Auto-fix everything: versions, renames, provider constraints
tfoutdated fix -p /path/to/terraform

# Safe mode: only non-breaking upgrades
tfoutdated fix --safe -p /path/to/terraform

# Preview changes without modifying files
tfoutdated fix --dry-run -p /path/to/terraform

# Full report: breaking changes + recommendations + impact
tfoutdated scan --full -p /path/to/terraform

# JSON output for CI pipelines
tfoutdated scan -o json

# HTML report
tfoutdated scan --output-file report.html
```

## Installation

### Bash script (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/AnassKartit/tfoutdated/main/install.sh | bash
```

### Go install

```bash
go install github.com/anasskartit/tfoutdated@latest
```

### Homebrew (macOS/Linux)

```bash
brew install anasskartit/tap/tfoutdated
```

### Docker

```bash
docker run --rm -v $(pwd):/data ghcr.io/anasskartit/tfoutdated scan -p /data
```

### Chocolatey (Windows)

```powershell
choco install tfoutdated
```

### GitHub Releases

Download pre-built binaries from [Releases](https://github.com/AnassKartit/tfoutdated/releases) for Linux, macOS, and Windows (amd64/arm64).

## Scan Output

The demo GIF above shows the full scan + fix flow. tfoutdated outputs a colored table with breaking change counts, grouped breaking changes with auto-fix labels, and upgrade path recommendations.

## How It Works

1. **Scan** — Reads your `.tf` files and resolves current vs latest versions from Terraform Registry
2. **Schema Diff** — Downloads both module versions from GitHub, parses HCL, compares variable schemas
3. **Rename Detection** — Multi-signal bipartite matching (name similarity, type, description, defaults) with 0.45 threshold
4. **Value Inference** — Derives accessor changes from variable name suffixes (e.g., `resource_group_name` &rarr; `parent_id` implies `.name` &rarr; `.id`)
5. **Provider Resolution** — Fetches module provider dependencies from registry API, merges constraints across all upgraded modules
6. **Fix** — Applies version bumps, variable renames, value transforms, attribute removals, and provider constraint updates in one pass

## Commands

| Command | Description |
|---------|-------------|
| `scan` | Detect outdated dependencies with breaking change analysis |
| `fix` | Auto-fix versions, renames, and provider constraints |
| `fix --safe` | Only upgrade to non-breaking versions |
| `recommend` | Generate governance recommendations |
| `report` | Verify breaking changes with `terraform validate` |

## Flags

| Flag | Description |
|------|-------------|
| `-p, --path` | Path to Terraform directory (default: `.`) |
| `-r, --recursive` | Recursively scan subdirectories (default: `true`) |
| `-o, --output` | Output format: `table`, `json`, `markdown`, `html`, `github`, `azdevops` |
| `--output-file` | Write report to file (auto-detects format from extension) |
| `--full` | Full report: scan + breaking + recommendations + impact |
| `--impact` | Provider impact analysis (e.g., `hashicorp/azurerm`) |
| `--target-version` | Target provider version for impact analysis |
| `--safe` | (fix) Only non-breaking upgrades |
| `--dry-run` | Show changes without modifying files |
| `--repos` | File with repo URLs/paths for multi-repo scanning |
| `--no-color` | Disable colored output |

## CI/CD Integration

### GitHub Action

```yaml
- uses: AnassKartit/tfoutdated@v0.4.0
  with:
    path: './terraform'
    fail-on-outdated: 'true'
```

### Azure DevOps Pipeline

```yaml
- script: |
    curl -sSL https://raw.githubusercontent.com/AnassKartit/tfoutdated/main/install.sh | bash
    tfoutdated scan -p ./terraform -o azdevops
  displayName: 'Check Terraform Dependencies'
```

### GitLab CI

```yaml
terraform-outdated:
  image: ghcr.io/anasskartit/tfoutdated:latest
  script:
    - tfoutdated scan -p ./terraform -o json > report.json
  artifacts:
    reports:
      codequality: report.json
```

## MCP Server

Use tfoutdated as an AI-powered tool in Claude, Cursor, Windsurf, Copilot, or any MCP-compatible assistant.

```bash
# Install
go install github.com/anasskartit/tfoutdated/cmd/tfoutdated-mcp@latest

# Claude Code
claude mcp add tfoutdated tfoutdated-mcp
```

<details>
<summary>Other editors (Cursor, Copilot, Gemini CLI, Codex)</summary>

Add to your MCP config:

```json
{
  "mcpServers": {
    "tfoutdated": {
      "command": "tfoutdated-mcp"
    }
  }
}
```

**Tools:** `tfoutdated_scan`, `tfoutdated_recommend`, `tfoutdated_impact`, `tfoutdated_full_report`, `tfoutdated_html_report`

</details>

## Configuration

```yaml
# .tfoutdated.yml
ignore:
  - name: "legacy-module"
    reason: "Pinned for compatibility"
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All dependencies up to date |
| `1` | Outdated dependencies found |
| `2` | Breaking changes detected |

## Star History

<a href="https://star-history.com/#AnassKartit/tfoutdated&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=AnassKartit/tfoutdated&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=AnassKartit/tfoutdated&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=AnassKartit/tfoutdated&type=Date" />
 </picture>
</a>

## Contributing

Contributions welcome! Please open an issue or PR on [GitHub](https://github.com/AnassKartit/tfoutdated).

## License

[MIT](LICENSE)
