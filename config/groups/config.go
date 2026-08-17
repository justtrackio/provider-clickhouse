// Package groups holds the per-resource Upjet configuration for the ClickHouse
// provider: API grouping, Kubernetes kind names and cross-resource references.
//
// This package is intentionally shared by both the cluster-scoped and the
// namespaced provider configurations. That is possible because every reference
// below is declared with config.Reference.TerraformName rather than the
// deprecated Type field: TerraformName is resolved through the provider's
// TerraformTypeMapper, so Upjet derives the correct Go type for whichever
// scope and API version it is generating. Using Type here would force this
// file to be duplicated once per scope with hardcoded import paths.
package groups

import (
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
)

// Terraform resource names, kept as constants so a typo becomes a compile
// error rather than a silently skipped configuration.
const (
	// Cloud - services
	resService                = "clickhouse_service"
	resServicePEAttachment    = "clickhouse_service_private_endpoints_attachment"
	resServiceScheduledScale  = "clickhouse_service_scheduled_scaling"
	resServiceUpgradeWindow   = "clickhouse_service_upgrade_window"
	resServiceTDEKeyAssoc     = "clickhouse_service_transparent_data_encryption_key_association"
	resPrivateEndpointRegistr = "clickhouse_private_endpoint_registration"

	// Cloud - ClickPipes
	resClickPipe         = "clickhouse_clickpipe"
	resClickPipeCDCInfra = "clickhouse_clickpipe_cdc_infrastructure"
	resClickPipesRPE     = "clickhouse_clickpipes_reverse_private_endpoint"
	resClickPipesRPEDNS  = "clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns"

	// Cloud - access control, organization, Postgres
	resRole            = "clickhouse_role"
	resRoleAssignment  = "clickhouse_role_assignment"
	resOrgSettings     = "clickhouse_organization_settings"
	resPostgresService = "clickhouse_postgres_service"

	// Cloud - user defined functions
	resUDF           = "clickhouse_udf"
	resUDFAttachment = "clickhouse_udf_attachment"

	// ClickStack
	resCSAlert       = "clickhouse_clickstack_alert"
	resCSConnection  = "clickhouse_clickstack_connection"
	resCSDashboard   = "clickhouse_clickstack_dashboard"
	resCSRole        = "clickhouse_clickstack_role"
	resCSSavedSearch = "clickhouse_clickstack_saved_search"
	resCSSource      = "clickhouse_clickstack_source"
	resCSTeam        = "clickhouse_clickstack_team"
	resCSTeamMember  = "clickhouse_clickstack_team_member"
	resCSWebhook     = "clickhouse_clickstack_webhook"
)

// API short groups. Without these, Upjet would place every resource in a
// single "clickhouse" group, which makes for an unnavigable API surface once
// all 25 resources are generated.
const (
	groupService      = "service"
	groupClickPipe    = "clickpipe"
	groupIAM          = "iam"
	groupOrganization = "organization"
	groupPostgres     = "postgres"
	groupUDF          = "udf"
	groupClickStack   = "clickstack"
)

// serviceIDRef is the reference every per-service resource uses to point at
// the ClickHouse service it belongs to. The default extractor resolves the
// referenced object's external name, which for clickhouse_service is the
// service UUID - exactly what service_id expects.
func serviceIDRef() ujconfig.Reference {
	return ujconfig.Reference{TerraformName: resService}
}

// Configure registers all resource configurators for this provider.
func Configure(p *ujconfig.Provider) {
	configureServices(p)
	configureClickPipes(p)
	configureIAM(p)
	configureOrganization(p)
	configurePostgres(p)
	configureUDFs(p)
	configureClickStack(p)
}

func configureServices(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resService, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "Service"
	})

	p.AddResourceConfigurator(resServicePEAttachment, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "PrivateEndpointsAttachment"
		r.References["service_id"] = serviceIDRef()
		r.References["private_endpoint_ids"] = ujconfig.Reference{
			TerraformName: resPrivateEndpointRegistr,
		}
	})

	p.AddResourceConfigurator(resServiceScheduledScale, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "ScheduledScaling"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resServiceUpgradeWindow, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "UpgradeWindow"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resServiceTDEKeyAssoc, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "TransparentDataEncryptionKeyAssociation"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resPrivateEndpointRegistr, func(r *ujconfig.Resource) {
		r.ShortGroup = groupService
		r.Kind = "PrivateEndpointRegistration"
	})
}

