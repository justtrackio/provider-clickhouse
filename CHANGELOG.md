# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md` and issue templates.

### Fixed

- Restored Renovate's reserved `customManagers` field names
  (`datasourceTemplate`, `depNameTemplate`, `extractVersionTemplate`), which the upstream
  template's `hack/prepare.sh` had rewritten to `...ClickHouse` via a blanket
  `s/Template/<ProviderName>/g`, making `.github/renovate.json5` invalid. ([#1])
- Cleared all GitHub Actions deprecation and unknown-input warnings: bumped
  `docker/login-action` to v4.6.0, `docker/setup-buildx-action` to v4.2.0,
  `docker/setup-qemu-action` to v4.2.0 and `fkirc/skip-duplicate-actions` to v5.3.2 (all
  now `node24`), and removed the `install` input that no longer exists in
  `setup-buildx-action` v4.

## [0.1.0] - 2026-08-17

First release.

### Added

- Upjet-generated Crossplane provider for the official ClickHouse Terraform provider
  **3.25.0**, exposing **all 25 upstream resources** — including the 9 ClickStack (HyperDX)
  resources — as 50 managed-resource CRDs across cluster-scoped
  (`clickhouse.justtrack.io`) and namespaced (`clickhouse.m.justtrack.io`) API groups.
- API groups `service`, `clickpipe`, `clickstack`, `iam`, `postgres`, `udf`, `organization`.
- External-name configuration for all 25 resources, derived from each resource's documented
  `terraform import` syntax.
- Cross-resource references for `sourceId`, `team`, `connectionId`, `serviceId`, `roleId`,
  `savedSearchId` and `channel.webhookId`, so server-assigned ids never need hardcoding.
- Credential-set validation at the `ProviderConfig` for three authentication modes
  (ClickHouse Cloud, ClickStack on Cloud, self-hosted ClickStack), covered by unit tests.
- `config/groups.SanitizeSensitiveContainers`, which clears the `Sensitive` flag on
  non-scalar schema fields that Upjet cannot express as secret references — a generic
  recursive walk rather than a hardcoded path list, so future upstream versions that mark
  another container sensitive will not break code generation. Secret-bearing leaves keep
  their own sensitivity.
- Multi-arch package published to `ghcr.io/justtrackio/provider-clickhouse`
  (`linux/amd64`, `linux/arm64`).

### Notes

- Uses Upjet's Terraform CLI execution mode. This is forced by upstream packaging: the
  ClickHouse provider exposed importable Go packages under `pkg/` only through v3.18.1,
  moved them to `internal/` in v3.19.0, and added ClickStack in v3.20.0 — so no release has
  both an importable package tree and ClickStack.

[Unreleased]: https://github.com/justtrackio/provider-clickhouse/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/justtrackio/provider-clickhouse/releases/tag/v0.1.0
[#1]: https://github.com/justtrackio/provider-clickhouse/issues/1
