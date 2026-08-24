package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crossplane/upjet/v2/pkg/config"

	"github.com/justtrackio/provider-clickhouse/internal/clickstack"
)

// paramName is the forProvider field clickStackIdentifier resolves a name from.
const paramName = "name"

// TestClickStackIdentifier_EmptyExternalName pins the behaviour the ClickStack
// resources depend on: before a resource exists, the Terraform id must be an
// unused ObjectID rather than an empty string, so that upjet's seeded state
// refreshes against a 404 instead of the collection endpoint.
func TestClickStackIdentifier_EmptyExternalName(t *testing.T) {
	t.Parallel()

	id, err := clickStackIdentifier("").GetIDFn(context.Background(), "", nil, nil)
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	if id != unusedObjectID {
		t.Errorf("expected %q for an empty external name, got %q", unusedObjectID, id)
	}
	if id == "" {
		t.Error("an empty id would make the provider refresh the collection endpoint")
	}
}

// TestClickStackIdentifier_ExistingExternalName proves the configuration is
// transparent once the resource has an id: it must behave exactly like
// config.IdentifierFromProvider.
func TestClickStackIdentifier_ExistingExternalName(t *testing.T) {
	t.Parallel()

	const objectID = "507f1f77bcf86cd799439011"

	got, err := clickStackIdentifier("").GetIDFn(context.Background(), objectID, nil, nil)
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	want, err := config.IdentifierFromProvider.GetIDFn(context.Background(), objectID, nil, nil)
	if err != nil {
		t.Fatalf("IdentifierFromProvider.GetIDFn: %v", err)
	}
	if got != want {
		t.Errorf("expected the same id as IdentifierFromProvider (%q), got %q", want, got)
	}
}

// TestUnusedObjectIDIsWellFormed guards the placeholder itself: the API
// validates ids as Mongo ObjectIDs and answers a well-formed but absent id with
// 404. A malformed value would be rejected as a request-validation error
// instead, which the client does not treat as not-found.
func TestUnusedObjectIDIsWellFormed(t *testing.T) {
	t.Parallel()

	if len(unusedObjectID) != 24 {
		t.Errorf("an ObjectID is 24 hex characters, got %d", len(unusedObjectID))
	}
	if strings.Trim(unusedObjectID, "0123456789abcdef") != "" {
		t.Errorf("%q is not lowercase hex", unusedObjectID)
	}
}

// TestClickStackResourcesUseClickStackIdentifier makes sure a newly added
// ClickStack resource cannot silently fall back to the plain
// IdentifierFromProvider configuration, which would reintroduce the wedge.
func TestClickStackResourcesUseClickStackIdentifier(t *testing.T) {
	t.Parallel()

	for name, cfg := range ExternalNameConfigs {
		if !strings.HasPrefix(name, "clickhouse_clickstack_") {
			continue
		}
		id, err := cfg.GetIDFn(context.Background(), "", nil, nil)
		if err != nil {
			t.Fatalf("%s: GetIDFn: %v", name, err)
		}
		if id != unusedObjectID {
			t.Errorf("%s: expected clickStackIdentifier (id %q for an empty external name), got %q",
				name, unusedObjectID, id)
		}
	}
}

// stubClickStack serves one ClickStack collection and records how many times it
// was listed, so the tests can assert both the adoption result and that a
// lookup only happens when it should.
func stubClickStack(t *testing.T, collection string, body string, calls *int) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != collection {
			t.Errorf("unexpected path %q, want %q", r.URL.Path, collection)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got, want := r.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Errorf("Authorization header = %q, want %q", got, want)
		}
		*calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setup builds the map upjet passes to GetIDFn as terraformProviderConfig.
func setup(endpoint string, adopt bool) map[string]any {
	meta := map[string]string{}
	if adopt {
		meta[clickstack.KeyAdoptByName] = "true"
	}
	return map[string]any{
		"configuration": map[string]any{
			clickstack.KeyClickStackEndpoint: endpoint,
			clickstack.KeyClickStackAPIKey:   "test-key",
		},
		"client_metadata": meta,
	}
}

// TestClickStackIdentifier_AdoptionIsOptIn is the guard that matters most for
// existing users: without clickstack_adopt_by_name the provider must keep
// creating, never silently take over an object that happens to share a name.
func TestClickStackIdentifier_AdoptionIsOptIn(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stubClickStack(t, clickstack.CollectionSources,
		`{"data":[{"id":"507f1f77bcf86cd799439011","name":"Logs"}]}`, &calls)

	id, err := clickStackIdentifier(clickstack.CollectionSources).
		GetIDFn(context.Background(), "", map[string]any{paramName: "Logs"}, setup(endpoint, false))
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	if id != unusedObjectID {
		t.Errorf("expected %q without opt-in, got %q", unusedObjectID, id)
	}
	if calls != 0 {
		t.Errorf("expected no API call without opt-in, got %d", calls)
	}
}

