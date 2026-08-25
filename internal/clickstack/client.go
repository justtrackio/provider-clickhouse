// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

// Package clickstack implements the small, read-only slice of the ClickStack
// (HyperDX) HTTP API that this provider needs outside of Terraform: resolving
// an object's server-assigned ObjectID from the name it carries in the UI, so
// that a pre-existing object can be adopted instead of duplicated.
//
// The upstream Terraform provider already has a complete ClickStack client, but
// it has lived under internal/ since v3.19.0 and Go forbids importing it. That
// is the same packaging constraint that forces Terraform CLI execution mode
// (see the README), so the lookup is reimplemented here rather than reused. It
// is deliberately kept to listing a collection: nothing in this package writes.
package clickstack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Collection paths on the self-hosted API, mirroring the upstream client's
// constants. Cloud mode serves the same collections with the /api/v2 prefix
// stripped, which do() handles.
//
// Only the collections that are worth adopting by name are listed. Alerts are
// keyed by the saved search they evaluate rather than by a name; webhooks have
// no GET-by-id and only a paginated list; team is a settings singleton and team
// members are keyed by email. None of them fit a name lookup, so they keep the
// plain create-only behaviour.
const (
	CollectionConnections   = "/api/v2/connections"
	CollectionSources       = "/api/v2/sources"
	CollectionSavedSearches = "/api/v2/saved-searches"
	CollectionDashboards    = "/api/v2/dashboards"
)

// defaultCloudAPIURL is the ClickHouse Cloud OpenAPI base used when the
// ProviderConfig does not override api_url. It matches the upstream provider's
// default.
const defaultCloudAPIURL = "https://api.clickhouse.cloud/v1"

// defaultTimeout bounds a lookup so that an unreachable ClickStack API fails
// the reconcile promptly instead of holding a Terraform workspace open.
const defaultTimeout = 30 * time.Second

// Object is the subset of a ClickStack object this package needs: the
// server-assigned ObjectID and the human-readable name that identifies it in
// the UI. Every ClickStack collection returns both.
type Object struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listEnvelope is the API's success envelope for a collection. Self-hosted
// ClickStack answers with {"data": [...]}; the Cloud OpenAPI wraps the same
// payload as {"result": [...]}. Decoding both keeps do() free of mode-specific
// unwrapping.
type listEnvelope struct {
	Data   []Object `json:"data"`
	Result []Object `json:"result"`
}

func (e listEnvelope) objects() []Object {
	if e.Data != nil {
		return e.Data
	}
	return e.Result
}

// Client lists ClickStack collections. It speaks either to a self-hosted
// ClickStack API (Bearer API key, /api/v2 paths) or to ClickStack managed by
// ClickHouse Cloud through the Cloud OpenAPI (HTTP basic auth with a Cloud API
// key, org/service-scoped paths).
type Client struct {
	httpClient *http.Client
	endpoint   string

	// apiKey authenticates against a self-hosted deployment.
	apiKey string
	// teamID, when non-empty, is sent as x-hdx-team to select the active team.
	// Only multi-team (EE) deployments honour it; single-team deployments
	// ignore it. Empty means "use the API key's team".
	teamID string

	// cloud mode credentials.
	cloud       bool
	tokenKey    string
	tokenSecret string
}

// New returns a Client for a self-hosted ClickStack API. The endpoint is the
// base URL without the /api/v2 suffix, e.g. "http://hyperdx.clickstack:8000".
func New(endpoint, apiKey string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("endpoint %q must use http or https", endpoint)
	}
	return &Client{
		httpClient: &http.Client{Timeout: orDefaultTimeout(timeout)},
		endpoint:   strings.TrimRight(endpoint, "/"),
		apiKey:     apiKey,
	}, nil
}

// NewCloud returns a Client for ClickStack managed by ClickHouse Cloud, served
// at {apiURL}/organizations/{organizationID}/services/{serviceID}/clickstack.
// An empty apiURL falls back to the public Cloud OpenAPI base.
func NewCloud(apiURL, organizationID, serviceID, tokenKey, tokenSecret string, timeout time.Duration) (*Client, error) {
	if apiURL == "" {
		apiURL = defaultCloudAPIURL
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse api url %q: %w", apiURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("api url %q must use http or https", apiURL)
	}
	return &Client{
		httpClient: &http.Client{Timeout: orDefaultTimeout(timeout)},
		endpoint: strings.TrimRight(apiURL, "/") +
			"/organizations/" + url.PathEscape(organizationID) +
			"/services/" + url.PathEscape(serviceID) + "/clickstack",
		cloud:       true,
		tokenKey:    tokenKey,
		tokenSecret: tokenSecret,
	}, nil
}

func orDefaultTimeout(t time.Duration) time.Duration {
	if t <= 0 {
		return defaultTimeout
	}
	return t
}

// WithTeam returns a shallow copy scoped to teamID. An empty teamID leaves team
// selection to the server. Teams are a self-hosted concept, so scoping a cloud
// client is rejected rather than silently ignored.
func (c *Client) WithTeam(teamID string) (*Client, error) {
	if teamID == "" || teamID == c.teamID {
		return c, nil
	}
	if c.cloud {
		return nil, fmt.Errorf("team %q: teams are not supported by ClickStack on ClickHouse Cloud, where a service is a single team", teamID)
	}
	clone := *c
	clone.teamID = teamID
	return &clone, nil
}

// FindIDByName lists a collection and returns the ObjectID of the single object
// called name. It returns an empty id and no error when nothing matches, so the
// caller can fall through to creating the object.
//
// More than one match is an error rather than an arbitrary pick: ClickStack
// does not constrain names to be unique, and silently adopting one of several
// same-named objects would make ownership depend on API ordering.
func (c *Client) FindIDByName(ctx context.Context, collection, name string) (string, error) {
	objects, err := c.list(ctx, collection)
	if err != nil {
		return "", err
	}

	matches := make([]Object, 0, 1)
	for _, o := range objects {
		if o.Name == name {
			matches = append(matches, o)
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		if matches[0].ID == "" {
			return "", fmt.Errorf("%s: object named %q has no id", collection, name)
		}
		return matches[0].ID, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return "", fmt.Errorf("%s: %d objects are named %q (%s): cannot decide which one to adopt, set crossplane.io/external-name explicitly",
			collection, len(matches), name, strings.Join(ids, ", "))
	}
}

func (c *Client) list(ctx context.Context, collection string) ([]Object, error) {
	reqPath := collection
	if c.cloud {
		reqPath = strings.TrimPrefix(collection, "/api/v2")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+reqPath, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.cloud {
		req.SetBasicAuth(c.tokenKey, c.tokenSecret)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if c.teamID != "" {
			req.Header.Set("x-hdx-team", c.teamID)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", collection, err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing actionable on close failure

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: unexpected status %d", collection, resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: read response: %w", collection, err)
	}

	var env listEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("GET %s: decode response: %w", collection, err)
	}
	return env.objects(), nil
}
