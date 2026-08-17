# Contributing

Thanks for considering a contribution. This provider is generated with
[Upjet](https://github.com/crossplane/upjet), which changes where you should make edits.

## The most important rule

**Do not edit generated code.** Everything under `apis/`, `internal/controller/` and
`package/crds/` is produced by `make generate`. Edits there are lost on the next run.

Change the inputs instead:

| To change | Edit |
|---|---|
| Which resources are exposed, their external-name / import behaviour | `config/external_name.go` |
| API groups, kind names, cross-resource references | `config/groups/config.go` |
| Sensitive-field handling | `config/groups/sensitive.go` |
| Credential validation and provider setup | `internal/clients/clickhouse.go` |
| Upstream provider version, Terraform version | `Makefile` |

Then run `make generate` and commit the regenerated output together with your change.

## Development setup

```bash
git clone --recurse-submodules git@github.com:justtrackio/provider-clickhouse.git
cd provider-clickhouse
make submodules          # if you forgot --recurse-submodules
make config/schema.json  # regenerate the Terraform provider schema
make generate            # regenerate CRDs, types and controllers
make build               # binary, image and xpkg
make test lint
```

`make generated.lst` prints the sorted list of upstream resource names from
`config/schema.json`, which is useful for auditing coverage after a version bump.

## Bumping the upstream ClickHouse provider

1. Change `TERRAFORM_PROVIDER_VERSION` in the `Makefile`.
2. `make config/schema.json generate`
3. Review `make crddiff` and `make schema-version-diff` for breaking changes.
4. Add any new resources to `config/external_name.go`, deriving the external-name config
   from the resource's documented `terraform import` syntax rather than guessing.
5. Update the resource table in `README.md` and add a `CHANGELOG.md` entry.

Note that `make generate` will panic if a new upstream version marks a non-scalar field as
sensitive. That is handled generically in `config/groups/sensitive.go`; if you hit it,
confirm the walk covers the new shape rather than special-casing the path.

## Before opening a pull request

- `make test lint` passes, and `make generate` leaves no uncommitted diff.
- Commit messages explain *why*, not just *what*.
- Add a `CHANGELOG.md` entry under `## [Unreleased]`.

## Reporting bugs

Please include:

- the managed resource YAML you applied (with secrets redacted),
- the `status.conditions` and any events on the resource,
- the corresponding upstream Terraform resource name (`clickhouse_...`),
- the provider package version.

Many surprises come from the upstream Terraform provider rather than this wrapper — the
[Caveats](README.md#caveats-inherited-from-upstream) section lists the known ones. Checking
whether the equivalent Terraform configuration behaves the same way is the fastest way to
tell the two apart, and makes the report much easier to act on.

## Code of conduct

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).
