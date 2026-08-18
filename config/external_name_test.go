package config

import (
	"context"
	"strings"
	"testing"

	"github.com/crossplane/upjet/v2/pkg/config"
)

// TestClickStackIdentifier_EmptyExternalName pins the behaviour the ClickStack
// resources depend on: before a resource exists, the Terraform id must be an
// unused ObjectID rather than an empty string, so that upjet's seeded state
// refreshes against a 404 instead of the collection endpoint.
func TestClickStackIdentifier_EmptyExternalName(t *testing.T) {
	t.Parallel()

	id, err := clickStackIdentifier().GetIDFn(context.Background(), "", nil, nil)
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

	got, err := clickStackIdentifier().GetIDFn(context.Background(), objectID, nil, nil)
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
