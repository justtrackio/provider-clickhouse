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
)

// Shared fixtures for the package's tests.
const (
	testSourceName = "Logs"
	testAPIKey     = "key"
	valueTrue      = "true"
)

func TestFindIDByName(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		body    string
		lookup  string
		want    string
		wantErr string
	}{
		"Match": {
			body:   `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Traces"},{"id":"507f1f77bcf86cd799439011","name":"Logs"}]}`,
			lookup: testSourceName,
			want:   "507f1f77bcf86cd799439011",
		},
		"NoMatchIsNotAnError": {
			body:   `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Traces"}]}`,
			lookup: testSourceName,
			want:   "",
		},
		"EmptyCollection": {
			body:   `{"data":[]}`,
			lookup: testSourceName,
			want:   "",
		},
		"CloudEnvelope": {
			// The Cloud OpenAPI wraps results in "result" rather than "data";
			// both are decoded so callers never care which mode they are in.
			body:   `{"status":200,"result":[{"id":"507f1f77bcf86cd799439011","name":"Logs"}]}`,
			lookup: testSourceName,
			want:   "507f1f77bcf86cd799439011",
		},
		"NameIsMatchedExactly": {
			body:   `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"logs"}]}`,
			lookup: testSourceName,
			want:   "",
		},
		"AmbiguousName": {
			body:    `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Logs"},{"id":"bbbbbbbbbbbbbbbbbbbbbbbb","name":"Logs"}]}`,
			lookup:  testSourceName,
			wantErr: "cannot decide which one to adopt",
		},
		"MatchWithoutID": {
			body:    `{"data":[{"name":"Logs"}]}`,
			lookup:  testSourceName,
			wantErr: `has no id`,
		},
		"MalformedBody": {
			body:    `not json`,
			lookup:  testSourceName,
			wantErr: "decode response",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, err := New(srv.URL, testAPIKey, 0)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			got, err := c.FindIDByName(context.Background(), CollectionSources, tc.lookup)
			switch {
			case tc.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
			case tc.wantErr == "" && err != nil:
				t.Fatalf("FindIDByName: %v", err)
			case tc.wantErr == "" && got != tc.want:
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindIDByNameHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c, err := New(srv.URL, testAPIKey, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.FindIDByName(context.Background(), CollectionSources, testSourceName); err == nil ||
		!strings.Contains(err.Error(), "unexpected status 403") {
		t.Fatalf("expected the status to be reported, got: %v", err)
	}
}

// TestSelfHostedRequest pins the wire contract for a self-hosted deployment:
// the /api/v2 path is used verbatim, the API key is a Bearer token, and the team
// is selected with the x-hdx-team header.
func TestSelfHostedRequest(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth, gotTeam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotTeam = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-hdx-team")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "secret-key", 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	scoped, err := c.WithTeam("650000000000000000000001")
	if err != nil {
		t.Fatalf("WithTeam: %v", err)
	}
	if _, err := scoped.FindIDByName(context.Background(), CollectionSavedSearches, "x"); err != nil {
		t.Fatalf("FindIDByName: %v", err)
	}

	if gotPath != CollectionSavedSearches {
		t.Errorf("path = %q, want %q", gotPath, CollectionSavedSearches)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotTeam != "650000000000000000000001" {
		t.Errorf("x-hdx-team = %q", gotTeam)
	}
}

// TestWithTeamLeavesReceiverUnchanged guards the shallow copy: scoping one
// lookup to a team must not leak into the shared client.
func TestWithTeamLeavesReceiverUnchanged(t *testing.T) {
	t.Parallel()

	c, err := New("http://example.invalid", testAPIKey, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.WithTeam("team-a"); err != nil {
		t.Fatalf("WithTeam: %v", err)
	}
	if c.teamID != "" {
		t.Errorf("receiver teamID = %q, want it untouched", c.teamID)
	}
}

// TestCloudRequest pins the cloud contract: the /api/v2 prefix is stripped, the
// path is org/service scoped, and auth is HTTP basic.
func TestCloudRequest(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	c, err := NewCloud(srv.URL, "org-1", "svc-1", "tk", "ts", 0)
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	if _, err := c.FindIDByName(context.Background(), CollectionSources, "x"); err != nil {
		t.Fatalf("FindIDByName: %v", err)
	}

	const wantPath = "/organizations/org-1/services/svc-1/clickstack/sources"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotUser != "tk" || gotPass != "ts" {
		t.Errorf("basic auth = %q/%q, want tk/ts", gotUser, gotPass)
	}
}

// TestCloudRejectsTeamScoping mirrors the upstream client: a Cloud service is a
// single ClickStack team, so scoping must fail loudly rather than silently
// widen the lookup to the wrong team.
func TestCloudRejectsTeamScoping(t *testing.T) {
	t.Parallel()

	c, err := NewCloud("https://api.clickhouse.cloud/v1", "org", "svc", "tk", "ts", 0)
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	if _, err := c.WithTeam("some-team"); err == nil ||
		!strings.Contains(err.Error(), "not supported by ClickStack on ClickHouse Cloud") {
		t.Fatalf("expected the cloud/team conflict to be reported, got: %v", err)
	}
}

func TestEndpointValidation(t *testing.T) {
	t.Parallel()

	if _, err := New("ftp://example.com", testAPIKey, 0); err == nil ||
		!strings.Contains(err.Error(), "must use http or https") {
		t.Errorf("expected a scheme error, got: %v", err)
	}
	if _, err := NewCloud("ftp://example.com", "o", "s", "k", "s", 0); err == nil ||
		!strings.Contains(err.Error(), "must use http or https") {
		t.Errorf("expected a scheme error, got: %v", err)
	}
}

// TestNewCloudDefaultsAPIURL keeps the public Cloud base as the fallback so a
// ProviderConfig that omits api_url still resolves.
func TestNewCloudDefaultsAPIURL(t *testing.T) {
	t.Parallel()

	c, err := NewCloud("", "org", "svc", "k", "s", 0)
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	if !strings.HasPrefix(c.endpoint, defaultCloudAPIURL) {
		t.Errorf("endpoint = %q, want it to start with %q", c.endpoint, defaultCloudAPIURL)
	}
}
