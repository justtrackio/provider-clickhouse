package config

import (
	"github.com/crossplane/upjet/v2/pkg/config"
)

// ExternalNameConfigs contains all external name configurations for this
// provider.
//
// Upjet only generates a CRD for a resource that appears here, so this map is
// also the provider's include list (see ExternalNameConfigured, wired into
// ujconfig.WithIncludeList in provider.go). Every resource exposed by the
// ClickHouse Terraform provider v3.25.0 is covered.
//
// Choosing a configuration per resource comes down to how the upstream
// resource is identified, which we take from its documented `terraform import`
// syntax rather than guessing:
//
//   - IdentifierFromProvider: the API assigns an opaque id that the resource
//     exposes as a read-only `id` attribute. The overwhelming majority.
//   - ParameterAsIdentifier: the resource has no `id` of its own and is keyed
//     by one of its own arguments. These are the singleton-per-parent
//     resources (one CDC infrastructure per service, one TDE association per
//     service, ...) plus UDFs, which are keyed by function name.
//   - TemplatedStringAsIdentifier: composite key built from several arguments.
var ExternalNameConfigs = map[string]config.ExternalName{
	// ------------------------------------------------------------------
	// ClickHouse Cloud - services
	// ------------------------------------------------------------------

	// Service id is a server-assigned UUID.
	"clickhouse_service": config.IdentifierFromProvider,

	// Singleton sub-resources of a service. None of these expose an `id`;
	// all are documented as importable by the service UUID alone, so the
	// service id *is* the external name.
	"clickhouse_service_private_endpoints_attachment":                config.ParameterAsIdentifier("service_id"),
	"clickhouse_service_transparent_data_encryption_key_association": config.ParameterAsIdentifier("service_id"),
	"clickhouse_clickpipe_cdc_infrastructure":                        config.ParameterAsIdentifier("service_id"),

	// These two do expose a read-only `id` even though they are logically
	// per-service, so let the provider supply it.
	"clickhouse_service_scheduled_scaling": config.IdentifierFromProvider,
	"clickhouse_service_upgrade_window":    config.IdentifierFromProvider,

	// Keyed by the caller-supplied cloud private endpoint id, not by a
	// ClickHouse-assigned id.
	"clickhouse_private_endpoint_registration": config.ParameterAsIdentifier("private_endpoint_id"),

	// ------------------------------------------------------------------
	// ClickHouse Cloud - ClickPipes
	// ------------------------------------------------------------------

	"clickhouse_clickpipe":                                              config.IdentifierFromProvider,
	"clickhouse_clickpipes_reverse_private_endpoint":                    config.IdentifierFromProvider,
	"clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns": config.IdentifierFromProvider,

	// ------------------------------------------------------------------
	// ClickHouse Cloud - organization, access control, Postgres
	// ------------------------------------------------------------------

	// Organization-wide singleton.
	"clickhouse_organization_settings": config.IdentifierFromProvider,

	"clickhouse_role":             config.IdentifierFromProvider,
	"clickhouse_role_assignment":  config.IdentifierFromProvider,
	"clickhouse_postgres_service": config.IdentifierFromProvider,

	// ------------------------------------------------------------------
	// ClickHouse Cloud - user defined functions
	// ------------------------------------------------------------------

	// Imported by function name: `terraform import clickhouse_udf.echo_string echo_string`.
	"clickhouse_udf": config.ParameterAsIdentifier("function_name"),

	// Composite key. Documented import is `<function_name>/<service_id>`,
	// e.g. `echo_string/11111111-1111-1111-1111-111111111111`. The empty
	// name field means the external name is derived wholly from the
	// template rather than mirrored from a single spec field.
	"clickhouse_udf_attachment": config.TemplatedStringAsIdentifier("", "{{ .parameters.function_name }}/{{ .parameters.service_id }}"),

	// ------------------------------------------------------------------
	// ClickStack (HyperDX)
	// ------------------------------------------------------------------
	//
	// Every ClickStack resource exposes a read-only `id` (a Mongo ObjectID
	// such as 507f1f77bcf86cd799439011 on self-hosted deployments), so all
	// of them take the id from the provider.
	//
	// Note on teams: these resources accept an optional `team` argument, and
	// their documented import syntax allows a `<team-id>/<id>` prefix for
	// multi-team (EE) deployments. That prefix is an import-time convenience
	// handled inside the upstream provider; the value stored in `id` remains
	// the bare resource id, and `team` stays a normal spec field. So
	// IdentifierFromProvider is correct here and no template is needed.
	//
	// `team` is immutable on these resources upstream - see the
	// clickstack group configuration where it is marked as forcing
	// replacement.
	"clickhouse_clickstack_alert":        config.IdentifierFromProvider,
	"clickhouse_clickstack_dashboard":    config.IdentifierFromProvider,
	"clickhouse_clickstack_saved_search": config.IdentifierFromProvider,
	"clickhouse_clickstack_source":       config.IdentifierFromProvider,
	"clickhouse_clickstack_webhook":      config.IdentifierFromProvider,

	// Self-hosted ClickStack only. On ClickHouse Cloud, roles/teams/members
	// are managed through clickhouse_role and clickhouse_role_assignment
	// instead, and these endpoints return route-not-found.
	"clickhouse_clickstack_role":        config.IdentifierFromProvider,
	"clickhouse_clickstack_team":        config.IdentifierFromProvider,
	"clickhouse_clickstack_team_member": config.IdentifierFromProvider,

	// Read-only in practice: the platform provisions connections, so an
	// imported connection can be read but not updated or destroyed. Users
	// should pair this with a management policy of ["Observe"].
	"clickhouse_clickstack_connection": config.IdentifierFromProvider,
}

// ExternalNameConfigured returns the list of all resources whose external name
// is configured explicitly.
func ExternalNameConfigured() []string {
	l := make([]string, len(ExternalNameConfigs))
	i := 0
	for name := range ExternalNameConfigs {
		// $ is added to match the exact string, otherwise it will match
		// resources whose names merely start with this one.
		l[i] = name + "$"
		i++
	}
	return l
}

// ExternalNameConfigurations applies all external name configs listed in the
// table ExternalNameConfigs and sets the version of these resources to v1beta1
// assuming they will be tested.
func ExternalNameConfigurations() config.ResourceOption {
	return func(r *config.Resource) {
		if e, ok := ExternalNameConfigs[r.Name]; ok {
			r.ExternalName = e
		}
	}
}
