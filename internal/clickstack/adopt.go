// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// ProviderConfig keys read from the credentials secret. They mirror the
// provider-level attributes of the ClickHouse Terraform provider, and are
// declared here because this package is the one that knows what a ClickStack
// credential is; internal/clients references them so the names cannot drift.
const (
	KeyOrganizationID      = "organization_id"
	KeyTokenKey            = "token_key"
	KeyTokenSecret         = "token_secret"
	KeyAPIURL              = "api_url"
	KeyTimeoutSeconds      = "timeout_seconds"
	KeyClickStackAPIKey    = "clickstack_api_key"
	KeyClickStackEndpoint  = "clickstack_endpoint"
	KeyClickStackServiceID = "clickstack_service_id"

	// KeyAdoptByName opts a ProviderConfig in to name-based adoption. It is
	// carried in Setup.ClientMetadata rather than Setup.Configuration because
	// it is not an attribute of the upstream Terraform provider, and passing an
	// unknown attribute would make every Terraform invocation fail.
	KeyAdoptByName = "clickstack_adopt_by_name"
)

// Keys upjet uses in terraform.Setup.Map(), which is what GetIDFn receives as
// its terraformProviderConfig argument.
const (
	setupKeyConfiguration  = "configuration"
	setupKeyClientMetadata = "client_metadata"
)

// AdoptionEnabled reports whether the ProviderConfig behind this Terraform
// setup opted in to name-based adoption.
//
// Adoption is opt-in because it changes what applying a resource means: with it
// on, a resource whose name already exists takes ownership of that object
// instead of creating its own. That is what you want when importing an estate
// that was built by hand, and emphatically not what you want by default.
func AdoptionEnabled(setup map[string]any) bool {
	meta, _ := setup[setupKeyClientMetadata].(map[string]string)
	switch strings.ToLower(strings.TrimSpace(meta[KeyAdoptByName])) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

// ResolveIDByName returns the ObjectID of the ClickStack object called name, or
// an empty string when there is nothing to adopt.
//
// An empty result is returned - without an error - whenever adoption cannot or
// should not happen: the ProviderConfig has not opted in, the resource kind has
// no name-addressable collection, the resource has no name yet, or no object
// carries that name. Every one of those cases means "go ahead and create".
//
// Failures that are not an absence are returned as errors, so that a ClickStack
// API that is unreachable or unauthorised blocks the reconcile instead of
// quietly falling through to a create that would duplicate the very object we
// were asked to adopt.
func ResolveIDByName(ctx context.Context, setup map[string]any, collection, name, team string) (string, error) {
	if collection == "" || name == "" || !AdoptionEnabled(setup) {
		return "", nil
	}

	client, err := clientFromSetup(setup)
	if err != nil {
		return "", err
	}
	if client == nil {
		// No ClickStack credentials in this ProviderConfig. Any ClickStack
		// resource using it will fail later with a much clearer message from
		// the Terraform provider itself, so stay quiet here.
		return "", nil
	}

	client, err = client.WithTeam(team)
	if err != nil {
		return "", err
	}

	id, err := client.FindIDByName(ctx, collection, name)
	if err != nil {
		return "", fmt.Errorf("cannot look up %q by name for adoption: %w", name, err)
	}
	return id, nil
}

// clientFromSetup builds a ClickStack client from the Terraform provider
// configuration upjet hands to GetIDFn. It returns a nil client when the
// ProviderConfig carries no ClickStack credentials at all.
//
// The self-hosted pair wins over the managed one: internal/clients already
// rejects a ProviderConfig that supplies both, so at most one is present here.
func clientFromSetup(setup map[string]any) (*Client, error) {
	cfg := configuration(setup)
	timeout := timeout(cfg)

	if endpoint := str(cfg, KeyClickStackEndpoint); endpoint != "" {
		return New(endpoint, str(cfg, KeyClickStackAPIKey), timeout)
	}

	serviceID := str(cfg, KeyClickStackServiceID)
	if serviceID == "" {
		return nil, nil
	}
	return NewCloud(
		str(cfg, KeyAPIURL),
		str(cfg, KeyOrganizationID),
		serviceID,
		str(cfg, KeyTokenKey),
		str(cfg, KeyTokenSecret),
		timeout,
	)
}

// configuration extracts the provider configuration from upjet's setup map.
//
// The value is a terraform.ProviderConfiguration, a named map type, so a plain
// map[string]any assertion does not match it. Importing the terraform package
// here to name that type would drag it into the generator binary through
// config; reflecting over the map keeps this package free of that dependency
// and also tolerates upjet switching to a plain map.
func configuration(setup map[string]any) map[string]any {
	raw, ok := setup[setupKeyConfiguration]
	if !ok {
		return nil
	}
	if m, ok := raw.(map[string]any); ok {
		return m
	}

	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil
	}
	out := make(map[string]any, rv.Len())
	for iter := rv.MapRange(); iter.Next(); {
		out[iter.Key().String()] = iter.Value().Interface()
	}
	return out
}

func str(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return s
}

// timeout honours the ProviderConfig's timeout_seconds so a lookup is bounded
// by the same budget as the Terraform calls. internal/clients has already
// converted it to an integer, but accept the string form too in case it reaches
// us straight from the secret.
func timeout(cfg map[string]any) time.Duration {
	switch v := cfg[KeyTimeoutSeconds].(type) {
	case int64:
		return time.Duration(v) * time.Second
	case int:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	default:
		return 0
	}
}
