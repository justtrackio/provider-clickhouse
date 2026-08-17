# provider-clickhouse

A [Crossplane](https://crossplane.io/) provider for [ClickHouse](https://clickhouse.com),
generated with [Upjet](https://github.com/crossplane/upjet) from the official
[ClickHouse Terraform provider](https://github.com/ClickHouse/terraform-provider-clickhouse).

It exposes all 25 resources of the upstream provider as Kubernetes custom
resources, in both cluster-scoped and namespaced (Crossplane v2) flavours:

| Group | Kinds |
|---|---|
| `service` | `Service`, `ScheduledScaling`, `UpgradeWindow`, `PrivateEndpointsAttachment`, `PrivateEndpointRegistration`, `TransparentDataEncryptionKeyAssociation` |
| `clickpipe` | `ClickPipe`, `CDCInfrastructure`, `ReversePrivateEndpoint`, `ReversePrivateEndpointCustomPrivateDNS` |
| `clickstack` | `SavedSearch`, `Dashboard`, `Alert`, `Source`, `Connection`, `Webhook`, `Team`, `TeamMember`, `Role` |
| `iam` | `Role`, `RoleAssignment` |
| `postgres` | `Service` |
| `udf` | `Function`, `Attachment` |
| `organization` | `Settings` |

API groups are `<group>.clickhouse.justtrack.io` (cluster-scoped) and
`<group>.clickhouse.m.justtrack.io` (namespaced).

## Versions

| Component | Version |
|---|---|
| ClickHouse Terraform provider | `3.25.0` |
| Terraform (bundled in image) | `1.5.7` |
| Upjet | v2 |

Terraform is pinned below 1.6.0 deliberately: 1.6+ is BSL licensed.

## Installation

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: contrib-provider-clickhouse
spec:
  package: ghcr.io/justtrackio/provider-clickhouse:v0.1.0
```

## Authentication

The upstream provider serves three surfaces that authenticate differently, and
every provider-level attribute is optional. This provider therefore validates
credential *sets* and rejects incomplete or contradictory combinations against
the `ProviderConfig` itself, rather than failing opaquely on every managed
resource.

| Mode | Required credential keys |
|---|---|
| ClickHouse Cloud | `organization_id`, `token_key`, `token_secret` (all three together) |
| ClickStack on ClickHouse Cloud | the Cloud set, plus `clickstack_service_id` |
| Self-hosted ClickStack (OSS/EE) | `clickstack_endpoint`, `clickstack_api_key` (both together) |

`clickstack_service_id` is mutually exclusive with the self-hosted pair.
Optional in any mode: `api_url`, `timeout_seconds` (integer).

Supplying only one set is valid; resources belonging to the other surface will
then error if used.

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

## Example: a team-scoped ClickStack saved search

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
    # Resolve the source by reference instead of hardcoding its id.
    sourceIdSelector:
      matchLabels:
        clickstack.justtrack.io/source: logs
    teamRef:
      name: marketing
```

`sourceId`, `team`, `connectionId`, `serviceId`, `roleId`, `savedSearchId` and
`channel.webhookId` all support `...Ref`/`...Selector` cross-resource
references, so a full ClickStack topology can be expressed without hardcoding
server-assigned ids.

## Caveats inherited from upstream

These are properties of the ClickHouse Terraform provider, not of this wrapper.

- **ClickStack is alpha upstream.** The `clickhouse_clickstack_*` resources emit
  an alpha warning at plan/apply time and their schema may change between
  provider versions. Pin the package version.
- **`team` is only valid on self-hosted ClickStack.** On ClickHouse Cloud a
  service *is* a single ClickStack team, and the upstream provider rejects the
  attribute. `Team`, `TeamMember` and the ClickStack `Role` kinds are likewise
  self-hosted only; on Cloud use the `iam` group instead.
- **`clickstack.Connection` is effectively read-only.** The platform provisions
  connections, so they can be observed but not updated or destroyed. Use
  `spec.managementPolicies: ["Observe"]`.
- **Saved search `filters` are format-sensitive.** They must be a JSON array of
  `{"type": "sql", "condition": "..."}` objects or the search will not render in
  the ClickStack sidebar.

## Secret handling

Fields the upstream schema marks sensitive are generated as secret references
and excluded from `status.atProvider`. For example `clickstack.Webhook` exposes
`urlSecretRef`, `bodySecretRef`, `headersSecretRef` and `queryParamsSecretRef`
in its spec, while its observation carries only non-secret metadata
(`id`, `name`, `service`, `team`, `description`, `headersVersion`,
`queryParamsVersion`).

One deviation is required: the upstream provider marks whole credential
*objects* sensitive on `clickhouse_clickpipe` (for example
`source.postgres.credentials`), which Upjet cannot express, since it can only
turn scalars and string collections into secret references. Those container
flags are cleared during generation by `config/groups.SanitizeSensitiveContainers`.
Nothing leaks: the secret-bearing leaves inside them (`password`, `username`,
`private_key`, ...) keep their own sensitivity and are still generated as secret
references.

## Development

```bash
make submodules          # fetch the crossplane/build submodule
make config/schema.json  # regenerate the Terraform provider schema
make generate            # regenerate CRDs, types and controllers
make build               # binary, image and xpkg
make test lint
make generated.lst       # list upstream resources, to audit coverage
```

Bumping the upstream provider means changing `TERRAFORM_PROVIDER_VERSION` in the
`Makefile`, re-running `make config/schema.json generate`, and reviewing
`make crddiff` / `make schema-version-diff` output for breaking changes.

### A note on the runtime mode

This provider uses Upjet's **Terraform CLI** execution mode, not the faster
no-CLI Plugin Framework mode. That is forced by upstream packaging rather than
chosen:

- The ClickHouse provider exposed importable Go packages under `pkg/` only up to
  `v3.18.1`. From `v3.19.0` everything moved under `internal/`, which Go forbids
  external modules from importing (a `go.mod` `replace` does not change this).
- ClickStack resources first appeared in `v3.20.0`.

No released version has both an importable `pkg/` tree and ClickStack, so
in-process Plugin Framework execution is impossible for ClickStack today. CLI
mode needs no Go imports of the provider, so it supports the full resource set.
If upstream re-exports a public package tree, switching to
`WithTerraformPluginFrameworkProvider` would be worthwhile.
