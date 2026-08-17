// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	cdcinfrastructure "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickpipe/cdcinfrastructure"
	clickpipe "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickpipe/clickpipe"
	reverseprivateendpoint "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickpipe/reverseprivateendpoint"
	reverseprivateendpointcustomprivatedns "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickpipe/reverseprivateendpointcustomprivatedns"
	alert "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/alert"
	connection "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/connection"
	dashboard "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/dashboard"
	role "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/role"
	savedsearch "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/savedsearch"
	source "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/source"
	team "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/team"
	teammember "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/teammember"
	webhook "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/clickstack/webhook"
	roleiam "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/iam/role"
	roleassignment "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/iam/roleassignment"
	settings "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/organization/settings"
	service "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/postgres/service"
	providerconfig "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/providerconfig"
	privateendpointregistration "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/privateendpointregistration"
	privateendpointsattachment "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/privateendpointsattachment"
	scheduledscaling "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/scheduledscaling"
	serviceservice "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/service"
	transparentdataencryptionkeyassociation "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/transparentdataencryptionkeyassociation"
	upgradewindow "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/service/upgradewindow"
	attachment "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/udf/attachment"
	function "github.com/justtrackio/provider-clickhouse/internal/controller/namespaced/udf/function"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		cdcinfrastructure.Setup,
		clickpipe.Setup,
		reverseprivateendpoint.Setup,
		reverseprivateendpointcustomprivatedns.Setup,
		alert.Setup,
		connection.Setup,
		dashboard.Setup,
		role.Setup,
		savedsearch.Setup,
		source.Setup,
		team.Setup,
		teammember.Setup,
		webhook.Setup,
		roleiam.Setup,
		roleassignment.Setup,
		settings.Setup,
		service.Setup,
		providerconfig.Setup,
		privateendpointregistration.Setup,
		privateendpointsattachment.Setup,
		scheduledscaling.Setup,
		serviceservice.Setup,
		transparentdataencryptionkeyassociation.Setup,
		upgradewindow.Setup,
		attachment.Setup,
		function.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		cdcinfrastructure.SetupGated,
		clickpipe.SetupGated,
		reverseprivateendpoint.SetupGated,
		reverseprivateendpointcustomprivatedns.SetupGated,
		alert.SetupGated,
		connection.SetupGated,
		dashboard.SetupGated,
		role.SetupGated,
		savedsearch.SetupGated,
		source.SetupGated,
		team.SetupGated,
		teammember.SetupGated,
		webhook.SetupGated,
		roleiam.SetupGated,
		roleassignment.SetupGated,
		settings.SetupGated,
		service.SetupGated,
		providerconfig.SetupGated,
		privateendpointregistration.SetupGated,
		privateendpointsattachment.SetupGated,
		scheduledscaling.SetupGated,
		serviceservice.SetupGated,
		transparentdataencryptionkeyassociation.SetupGated,
		upgradewindow.SetupGated,
		attachment.SetupGated,
		function.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupWebhookWithManager registers conversion webhooks for all resource kinds in the group.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, setup := range []func(ctrl.Manager) error{
		cdcinfrastructure.SetupWebhookWithManager,
		clickpipe.SetupWebhookWithManager,
		reverseprivateendpoint.SetupWebhookWithManager,
		reverseprivateendpointcustomprivatedns.SetupWebhookWithManager,
		alert.SetupWebhookWithManager,
		connection.SetupWebhookWithManager,
		dashboard.SetupWebhookWithManager,
		role.SetupWebhookWithManager,
		savedsearch.SetupWebhookWithManager,
		source.SetupWebhookWithManager,
		team.SetupWebhookWithManager,
		teammember.SetupWebhookWithManager,
		webhook.SetupWebhookWithManager,
		roleiam.SetupWebhookWithManager,
		roleassignment.SetupWebhookWithManager,
		settings.SetupWebhookWithManager,
		service.SetupWebhookWithManager,
		providerconfig.SetupWebhookWithManager,
		privateendpointregistration.SetupWebhookWithManager,
		privateendpointsattachment.SetupWebhookWithManager,
		scheduledscaling.SetupWebhookWithManager,
		serviceservice.SetupWebhookWithManager,
		transparentdataencryptionkeyassociation.SetupWebhookWithManager,
		upgradewindow.SetupWebhookWithManager,
		attachment.SetupWebhookWithManager,
		function.SetupWebhookWithManager,
	} {
		if err := setup(mgr); err != nil {
			return err
		}
	}
	return nil
}
