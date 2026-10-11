---
name: tfoutdated
description: Keep Terraform module and provider dependencies current, and catch modules that are deprecated, archived or moved upstream (Terraform Registry, GitHub, Azure Verified Modules). Use when writing or reviewing module blocks, upgrading providers or modules, or auditing a Terraform repository.
---

# tfoutdated

`tfoutdated` reads Terraform, finds outdated modules and providers, flags modules
whose upstream has stopped (deprecated, archived, moved), and can upgrade them
with the code changes the new versions need.

## Before writing or upgrading a module block

Check the module first, so new code never starts on a dead module:

```bash
tfoutdated module Azure/avm-res-network-virtualnetwork/azurerm
```

- Exit `0`: usable. Pin to the `latest` version it prints.
- Exit `2`: deprecated or archived. Do not use it. If it prints `replaced by`, use that module instead.
- With the MCP server, call `tfoutdated_module_status` with the `source`.

## Reviewing a repository

```bash
tfoutdated check            # report only, changes nothing
tfoutdated check -format=json
```

Report findings in this order:
1. Deprecated or archived modules, with `replaced_by` and `deprecation_reason`.
2. Major upgrades and breaking changes.
3. Minor and patch upgrades.

## Upgrading

```bash
tfoutdated plan             # what would change; no file is modified
tfoutdated apply            # asks for confirmation
tfoutdated apply -safe      # only upgrades without breaking changes
```

Run `plan` and show it before `apply`. In a pipeline or as an agent, `apply` needs
`-auto-approve`; only pass it when the user asked for the changes to be made.
After `apply`, run `terraform init -upgrade` and `terraform validate`.

## In CI

```bash
tfoutdated check -fail-on=deprecated,major
```

Exit codes: `0` nothing matched, `1` error, `2` findings matched `-fail-on`.
Every flag can also be set as an environment variable, for example
`TFOUTDATED_FAIL_ON=deprecated`. Set `GH_TOKEN` for repositories with many
modules (GitHub allows 60 unauthenticated requests per hour).

## Do not

- Do not suggest a module reported as deprecated, even when it is the latest version.
- Do not run `apply -auto-approve` without the user asking for changes.
- Do not treat "upstream check incomplete" as clean: say the check could not finish.
