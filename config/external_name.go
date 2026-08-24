package config

import (
	"context"

	"github.com/crossplane/upjet/v2/pkg/config"

	"github.com/justtrackio/provider-clickhouse/internal/clickstack"
)

// unusedObjectID is a syntactically valid Mongo ObjectID that no ClickStack
// object can have: ObjectIDs embed a creation timestamp in their first four
// bytes, so an all-zero value would mean "created at the Unix epoch". The API
// still accepts it as a well-formed id and answers a lookup with 404, which is
// exactly what clickStackIdentifier needs below.
const unusedObjectID = "000000000000000000000000"

// clickStackIdentifier is config.IdentifierFromProvider for ClickStack
// resources, with a single deviation: a resource that has not been created yet
// gets unusedObjectID as its Terraform id instead of an empty string.
//
// Upjet seeds terraform.tfstate for every managed resource before it runs
// Terraform, taking the state's `id` from GetIDFn (see upjet
// pkg/terraform/store.go, which calls FileProducer.EnsureTFState with the
// result of GetIDFn; EnsureTFState writes the entry unconditionally and has no
// empty-id guard). A resource that does not exist yet has no external name, so
// with plain IdentifierFromProvider the seeded id is "" and the refresh that
// follows reads an empty id.
//
// That is where the ClickStack API surface turns a harmless no-op into a
// deadlock. The upstream client builds "<collection>/<id>", which for an empty
// id collapses to the collection path with a trailing slash, and HyperDX
// serves that as the collection itself - HTTP 200 with a JSON array - instead
// of 404. Decoding an array into the single-object response envelope then
// fails:
//
//	Error Reading Saved Search: decode saved search: json: cannot unmarshal
//	array into Go struct field savedSearchEnvelope.data of type
//	client.SavedSearch
//
// Observe never succeeds, so Crossplane never advances to Create, so an
// external name is never assigned and the next reconcile repeats the same
// refresh. Handing Terraform an id that cannot exist makes that refresh a
// clean 404, which the client maps to its not-found error; Terraform drops the
// resource from state and the resource is created normally.
//
// Once created, behaviour is identical to IdentifierFromProvider: the external
// name is read back from the state id, and every later refresh uses the real
// ObjectID.
//
// The collection argument enables the second deviation, name-based adoption.
// When it is non-empty and the ProviderConfig sets clickstack_adopt_by_name, a
// resource that has no external name yet first asks the ClickStack API whether
// an object of that name already exists, and adopts its ObjectID instead of
// falling through to a create. This exists because ClickStack objects are
// identified only by their server-assigned ObjectID: there is no name-based
// lookup upstream, and the documented terraform import syntax is the bare id.
// Without adoption, pointing a manifest at an estate that was built in the UI
// duplicates every object it describes. Pass an empty collection for kinds that
// have no name-addressable collection; they keep the create-only behaviour.
//
// A resource that has been adopted is indistinguishable from one that was
// created here: Terraform refreshes the returned id, upjet reads it back through
// IDAsExternalName, and Crossplane persists it as crossplane.io/external-name.
// The lookup therefore happens at most once per resource, and never again once
// the annotation exists.
func clickStackIdentifier(collection string) config.ExternalName {
	e := config.IdentifierFromProvider
	e.GetIDFn = func(ctx context.Context, externalName string, parameters map[string]any, terraformProviderConfig map[string]any) (string, error) {
		if externalName != "" {
			return config.ExternalNameAsID(ctx, externalName, parameters, terraformProviderConfig)
		}

		name, _ := parameters["name"].(string)
		team, _ := parameters["team"].(string)
		id, err := clickstack.ResolveIDByName(ctx, terraformProviderConfig, collection, name, team)
		if err != nil {
			return "", err
		}
		if id != "" {
			return id, nil
		}
		return unusedObjectID, nil
	}
	return e
}

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
	// of them take the id from the provider. They use
	// clickStackIdentifier rather than config.IdentifierFromProvider
	// directly so that a not-yet-created resource refreshes against an
	// unused id instead of an empty one - see clickStackIdentifier for why
	// an empty id wedges these resources.
	//
	// Note on teams: these resources accept an optional `team` argument, and
	// their documented import syntax allows a `<team-id>/<id>` prefix for
	// multi-team (EE) deployments. That prefix is an import-time convenience
	// handled inside the upstream provider; the value stored in `id` remains
	// the bare resource id, and `team` stays a normal spec field. So
	// taking the identifier from the provider is correct here and no
	// template is needed.
	//
	// `team` is immutable on these resources upstream - see the
	// clickstack group configuration where it is marked as forcing
	// replacement.
	// Adoptable by name where the collection is name-addressable, so that a
	// manifest can take ownership of an object that already exists instead of
	// creating a duplicate. See clickStackIdentifier.
	//
	// Alerts and webhooks pass no collection: an alert is keyed by the saved
	// search it evaluates rather than by a name, and the v2 webhooks endpoint
	// offers only a paginated list with no GET-by-id.
	"clickhouse_clickstack_alert":        clickStackIdentifier(""),
	"clickhouse_clickstack_dashboard":    clickStackIdentifier(clickstack.CollectionDashboards),
	"clickhouse_clickstack_saved_search": clickStackIdentifier(clickstack.CollectionSavedSearches),
	"clickhouse_clickstack_source":       clickStackIdentifier(clickstack.CollectionSources),
	"clickhouse_clickstack_webhook":      clickStackIdentifier(""),

	// Self-hosted ClickStack only. On ClickHouse Cloud, roles/teams/members
	// are managed through clickhouse_role and clickhouse_role_assignment
	// instead, and these endpoints return route-not-found.
	//
	// None is name-adoptable: /api/v2/team is a settings singleton rather than a
	// collection, team members are keyed by email, and roles are rarely
	// pre-existing.
	"clickhouse_clickstack_role":        clickStackIdentifier(""),
	"clickhouse_clickstack_team":        clickStackIdentifier(""),
	"clickhouse_clickstack_team_member": clickStackIdentifier(""),

	// Read-only in practice: the platform provisions connections, so an
	// imported connection can be read but not updated or destroyed. Users
	// should pair this with a management policy of ["Observe"], which needs an
	// external name to observe - so this is the kind that benefits most from
	// name-based adoption.
	"clickhouse_clickstack_connection": clickStackIdentifier(clickstack.CollectionConnections),
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
