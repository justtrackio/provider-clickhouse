// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/upjet/v2/pkg/terraform"
)

func TestAdoptionEnabled(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup map[string]any
		want  bool
	}{
		"NilSetup":       {setup: nil, want: false},
		"NoMetadata":     {setup: map[string]any{}, want: false},
		"NilMetadata":    {setup: map[string]any{setupKeyClientMetadata: map[string]string(nil)}, want: false},
		"Absent":         {setup: withMeta(map[string]string{}), want: false},
		"Empty":          {setup: withMeta(map[string]string{KeyAdoptByName: ""}), want: false},
		"False":          {setup: withMeta(map[string]string{KeyAdoptByName: "false"}), want: false},
		"Nonsense":       {setup: withMeta(map[string]string{KeyAdoptByName: "maybe"}), want: false},
		"True":           {setup: withMeta(map[string]string{KeyAdoptByName: valueTrue}), want: true},
		"TitleCaseTrue":  {setup: withMeta(map[string]string{KeyAdoptByName: "True"}), want: true},
		"One":            {setup: withMeta(map[string]string{KeyAdoptByName: "1"}), want: true},
		"Yes":            {setup: withMeta(map[string]string{KeyAdoptByName: "yes"}), want: true},
		"WrongMetaShape": {setup: map[string]any{setupKeyClientMetadata: map[string]any{KeyAdoptByName: valueTrue}}, want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := AdoptionEnabled(tc.setup); got != tc.want {
				t.Errorf("AdoptionEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func withMeta(m map[string]string) map[string]any {
	return map[string]any{setupKeyClientMetadata: m}
}

// TestConfigurationAcceptsProviderConfigurationType is the load-bearing test for
// the setup-map plumbing. Upjet's Setup.Map() stores the configuration as a
// terraform.ProviderConfiguration, a named map type, so a plain map[string]any
// assertion silently misses it and adoption would quietly never happen. Build
// the map exactly the way upjet does and assert we still read it.
func TestConfigurationAcceptsProviderConfigurationType(t *testing.T) {
	t.Parallel()

	upjetSetup := terraform.Setup{
		Configuration: terraform.ProviderConfiguration{
			KeyClickStackEndpoint: "http://hyperdx:8000",
			KeyClickStackAPIKey:   testAPIKey,
		},
		ClientMetadata: map[string]string{KeyAdoptByName: valueTrue},
	}.Map()

	cfg := configuration(upjetSetup)
	if got := str(cfg, KeyClickStackEndpoint); got != "http://hyperdx:8000" {
		t.Errorf("endpoint = %q, want it read out of terraform.ProviderConfiguration", got)
	}
	if !AdoptionEnabled(upjetSetup) {
		t.Error("adoption should be enabled from a real upjet setup map")
	}

	client, err := clientFromSetup(upjetSetup)
	if err != nil {
		t.Fatalf("clientFromSetup: %v", err)
	}
	if client == nil || client.cloud {
		t.Fatalf("expected a self-hosted client, got %+v", client)
	}
}

func TestConfigurationEdgeCases(t *testing.T) {
	t.Parallel()

	if got := configuration(nil); got != nil {
		t.Errorf("configuration(nil) = %v, want nil", got)
	}
	if got := configuration(map[string]any{setupKeyConfiguration: "not a map"}); got != nil {
		t.Errorf("configuration(string) = %v, want nil", got)
	}
	if got := configuration(map[string]any{setupKeyConfiguration: map[int]any{1: "x"}}); got != nil {
		t.Errorf("configuration(non-string keys) = %v, want nil", got)
	}
}

func TestClientFromSetup(t *testing.T) {
	t.Parallel()

	t.Run("SelfHosted", func(t *testing.T) {
		t.Parallel()
		c, err := clientFromSetup(map[string]any{setupKeyConfiguration: map[string]any{
			KeyClickStackEndpoint: "http://hyperdx:8000",
			KeyClickStackAPIKey:   testAPIKey,
		}})
		if err != nil {
			t.Fatalf("clientFromSetup: %v", err)
		}
		if c == nil || c.cloud {
			t.Fatalf("expected a self-hosted client, got %+v", c)
		}
	})

	t.Run("Cloud", func(t *testing.T) {
		t.Parallel()
		c, err := clientFromSetup(map[string]any{setupKeyConfiguration: map[string]any{
			KeyClickStackServiceID: "svc",
			KeyOrganizationID:      "org",
			KeyTokenKey:            "tk",
			KeyTokenSecret:         "ts",
		}})
		if err != nil {
			t.Fatalf("clientFromSetup: %v", err)
		}
		if c == nil || !c.cloud {
			t.Fatalf("expected a cloud client, got %+v", c)
		}
	})

	t.Run("NoClickStackCredentials", func(t *testing.T) {
		t.Parallel()
		// A Cloud-only ProviderConfig with no ClickStack surface selected: there
		// is nothing to look up, and the Terraform provider gives a far better
		// error than we could.
		c, err := clientFromSetup(map[string]any{setupKeyConfiguration: map[string]any{
			KeyOrganizationID: "org",
			KeyTokenKey:       "tk",
			KeyTokenSecret:    "ts",
		}})
		if err != nil {
			t.Fatalf("clientFromSetup: %v", err)
		}
		if c != nil {
			t.Errorf("expected no client, got %+v", c)
		}
	})
}

func TestTimeout(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value any
		want  time.Duration
	}{
		"Int64FromClients": {value: int64(5), want: 5 * time.Second},
		"Int":              {value: 7, want: 7 * time.Second},
		"Float64FromJSON":  {value: float64(9), want: 9 * time.Second},
		"StringIsIgnored":  {value: "11", want: 0},
		"Absent":           {value: nil, want: 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := map[string]any{}
			if tc.value != nil {
				cfg[KeyTimeoutSeconds] = tc.value
			}
			if got := timeout(cfg); got != tc.want {
				t.Errorf("timeout = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestResolveIDByNameShortCircuits covers every reason to skip the lookup. Each
// must be a silent no-op rather than an error, because each one means "create
// the object normally".
func TestResolveIDByNameShortCircuits(t *testing.T) {
	t.Parallel()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[{"id":"507f1f77bcf86cd799439011","name":"Logs"}]}`))
	}))
	defer srv.Close()

	enabled := map[string]any{
		setupKeyConfiguration: map[string]any{
			KeyClickStackEndpoint: srv.URL,
			KeyClickStackAPIKey:   testAPIKey,
		},
		setupKeyClientMetadata: map[string]string{KeyAdoptByName: valueTrue},
	}

	cases := map[string]struct {
		setup      map[string]any
		collection string
		name       string
	}{
		"NoCollection": {setup: enabled, collection: "", name: testSourceName},
		"NoName":       {setup: enabled, collection: CollectionSources, name: ""},
		"NotOptedIn": {setup: map[string]any{setupKeyConfiguration: map[string]any{
			KeyClickStackEndpoint: srv.URL,
			KeyClickStackAPIKey:   testAPIKey,
		}}, collection: CollectionSources, name: testSourceName},
		"NoCredentials": {setup: map[string]any{
			setupKeyClientMetadata: map[string]string{KeyAdoptByName: valueTrue},
		}, collection: CollectionSources, name: testSourceName},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			id, err := ResolveIDByName(context.Background(), tc.setup, tc.collection, tc.name, "")
			if err != nil {
				t.Fatalf("ResolveIDByName: %v", err)
			}
			if id != "" {
				t.Errorf("expected no adoption, got %q", id)
			}
		})
	}

	if calls != 0 {
		t.Errorf("expected no API calls, got %d", calls)
	}
}

func TestResolveIDByNameAdopts(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"507f1f77bcf86cd799439011","name":"Logs"}]}`))
	}))
	defer srv.Close()

	setup := map[string]any{
		setupKeyConfiguration: map[string]any{
			KeyClickStackEndpoint: srv.URL,
			KeyClickStackAPIKey:   testAPIKey,
		},
		setupKeyClientMetadata: map[string]string{KeyAdoptByName: valueTrue},
	}

	id, err := ResolveIDByName(context.Background(), setup, CollectionSources, testSourceName, "")
	if err != nil {
		t.Fatalf("ResolveIDByName: %v", err)
	}
	if id != "507f1f77bcf86cd799439011" {
		t.Errorf("got %q, want the existing ObjectID", id)
	}
}

// TestResolveIDByNameCloudTeamConflict makes sure the team check is reached
// through ResolveIDByName, not just on the client.
func TestResolveIDByNameCloudTeamConflict(t *testing.T) {
	t.Parallel()

	setup := map[string]any{
		setupKeyConfiguration: map[string]any{
			KeyClickStackServiceID: "svc",
			KeyOrganizationID:      "org",
			KeyTokenKey:            "tk",
			KeyTokenSecret:         "ts",
		},
		setupKeyClientMetadata: map[string]string{KeyAdoptByName: valueTrue},
	}

	_, err := ResolveIDByName(context.Background(), setup, CollectionSources, testSourceName, "team-1")
	if err == nil || !strings.Contains(err.Error(), "ClickHouse Cloud") {
		t.Fatalf("expected the cloud/team conflict, got: %v", err)
	}
}
