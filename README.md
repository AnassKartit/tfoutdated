<p align="center">
  <h1 align="center">tfoutdated</h1>
  <p align="center">
    <strong>Keep your Terraform dependencies up to date across AWS, Azure, and GCP</strong>
  </p>
  <p align="center">
    Scan, detect breaking changes, and auto-fix outdated modules &amp; providers — in HCL and CDKTF.
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
  <a href="#cdktf-support">CDKTF</a> &middot;
  <a href="#terragrunt-support">Terragrunt</a> &middot;
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
| **CDKTF support (cdktf.json + package.json)** | &check; | &cross; | &cross; | &cross; |
| **Terragrunt support (terragrunt.hcl)** | &check; | &cross; | &cross; | &check;\*\* |
| Creates PRs automatically | &cross; | &cross;\* | &check; | &check; |
| MCP server (AI editor integration) | &check; | &cross; | &cross; | &cross; |
| Multi-cloud (AWS, Azure, GCP) | &check; | &check; | &check; | &check; |

\* tfupdate can be combined with CI to create PRs, but doesn't do it natively.
\*\* Dependabot has basic Terragrunt version bumping but no breaking change detection or code transforms.

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

See [live CI results](https://github.com/AnassKartit/tfoutdated-tests/actions) across all three clouds + CDKTF.

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
```

## Installation

<details>
<summary><strong>All installation methods</strong></summary>

### Homebrew (macOS/Linux)

```bash
brew install anasskartit/tap/tfoutdated
```

### Bash script (Linux/macOS)

```bash
curl -sSL https://raw.githubusercontent.com/AnassKartit/tfoutdated/main/install.sh | bash
```

### Go install

```bash
go install github.com/anasskartit/tfoutdated@latest
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

</details>

## Features

### Scan — Detect outdated dependencies

```bash
tfoutdated scan -p ./terraform
```

Reads `.tf` files (or `cdktf.json`) and checks the [Terraform Registry](https://registry.terraform.io) for newer versions. Shows a colored table with update types, breaking change counts, and impact.

```bash
# JSON output (for scripts and CI)
tfoutdated scan -p ./terraform -o json

# Markdown output
tfoutdated scan -p ./terraform -o markdown

# HTML report to file
tfoutdated scan -p ./terraform --output-file report.html

# Full report: scan + breaking changes + recommendations + impact
tfoutdated scan -p ./terraform --full

# Verbose: show all breaking changes (default truncates at 10)
tfoutdated scan -p ./terraform --verbose
```

### Fix — Auto-upgrade with code transforms

```bash
tfoutdated fix -p ./terraform
```

Bumps versions **and** applies code changes:

- **Version bumps** — Updates version constraints in `.tf`, `cdktf.json`, `package.json`, and `terragrunt.hcl`
- **Variable renames** — Rewrites renamed attributes in module calls (e.g., `cluster_name` &rarr; `name`)
- **Value transforms** — Updates accessor patterns (e.g., `.name` &rarr; `.id`)
- **Attribute removals** — Removes deleted attributes with comments
- **Provider constraints** — Updates `required_providers` to match module dependencies

```bash
# Preview changes without modifying files
tfoutdated fix --dry-run -p ./terraform

# Only non-breaking upgrades (safe mode)
tfoutdated fix --safe -p ./terraform
```

### Breaking Change Detection

tfoutdated detects breaking changes in two ways:

1. **Knowledge base** — Hand-curated rules for major provider upgrades (azurerm 3→4, aws 5→6)
2. **Schema diffing** — Downloads both module versions, parses HCL variables, and compares schemas using bipartite matching

```bash
# See full breaking change report
tfoutdated scan --full -p ./terraform
```

Breaking changes are categorized:
- **Renames** — Variable renamed (auto-fixable)
- **Removals** — Variable removed
- **Type changes** — Variable type changed
- **Behavior changes** — Default value or validation changed

### Provider Impact Analysis

Analyze how a provider upgrade affects your codebase:

```bash
# Impact of upgrading azurerm
tfoutdated scan --impact hashicorp/azurerm -p ./terraform

# Target a specific version
tfoutdated scan --impact hashicorp/azurerm --target-version 4.0.0 -p ./terraform
```

### Multi-Path and Multi-Repo Scanning

```bash
# Scan multiple paths
tfoutdated scan -p ./infra/prod,./infra/staging,./infra/dev

# Scan repos from a file (one URL/path per line)
tfoutdated scan --repos repos.txt
```

### Recommendations

```bash
tfoutdated recommend -p ./terraform
```

Generates governance recommendations: pinning strategy, upgrade priority, risk assessment.

## CDKTF Support

tfoutdated scans CDKTF (TypeScript/Python) projects alongside standard HCL. Two patterns are supported:

### 1. Module wrappers via `cdktf.json`

If your CDKTF project uses Terraform Registry modules, tfoutdated reads `terraformModules` from `cdktf.json`:

```json
{
  "terraformModules": [
    {
      "name": "eks",
      "source": "terraform-aws-modules/eks/aws",
      "version": "19.0.0"
    },
    {
      "name": "vpc",
      "source": "terraform-aws-modules/vpc/aws",
      "version": "4.0.0"
    }
  ],
  "terraformProviders": [
    "hashicorp/aws@~> 5.30"
  ]
}
```

```bash
$ tfoutdated scan -p ./my-cdktf-project

  3 outdated (3 major) · 51 breaking (32 auto-fixable)

 DEPENDENCY                           LOCATION        CURRENT     LATEST      TYPE
 terraform-aws-modules/eks/aws        cdktf.json:1    19.0.0      21.15.1     MAJOR ↑2
 terraform-aws-modules/s3-bucket/aws  cdktf.json:3    3.0.0       5.10.0      MAJOR ↑2
 terraform-aws-modules/vpc/aws        cdktf.json:2    4.0.0       6.6.0       MAJOR ↑2
```

`tfoutdated fix` updates versions directly in `cdktf.json`:

```bash
$ tfoutdated fix -p ./my-cdktf-project

  cdktf.json
    ✓ eks  19.0.0 → 21.15.1
    ✓ s3_bucket  3.0.0 → 5.10.0
    ✓ vpc  4.0.0 → 6.6.0
    ⚡ aws  ~> 5.30 → ~> 6.28

  4 changes applied:  3 upgraded · 1 constraints
```

Provider constraints in both string (`"hashicorp/aws@~> 5.30"`) and object (`{"name": "azurerm", "version": "~> 3.75"}`) formats are supported.

### 2. Native TypeScript providers via `package.json`

If you use `@cdktf/provider-*` npm packages, tfoutdated detects them in `package.json` and maps them to the underlying Terraform provider:

```json
{
  "dependencies": {
    "@cdktf/provider-aws": "^19.0.0",
    "@cdktf/provider-azurerm": "^11.0.0"
  }
}
```

The `fix` command preserves npm version prefixes (`^`, `~`) while updating the version:

```bash
$ tfoutdated fix -p ./my-cdktf-project

  package.json
    ⚡ aws  19.0.0 → ^6.28.0
```

Supported provider packages: `aws`, `azurerm`, `google`, `azuread`, `azapi`, `kubernetes`, `helm`, `null`, `random`, `local`, `external`, `tls`, `dns`, `time`, `archive`, `http`.

See [live CDKTF CI results](https://github.com/AnassKartit/tfoutdated-tests/actions) for AWS and Azure.

## Terragrunt Support

tfoutdated scans `terragrunt.hcl` files that use Terraform Registry modules via the `tfr:///` source format.

```hcl
# terragrunt.hcl
terraform {
  source = "tfr:///terraform-aws-modules/eks/aws?version=19.0.0"
}

inputs = {
  cluster_name    = "production-cluster"
  cluster_version = "1.27"
}
```

```bash
$ tfoutdated scan -p ./my-terragrunt-project

  1 outdated (1 major) · 50 breaking (32 auto-fixable)

 DEPENDENCY                      LOCATION          CURRENT   LATEST    TYPE
 terraform-aws-modules/eks/aws   terragrunt.hcl:1  19.0.0    21.15.1   MAJOR ↑2
```

`tfoutdated fix` rewrites the `?version=` parameter in-place:

```bash
$ tfoutdated fix -p ./my-terragrunt-project

  terragrunt.hcl
    ✓ eks  19.0.0 → 21.15.1

  1 changes applied:  1 upgraded
```

```diff
 terraform {
-  source = "tfr:///terraform-aws-modules/eks/aws?version=19.0.0"
+  source = "tfr:///terraform-aws-modules/eks/aws?version=21.15.1"
 }
```

Supported source formats:
- `tfr:///namespace/name/provider?version=X.Y.Z` — Terraform Registry
- `tfr://namespace/name/provider?version=X.Y.Z` — alternate double-slash
- `git::https://github.com/org/repo.git?ref=vX.Y.Z` — Git sources with version tags

See [live Terragrunt CI results](https://github.com/AnassKartit/tfoutdated-tests/actions) for AWS and Azure.

## Output Formats

| Format | Flag | Use Case |
|--------|------|----------|
| Table | `-o table` (default) | Terminal — colored, grouped, truncated |
| JSON | `-o json` | CI pipelines, scripts, programmatic access |
| Markdown | `-o markdown` | PR comments, documentation |
| HTML | `-o html` or `--output-file report.html` | Standalone reports |
| GitHub | `-o github` (auto-detected in Actions) | Annotations + `GITHUB_STEP_SUMMARY` |
| Azure DevOps | `-o azdevops` (auto-detected in Pipelines) | `##vso` commands + collapsible sections |

CI format is auto-detected: GitHub Actions and Azure DevOps are selected automatically when running in those environments.

## CI/CD Integration

### GitHub Action

```yaml
- uses: AnassKartit/tfoutdated@v0.5.0
  with:
    path: './terraform'
    fail-on-outdated: 'true'
```

Or with the install script:

```yaml
- name: Install tfoutdated
  run: curl -sSL https://raw.githubusercontent.com/AnassKartit/tfoutdated/main/install.sh | bash

- name: Scan
  run: tfoutdated scan -p ./terraform

- name: Fix (dry run)
  run: tfoutdated fix --dry-run -p ./terraform
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

## Commands

| Command | Description |
|---------|-------------|
| [`scan`](#scan--detect-outdated-dependencies) | Detect outdated dependencies with breaking change analysis |
| [`fix`](#fix--auto-upgrade-with-code-transforms) | Auto-fix versions, renames, and provider constraints |
| [`fix --safe`](#fix--auto-upgrade-with-code-transforms) | Only upgrade to non-breaking versions |
| [`recommend`](#recommendations) | Generate governance recommendations |
| `report` | Verify breaking changes with `terraform validate` |

## Flags

| Flag | Description |
|------|-------------|
| `-p, --path` | Path to Terraform/CDKTF directory (default: `.`) |
| `-r, --recursive` | Recursively scan subdirectories (default: `true`) |
| `-o, --output` | Output format: `table`, `json`, `markdown`, `html`, `github`, `azdevops` |
| `--output-file` | Write report to file (auto-detects format from extension) |
| `--full` | Full report: scan + breaking + recommendations + impact |
| `--impact` | Provider impact analysis (e.g., `hashicorp/azurerm`) |
| `--target-version` | Target provider version for impact analysis |
| `--safe` | (fix) Only non-breaking upgrades |
| `--dry-run` | Show changes without modifying files |
| `-v, --verbose` | Show all breaking changes (no truncation) |
| `--repos` | File with repo URLs/paths for multi-repo scanning |
| `--no-color` | Disable colored output |

## How It Works

1. **Scan** — Reads `.tf` files, `cdktf.json`, `package.json`, and `terragrunt.hcl`, resolves current vs latest versions from [Terraform Registry](https://registry.terraform.io)
2. **Schema Diff** — Downloads both module versions from GitHub, parses HCL, compares variable schemas
3. **Rename Detection** — Multi-signal bipartite matching (name similarity, type, description, defaults)
4. **Value Inference** — Derives accessor changes from variable name suffixes (e.g., `resource_group_name` &rarr; `parent_id` implies `.name` &rarr; `.id`)
5. **Provider Resolution** — Fetches module provider dependencies from registry API, merges constraints across all upgraded modules
6. **Fix** — Applies version bumps, variable renames, value transforms, attribute removals, and provider constraint updates in one pass

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