func configureClickPipes(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resClickPipe, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickPipe
		r.Kind = "ClickPipe"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resClickPipeCDCInfra, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickPipe
		r.Kind = "CDCInfrastructure"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resClickPipesRPE, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickPipe
		r.Kind = "ReversePrivateEndpoint"
		r.References["service_id"] = serviceIDRef()
	})

	p.AddResourceConfigurator(resClickPipesRPEDNS, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickPipe
		r.Kind = "ReversePrivateEndpointCustomPrivateDNS"
		r.References["service_id"] = serviceIDRef()
		r.References["reverse_private_endpoint_id"] = ujconfig.Reference{
			TerraformName: resClickPipesRPE,
		}
	})
}

func configureIAM(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resRole, func(r *ujconfig.Resource) {
		r.ShortGroup = groupIAM
		r.Kind = "Role"
	})

	p.AddResourceConfigurator(resRoleAssignment, func(r *ujconfig.Resource) {
		r.ShortGroup = groupIAM
		r.Kind = "RoleAssignment"
		r.References["role_id"] = ujconfig.Reference{TerraformName: resRole}
	})
}

func configureOrganization(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resOrgSettings, func(r *ujconfig.Resource) {
		r.ShortGroup = groupOrganization
		r.Kind = "Settings"
	})
}

func configurePostgres(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resPostgresService, func(r *ujconfig.Resource) {
		r.ShortGroup = groupPostgres
		r.Kind = "Service"
	})
}

func configureUDFs(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resUDF, func(r *ujconfig.Resource) {
		r.ShortGroup = groupUDF
		r.Kind = "Function"
	})

	p.AddResourceConfigurator(resUDFAttachment, func(r *ujconfig.Resource) {
		r.ShortGroup = groupUDF
		r.Kind = "Attachment"
		r.References["service_id"] = serviceIDRef()
		// clickhouse_udf's external name is its function_name, so the
		// default external-name extractor yields the right value here.
		r.References["function_name"] = ujconfig.Reference{TerraformName: resUDF}
	})
}

// teamRef points at a ClickStack team. Only meaningful on self-hosted
// (multi-team / EE) deployments: on ClickHouse Cloud a service *is* a single
// ClickStack team and the upstream provider rejects the team argument.
func teamRef() ujconfig.Reference {
	return ujconfig.Reference{TerraformName: resCSTeam}
}

func configureClickStack(p *ujconfig.Provider) {
	p.AddResourceConfigurator(resCSConnection, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Connection"
		// Connections are provisioned by the platform and cannot be
		// updated or destroyed through the API. Users should manage
		// these with spec.managementPolicies: ["Observe"].
	})

	p.AddResourceConfigurator(resCSSource, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Source"
		r.References["connection_id"] = ujconfig.Reference{TerraformName: resCSConnection}
		r.References["team"] = teamRef()
	})

	p.AddResourceConfigurator(resCSSavedSearch, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "SavedSearch"
		r.References["source_id"] = ujconfig.Reference{TerraformName: resCSSource}
		r.References["team"] = teamRef()
	})

	p.AddResourceConfigurator(resCSAlert, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Alert"
		r.References["saved_search_id"] = ujconfig.Reference{TerraformName: resCSSavedSearch}
		// webhook_id lives inside the single-nested `channel` attribute,
		// not at the top level. Alerts have no dashboard_id: a tile alert
		// is expressed inside clickhouse_clickstack_dashboard's
		// dashboard_json instead.
		r.References["channel.webhook_id"] = ujconfig.Reference{TerraformName: resCSWebhook}
		r.References["team"] = teamRef()
	})

	p.AddResourceConfigurator(resCSDashboard, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Dashboard"
		r.References["team"] = teamRef()
	})

	p.AddResourceConfigurator(resCSWebhook, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Webhook"
		r.References["team"] = teamRef()
		// url, body, headers and query_params are all marked Sensitive in
		// the upstream schema, so Upjet generates them as secret
		// references and keeps them out of status.atProvider.
	})

	p.AddResourceConfigurator(resCSRole, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Role"
	})

	p.AddResourceConfigurator(resCSTeam, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "Team"
		r.References["default_user_role_id"] = ujconfig.Reference{TerraformName: resCSRole}
	})

	p.AddResourceConfigurator(resCSTeamMember, func(r *ujconfig.Resource) {
		r.ShortGroup = groupClickStack
		r.Kind = "TeamMember"
		r.References["team"] = teamRef()
		r.References["role_id"] = ujconfig.Reference{TerraformName: resCSRole}
	})
}
