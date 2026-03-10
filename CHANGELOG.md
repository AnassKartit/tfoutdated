# Changelog

## v0.4.0

### Added

- **Multi-signal variable rename detection** — Bipartite matching with 6 weighted signals (name similarity, description, type, default, required, sensitive) replaces naive string matching. Threshold 0.45 with greedy 1:1 assignment for deterministic results
- **Value transform hints** — When a variable rename implies an accessor change (e.g., `resource_group_name` → `parent_id`), the fixer auto-rewrites values like `.name` → `.id` with confidence scoring
- **Provider constraint auto-fix** — Fetches module provider dependencies from Terraform Registry, merges requirements across all upgraded modules, and updates `required_providers` block including cross-major bumps (e.g., azurerm `~> 3.75` → `~> 4.0`)
- **Multi-line attribute removal** — `findAttributeExtent` with brace/bracket/heredoc counting for correct removal of block-valued attributes
- **Major version upgrade support** — `fix` now upgrades across major versions (AWS EKS 19→21, GCP GKE 28→44, etc.) with full transform support

### Changed

- Schema diffing is now fully dynamic — no hardcoded patterns for variable renames or value transforms
- `fix` command now upgrades all versions by default (use `--safe` for non-breaking only)
- Provider constraints derived from module requirements, not standalone provider bumps
- Rename detection excludes matched targets from RequiredAdded false positives

### Fixed

- Provider constraint conflict: standalone provider bumps no longer override module-required constraints
- Module transforms no longer applied when version wasn't actually bumped

### Tested

- **30+ real scenarios across AWS, Azure, and GCP** pass `terraform init && terraform validate`
- **AWS:** EKS 19→21 (4 renames), S3 3→5, Lambda 4→8, RDS 5→7, EC2 4→6, ALB 8→10, ECS 4→7, VPC 4→6, DynamoDB 3→5, SQS 3→5
- **Azure AVM:** VNet, ACR, Service Bus, SQL, Key Vault, Storage, PostgreSQL, CosmosDB, NSG, multi-module
- **GCP:** Cloud Run 0.3→0.25, GKE 28→44, Cloud NAT 4→7, Network 8→16, Cloud Functions, Pub/Sub, GCS, Memorystore, Cloud SQL, multi-module

## v0.3.0

### Added

- **Dynamic schema diffing** — Detects breaking changes by comparing module input/output schemas between versions, complementing the curated knowledge base
- **AVM breaking change knowledge** — Expanded coverage for Azure Verified Modules (AVM)
- **Self-update check** — Notifies when a newer version is available
- **Docker support** — Published to `ghcr.io/anasskartit/tfoutdated`
- **Chocolatey package** — `choco install tfoutdated` on Windows
- **Homebrew formula** — `brew install anasskartit/tap/tfoutdated`

### Changed

- `proof` command renamed to `report` for clarity
- Unified schema diffing across `scan`, `fix`, and `report` commands
- `--safe` upgrade paths now correctly respect breaking change boundaries

### Removed

- `wizard` command (interactive TUI mode)
- `validate` command

### Fixed

- Version replacement bug in `fix` command that could produce malformed constraints
- Double-prefix issue in report output
- Fixer now correctly handles multi-file version constraints

## v0.2.0

Initial release.

- Dependency scanning for Terraform modules and providers
- Breaking change detection with curated Azure provider knowledge base
- Lock file awareness (`.terraform.lock.hcl`)
- Upgrade path recommendations
- Auto-fix with `--safe` mode
- Impact analysis for provider upgrades
- Governance recommendations
- Multiple output formats (table, JSON, Markdown, HTML, GitHub Actions, Azure DevOps)
- CI/CD integration (GitHub Actions, Azure DevOps)
- Multi-repo / multi-path scanning
- MCP server for AI-assisted workflows
