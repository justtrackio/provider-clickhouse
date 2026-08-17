# provider-clickhouse

**Manage ClickHouse Cloud _and_ your ClickStack (HyperDX) observability stack as Kubernetes resources.**

[![Release](https://img.shields.io/github/v/release/justtrackio/provider-clickhouse?sort=semver&logo=github)](https://github.com/justtrackio/provider-clickhouse/releases/latest)
[![CI](https://github.com/justtrackio/provider-clickhouse/actions/workflows/ci.yml/badge.svg)](https://github.com/justtrackio/provider-clickhouse/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/justtrackio/provider-clickhouse)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/justtrackio/provider-clickhouse)](go.mod)
[![Crossplane](https://img.shields.io/badge/Crossplane-v2%20ready-1F6FEB)](https://crossplane.io/)
[![ClickHouse Terraform provider](https://img.shields.io/badge/ClickHouse%20TF%20provider-3.25.0-FFCC01)](https://github.com/ClickHouse/terraform-provider-clickhouse)

A [Crossplane](https://crossplane.io/) provider for [ClickHouse](https://clickhouse.com),
generated with [Upjet](https://github.com/crossplane/upjet) from the official
[ClickHouse Terraform provider](https://github.com/ClickHouse/terraform-provider-clickhouse).

## Why this provider

It covers **all 25 upstream resources — including the 9 ClickStack (HyperDX) ones**, which
means your observability configuration becomes GitOps-managed alongside everything else:

> Saved searches, dashboards, alerts, sources, webhooks, teams and roles as YAML,
> reconciled continuously, reviewed in pull requests, and deleted when the branch is.

If you have ever rebuilt a HyperDX saved search by hand after someone deleted it, or
struggled to keep the same set of dashboards across staging and production, this is
the gap it closes. No other Crossplane provider exposes ClickStack today — see
[why](#why-terraform-cli-mode) for the (fairly interesting) packaging reason.

## Quickstart

Install the provider:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: contrib-provider-clickhouse
spec:
  package: ghcr.io/justtrackio/provider-clickhouse:v0.1.0
```

Point it at a self-hosted ClickStack:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: clickstack-creds
  namespace: crossplane-system
type: Opaque
stringData:
  credentials: |
    {
      "clickstack_endpoint": "http://hyperdx.observability.svc.cluster.local:8000",
      "clickstack_api_key": "REPLACE_ME"
    }
---
apiVersion: clickhouse.justtrack.io/v1beta1
kind: ProviderConfig
metadata:
  name: clickstack
spec:
  credentials:
    source: Secret
    secretRef:
      name: clickstack-creds
      namespace: crossplane-system
      key: credentials
```

Now declare a team-scoped saved search:

```yaml
apiVersion: clickstack.clickhouse.justtrack.io/v1alpha1
kind: SavedSearch
metadata:
  name: production-errors
spec:
  providerConfigRef:
    name: clickstack
  forProvider:
    name: Production errors
    where: SeverityText:error
    whereLanguage: lucene
    select: Timestamp, ServiceName, Body
    orderBy: Timestamp DESC
    tags: [production, errors]
    # Resolve server-assigned ids by reference instead of hardcoding them.
    sourceIdSelector:
      matchLabels:
        clickstack.justtrack.io/source: logs
    teamRef:
      name: marketing
```

`sourceId`, `team`, `connectionId`, `serviceId`, `roleId`, `savedSearchId` and
`channel.webhookId` all support `...Ref`/`...Selector` cross-resource references, so a
whole ClickStack topology can be expressed without a single hardcoded id.

## What you can manage

All 25 upstream resources, in both cluster-scoped and namespaced (Crossplane v2) flavours —
50 managed-resource CRDs in total.

| Group | Kinds |
|---|---|
| `clickstack` | `SavedSearch`, `Dashboard`, `Alert`, `Source`, `Connection`, `Webhook`, `Team`, `TeamMember`, `Role` |
| `service` | `Service`, `ScheduledScaling`, `UpgradeWindow`, `PrivateEndpointsAttachment`, `PrivateEndpointRegistration`, `TransparentDataEncryptionKeyAssociation` |
| `clickpipe` | `ClickPipe`, `CDCInfrastructure`, `ReversePrivateEndpoint`, `ReversePrivateEndpointCustomPrivateDNS` |
| `iam` | `Role`, `RoleAssignment` |
| `udf` | `Function`, `Attachment` |
| `postgres` | `Service` |
| `organization` | `Settings` |

API groups are `<group>.clickhouse.justtrack.io` (cluster-scoped) and
`<group>.clickhouse.m.justtrack.io` (namespaced).

## Which ClickHouse Crossplane provider should I use?

There is one other Upjet-based option, and it is a reasonable choice for a subset of use
cases. An honest comparison:

| | justtrackio/provider-clickhouse | [lansweeper-oss/provider-clickhouse](https://github.com/lansweeper-oss/provider-clickhouse) |
|---|---|---|
| ClickHouse Cloud resources | ✅ | ✅ |
| ClickStack / HyperDX resources | ✅ all 9 | ❌ none |
| Upstream provider version | 3.25.0 | 3.18.1 |
| Execution mode | Terraform CLI | Plugin Framework (in-process) |
| Runtime footprint | larger (bundles Terraform) | **smaller and faster** |
| Namespaced (Crossplane v2) APIs | ✅ | — |

Use theirs if you only need ClickHouse Cloud and want the leaner in-process runtime.
Use this one if you need ClickStack, newer resources, or Crossplane v2 namespaced APIs.

## Authentication

The upstream provider serves three surfaces that authenticate differently, and every
provider-level attribute is optional. This provider therefore validates credential *sets*
and rejects incomplete or contradictory combinations against the `ProviderConfig` itself,
rather than failing opaquely on every managed resource.

| Mode | Required credential keys |
|---|---|
| ClickHouse Cloud | `organization_id`, `token_key`, `token_secret` (all three together) |
| ClickStack on ClickHouse Cloud | the Cloud set, plus `clickstack_service_id` |
| Self-hosted ClickStack (OSS/EE) | `clickstack_endpoint`, `clickstack_api_key` (both together) |

`clickstack_service_id` is mutually exclusive with the self-hosted pair. Optional in any
mode: `api_url`, `timeout_seconds` (integer). Supplying only one set is valid; resources
belonging to the other surface will then error if used.

## Secret handling

Fields the upstream schema marks sensitive are generated as secret references and excluded
from `status.atProvider`. For example `clickstack.Webhook` exposes `urlSecretRef`,
`bodySecretRef`, `headersSecretRef` and `queryParamsSecretRef` in its spec, while its
observation carries only non-secret metadata (`id`, `name`, `service`, `team`,
`description`, `headersVersion`, `queryParamsVersion`).

One deviation is required: the upstream provider marks whole credential *objects* sensitive
on `clickhouse_clickpipe` (for example `source.postgres.credentials`), which Upjet cannot
express, since it can only turn scalars and string collections into secret references.
Those container flags are cleared during generation by
`config/groups.SanitizeSensitiveContainers`. Nothing leaks: the secret-bearing leaves
inside them (`password`, `username`, `private_key`, ...) keep their own sensitivity and are
still generated as secret references.

## Caveats inherited from upstream

These are properties of the ClickHouse Terraform provider, not of this wrapper.

- **ClickStack is alpha upstream.** The `clickhouse_clickstack_*` resources emit an alpha
  warning at plan/apply time and their schema may change between provider versions. Pin the
  package version.
- **`team` is only valid on self-hosted ClickStack.** On ClickHouse Cloud a service *is* a
  single ClickStack team, and the upstream provider rejects the attribute. `Team`,
  `TeamMember` and the ClickStack `Role` kinds are likewise self-hosted only; on Cloud use
  the `iam` group instead.
- **`clickstack.Connection` is effectively read-only.** The platform provisions connections,
  so they can be observed but not updated or destroyed. Use
  `spec.managementPolicies: ["Observe"]`.
- **Saved search `filters` are format-sensitive.** They must be a JSON array of
  `{"type": "sql", "condition": "..."}` objects or the search will not render in the
  ClickStack sidebar.

## Why Terraform CLI mode

This provider uses Upjet's **Terraform CLI** execution mode, not the faster no-CLI Plugin
Framework mode. That is forced by upstream packaging rather than chosen:

- The ClickHouse provider exposed importable Go packages under `pkg/` only up to **v3.18.1**.
  From **v3.19.0** everything moved under `internal/`, which Go forbids external modules
  from importing (a `go.mod` `replace` does not change this).
- ClickStack resources first appeared in **v3.20.0**.

**No released version has both an importable `pkg/` tree and ClickStack**, so in-process
Plugin Framework execution is impossible for ClickStack today. CLI mode needs no Go imports
of the provider, so it supports the full resource set — which is also why the only other
Upjet ClickHouse provider, pinned to 3.18.1 for Plugin Framework mode, has no ClickStack
resources. If upstream re-exports a public package tree, switching to
`WithTerraformPluginFrameworkProvider` would be worthwhile; that is tracked in
[ClickHouse/terraform-provider-clickhouse#661](https://github.com/ClickHouse/terraform-provider-clickhouse/issues/661).

## Versions

| Component | Version |
|---|---|
| ClickHouse Terraform provider | `3.25.0` |
| Terraform (bundled in image) | `1.5.7` |
| Upjet | v2 |

Terraform is pinned below 1.6.0 deliberately: 1.6+ is BSL licensed.

## Development

```bash
make submodules          # fetch the crossplane/build submodule
make config/schema.json  # regenerate the Terraform provider schema
make generate            # regenerate CRDs, types and controllers
make build               # binary, image and xpkg
make test lint
make generated.lst       # list upstream resources, to audit coverage
```

Bumping the upstream provider means changing `TERRAFORM_PROVIDER_VERSION` in the `Makefile`,
re-running `make config/schema.json generate`, and reviewing `make crddiff` /
`make schema-version-diff` output for breaking changes.

## Contributing

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). If you hit a
resource that does not behave as expected, please include the generated CRD and the upstream
Terraform resource name.

## License

Apache 2.0 — see [LICENSE](LICENSE).
