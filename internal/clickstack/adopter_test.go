// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	clusterapis "github.com/justtrackio/provider-clickhouse/apis/cluster"
	clickstackv1alpha1 "github.com/justtrackio/provider-clickhouse/apis/cluster/clickstack/v1alpha1"
	clusterv1beta1 "github.com/justtrackio/provider-clickhouse/apis/cluster/v1beta1"
	"github.com/justtrackio/provider-clickhouse/internal/clickstack"
)

const (
	sourceName = "Logs"
	objectID   = "507f1f77bcf86cd799439011"
)

// stub serves one ClickStack collection and counts how many times it was listed,
// so the tests can assert both the outcome and that no lookup happens when it
// should not.
func stub(t *testing.T, body string, calls *int) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, clickstack.CollectionSources; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		*calls++
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// world builds a cluster-scoped Source plus the ProviderConfig and Secret it
// references, in a fake API server that records writes.
func world(t *testing.T, endpoint string, adopt bool, mutate func(*clickstackv1alpha1.Source)) (client.Client, *clickstackv1alpha1.Source) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clusterapis.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(core): %v", err)
	}

	credentials := `{"` + clickstack.KeyClickStackEndpoint + `":"` + endpoint + `",` +
		`"` + clickstack.KeyClickStackAPIKey + `":"test-key"`
	if adopt {
		credentials += `,"` + clickstack.KeyAdoptByName + `":"true"`
	}
	credentials += `}`

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"credentials": []byte(credentials)},
	}

	pc := &clusterv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "clickstack"},
		Spec: clusterv1beta1.ProviderConfigSpec{
			Credentials: clusterv1beta1.ProviderCredentials{
				Source: xpv2.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv2.CommonCredentialSelectors{
					SecretRef: &xpv2.SecretKeySelector{
						SecretReference: xpv2.SecretReference{Name: "creds", Namespace: "crossplane-system"},
						Key:             "credentials",
					},
				},
			},
		},
	}

	name := sourceName
	source := &clickstackv1alpha1.Source{
		ObjectMeta: metav1.ObjectMeta{Name: "logs"},
		Spec: clickstackv1alpha1.SourceSpec{
			ForProvider: clickstackv1alpha1.SourceParameters{Name: &name},
		},
	}
	source.SetProviderConfigReference(&xpv2.Reference{Name: "clickstack"})
	source.SetGroupVersionKind(clickstackv1alpha1.Source_GroupVersionKind)
	if mutate != nil {
		mutate(source)
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(secret, pc, source).Build()
	return kube, source
}

// TestInitializeAdopts is the point of the whole change: the external name must
// be resolved from the spec name AND persisted, so the lookup happens once.
func TestInitializeAdopts(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Traces"},{"id":"`+objectID+`","name":"Logs"}]}`, &calls)
	kube, source := world(t, endpoint, true, nil)

	if err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if got := meta.GetExternalName(source); got != objectID {
		t.Errorf("external name = %q, want %q", got, objectID)
	}
	if calls != 1 {
		t.Errorf("expected one lookup, got %d", calls)
	}

	// Persisted, not just set in memory. This is what GetIDFn could not do for
	// an Observe-only or non-late-initialized resource.
	persisted := &clickstackv1alpha1.Source{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "logs"}, persisted); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := meta.GetExternalName(persisted); got != objectID {
		t.Errorf("persisted external name = %q, want %q", got, objectID)
	}
}

// TestInitializeIsOptIn is the guard for existing users: without the flag the
// provider must not look anything up, let alone take ownership of it.
func TestInitializeIsOptIn(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[{"id":"`+objectID+`","name":"Logs"}]}`, &calls)
	kube, source := world(t, endpoint, false, nil)

	if err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := meta.GetExternalName(source); got != "" {
		t.Errorf("expected no external name without opt-in, got %q", got)
	}
	if calls != 0 {
		t.Errorf("expected no lookup without opt-in, got %d", calls)
	}
}

// TestInitializeSkipsAdoptedResource keeps the initializer off the hot path once
// the annotation exists.
func TestInitializeSkipsAdoptedResource(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[]}`, &calls)
	kube, source := world(t, endpoint, true, func(s *clickstackv1alpha1.Source) {
		meta.SetExternalName(s, objectID)
	})

	if err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected no lookup for an adopted resource, got %d", calls)
	}
}

// TestInitializeFallsThroughToCreate proves adoption does not obstruct genuinely
// new objects.
func TestInitializeFallsThroughToCreate(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Traces"}]}`, &calls)
	kube, source := world(t, endpoint, true, nil)

	if err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if got := meta.GetExternalName(source); got != "" {
		t.Errorf("expected no external name when nothing matches, got %q", got)
	}
	if calls != 1 {
		t.Errorf("expected one lookup, got %d", calls)
	}
}

// TestInitializeAmbiguousNameFails pins the refusal to guess.
func TestInitializeAmbiguousNameFails(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaa","name":"Logs"},{"id":"bbbbbbbbbbbbbbbbbbbbbbbb","name":"Logs"}]}`, &calls)
	kube, source := world(t, endpoint, true, nil)

	err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source)
	if err == nil || !strings.Contains(err.Error(), "cannot decide which one to adopt") {
		t.Fatalf("expected the ambiguity to be reported, got: %v", err)
	}
	if got := meta.GetExternalName(source); got != "" {
		t.Errorf("expected no external name on an ambiguous match, got %q", got)
	}
}

// TestInitializeLookupFailureBlocks is the safety property: an unreachable or
// unauthorised API must not fall through to a create that duplicates the object
// we were asked to adopt.
func TestInitializeLookupFailureBlocks(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	kube, source := world(t, srv.URL, true, nil)

	err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source)
	if err == nil || !strings.Contains(err.Error(), "adoption") {
		t.Fatalf("expected the lookup failure to block the reconcile, got: %v", err)
	}
	if got := meta.GetExternalName(source); got != "" {
		t.Errorf("expected no external name after a failed lookup, got %q", got)
	}
}

// TestInitializeWithoutName covers a spec that has no name to resolve yet.
func TestInitializeWithoutName(t *testing.T) {
	t.Parallel()

	calls := 0
	endpoint := stub(t, `{"data":[{"id":"`+objectID+`","name":"Logs"}]}`, &calls)
	kube, source := world(t, endpoint, true, func(s *clickstackv1alpha1.Source) {
		s.Spec.ForProvider.Name = nil
	})

	if err := clickstack.NewAdopter(kube, clickstack.CollectionSources).Initialize(context.Background(), source); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected no lookup without a name, got %d", calls)
	}
}

// TestAdoptByNameBuildsInitializer covers the factory upjet wires into the
// generated controllers.
func TestAdoptByNameBuildsInitializer(t *testing.T) {
	t.Parallel()

	if got := clickstack.AdoptByName(clickstack.CollectionSources)(fake.NewClientBuilder().Build()); got == nil {
		t.Error("AdoptByName returned no initializer")
	}
}