// TestClickStackIdentifier_AdoptsExistingObject covers the feature itself.
func TestClickStackIdentifier_AdoptsExistingObject(t *testing.T) {
	t.Parallel()

	const want = "507f1f77bcf86cd799439011"
	calls := 0
	endpoint := stubClickStack(t, clickstack.CollectionSources,
		`{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Traces"},{"id":"`+want+`","name":"Logs"}]}`, &calls)

	id, err := clickStackIdentifier(clickstack.CollectionSources).
		GetIDFn(context.Background(), "", map[string]any{paramName: "Logs"}, setup(endpoint, true))
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	if id != want {
		t.Errorf("expected the existing id %q, got %q", want, id)
	}
	if calls != 1 {
		t.Errorf("expected exactly one API call, got %d", calls)
	}
}

// TestClickStackIdentifier_AdoptionFallsThroughToCreate proves adoption does not
// get in the way of creating genuinely new objects.
func TestClickStackIdentifier_AdoptionFallsThroughToCreate(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stubClickStack(t, clickstack.CollectionSavedSearches, `{"data":[]}`, &calls)

	id, err := clickStackIdentifier(clickstack.CollectionSavedSearches).
		GetIDFn(context.Background(), "", map[string]any{paramName: "brand new"}, setup(endpoint, true))
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	if id != unusedObjectID {
		t.Errorf("expected %q when nothing matches, got %q", unusedObjectID, id)
	}
}

// TestClickStackIdentifier_AmbiguousNameIsAnError pins the refusal to guess:
// ClickStack does not enforce unique names, and picking one of several
// same-named objects would make ownership depend on API ordering.
func TestClickStackIdentifier_AmbiguousNameIsAnError(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stubClickStack(t, clickstack.CollectionConnections,
		`{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Local ClickHouse"},{"id":"bbbbbbbbbbbbbbbbbbbbbbbb","name":"Local ClickHouse"}]}`, &calls)

	_, err := clickStackIdentifier(clickstack.CollectionConnections).
		GetIDFn(context.Background(), "", map[string]any{paramName: "Local ClickHouse"}, setup(endpoint, true))
	if err == nil {
		t.Fatal("expected an error for two objects sharing a name")
	}
	if !strings.Contains(err.Error(), "cannot decide which one to adopt") {
		t.Errorf("error should explain the ambiguity, got: %v", err)
	}
}

// TestClickStackIdentifier_LookupFailureIsAnError makes sure an unreachable or
// unauthorised API blocks the reconcile instead of falling through to a create
// that would duplicate the object we were asked to adopt.
func TestClickStackIdentifier_LookupFailureIsAnError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	_, err := clickStackIdentifier(clickstack.CollectionSources).
		GetIDFn(context.Background(), "", map[string]any{paramName: "Logs"}, setup(srv.URL, true))
	if err == nil {
		t.Fatal("expected an error when the lookup fails")
	}
	if !strings.Contains(err.Error(), "adoption") {
		t.Errorf("error should mention adoption, got: %v", err)
	}
}

// TestClickStackIdentifier_AdoptedResourceSkipsLookup shows the lookup is a
// one-off: once Crossplane has persisted the external name, GetIDFn is a pure
// passthrough again and never touches the API.
func TestClickStackIdentifier_AdoptedResourceSkipsLookup(t *testing.T) {
	t.Parallel()

	const objectID = "507f1f77bcf86cd799439011"
	calls := 0
	endpoint := stubClickStack(t, clickstack.CollectionSources, `{"data":[]}`, &calls)

	id, err := clickStackIdentifier(clickstack.CollectionSources).
		GetIDFn(context.Background(), objectID, map[string]any{paramName: "Logs"}, setup(endpoint, true))
	if err != nil {
		t.Fatalf("GetIDFn: %v", err)
	}
	if id != objectID {
		t.Errorf("expected the external name %q to pass through, got %q", objectID, id)
	}
	if calls != 0 {
		t.Errorf("expected no API call for an adopted resource, got %d", calls)
	}
}

// TestClickStackAdoptableCollections documents which kinds opted in to
// name-based adoption, so that adding a kind is a deliberate decision reviewed
// here rather than an accident.
func TestClickStackAdoptableCollections(t *testing.T) {
	t.Parallel()

	adoptable := map[string]string{
		"clickhouse_clickstack_connection":   clickstack.CollectionConnections,
		"clickhouse_clickstack_source":       clickstack.CollectionSources,
		"clickhouse_clickstack_saved_search": clickstack.CollectionSavedSearches,
		"clickhouse_clickstack_dashboard":    clickstack.CollectionDashboards,
	}

	for name, cfg := range ExternalNameConfigs {
		if !strings.HasPrefix(name, "clickhouse_clickstack_") {
			continue
		}

		collection, wantAdoptable := adoptable[name]
		calls := 0
		endpoint := stubClickStack(t, collection, `{"data":[{"id":"507f1f77bcf86cd799439011","name":"probe"}]}`, &calls)

		id, err := cfg.GetIDFn(context.Background(), "", map[string]any{paramName: "probe"}, setup(endpoint, true))
		if err != nil {
			t.Fatalf("%s: GetIDFn: %v", name, err)
		}

		if wantAdoptable {
			if id != "507f1f77bcf86cd799439011" {
				t.Errorf("%s: expected adoption of the existing object, got %q", name, id)
			}
			continue
		}
		if id != unusedObjectID {
			t.Errorf("%s: expected no adoption (%q), got %q", name, unusedObjectID, id)
		}
	}
}
