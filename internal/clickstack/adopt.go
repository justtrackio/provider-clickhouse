// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"strconv"
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

	// KeyAdoptByName opts a ProviderConfig in to name-based adoption. It is not
	// an attribute of the upstream Terraform provider and is never forwarded to
	// it: an unknown provider attribute would fail every invocation.
	KeyAdoptByName = "clickstack_adopt_by_name"
)

// Credentials is the raw ProviderConfig secret, with presence-aware lookups so
// that a key present but blank reads the same as a key that is absent.
type Credentials map[string]string

// Get returns the trimmed value for key.
func (c Credentials) Get(key string) string { return strings.TrimSpace(c[key]) }

// Has reports whether key has a non-blank value.
func (c Credentials) Has(key string) bool { return c.Get(key) != "" }

// AdoptionEnabled reports whether this ProviderConfig opted in to name-based
// adoption.
//
// Adoption is opt-in because it changes what applying a resource means: with it
// on, a resource whose name already exists takes ownership of that object
// instead of creating its own. That is what you want when importing an estate
// that was built by hand, and emphatically not what you want by default.
func (c Credentials) AdoptionEnabled() bool {
	switch strings.ToLower(c.Get(KeyAdoptByName)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

// NewFromCredentials builds a ClickStack client from a ProviderConfig secret. It
// returns a nil client, and no error, when the secret carries no ClickStack
// credentials: any ClickStack resource using it will fail later with a much
// clearer message from the Terraform provider itself.
//
// The self-hosted pair wins over the managed one. internal/clients already
// rejects a ProviderConfig supplying both, so at most one is present here.
func NewFromCredentials(c Credentials) (*Client, error) {
	timeout := c.timeout()

	if endpoint := c.Get(KeyClickStackEndpoint); endpoint != "" {
		return New(endpoint, c.Get(KeyClickStackAPIKey), timeout)
	}

	serviceID := c.Get(KeyClickStackServiceID)
	if serviceID == "" {
		return nil, nil
	}
	return NewCloud(
		c.Get(KeyAPIURL),
		c.Get(KeyOrganizationID),
		serviceID,
		c.Get(KeyTokenKey),
		c.Get(KeyTokenSecret),
		timeout,
	)
}

// timeout honours the ProviderConfig's timeout_seconds so a lookup is bounded by
// the same budget as the Terraform calls. A malformed value is ignored rather
// than reported: it is validated where the Terraform configuration is built, and
// failing adoption over it would be a confusing place to surface it.
func (c Credentials) timeout() time.Duration {
	if !c.Has(KeyTimeoutSeconds) {
		return 0
	}
	seconds, err := strconv.ParseInt(c.Get(KeyTimeoutSeconds), 10, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
