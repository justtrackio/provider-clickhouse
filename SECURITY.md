# Security Policy

## Supported versions

This project is pre-1.0. Security fixes are applied to the latest released minor version
only.

| Version | Supported |
|---|---|
| 0.1.x | ✅ |
| < 0.1 | ❌ |

## Reporting a vulnerability

**Please do not report security vulnerabilities in public issues.**

Use GitHub's private vulnerability reporting for this repository:
[Report a vulnerability](https://github.com/justtrackio/provider-clickhouse/security/advisories/new).
That opens a private channel visible only to the maintainers.

Please include the affected provider version, the managed resource or code path involved, and
a description of the impact. A minimal reproduction helps a great deal.

We will acknowledge the report and keep you informed as we work on a fix, and will credit you
in the advisory unless you prefer otherwise.

## Scope

This repository is a thin, generated wrapper around the official
[ClickHouse Terraform provider](https://github.com/ClickHouse/terraform-provider-clickhouse).

Please report to us anything in **this** repository's own code and packaging, including:

- credential handling in `internal/clients/`,
- secret-reference generation and whether any sensitive value can reach
  `status.atProvider`,
- the provider image and its bundled Terraform binary,
- release and publishing workflows.

Vulnerabilities in the upstream Terraform provider, in ClickHouse Cloud, or in ClickStack
itself should be reported to those projects. If you are unsure which applies, report it here
and we will help route it.

## Handling of credentials

Provider credentials are read from a Kubernetes `Secret` referenced by a `ProviderConfig` and
are never written to managed-resource status. Fields the upstream schema marks sensitive are
generated as secret references and excluded from `status.atProvider`.

One deviation is documented in the [README](README.md#secret-handling): the upstream provider
marks whole credential *objects* sensitive on `clickhouse_clickpipe`, which Upjet cannot
express as a secret reference. Those container-level flags are cleared during code
generation, while the secret-bearing leaves inside them keep their own sensitivity and are
still generated as secret references. If you find a path where that reasoning does not hold
and a secret becomes observable, please treat it as a vulnerability and report it.
