// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"context"
	"encoding/json"
	"strings"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
)

// providerConfigVersion is the API version of every ProviderConfig kind this
// provider serves. The group is derived from the managed resource itself rather
// than hardcoded; see providerConfigGVK.
const providerConfigVersion = "v1beta1"

// Adopter resolves a ClickStack object's server-assigned ObjectID from the name
// in its spec and writes it to crossplane.io/external-name, so that a manifest
// takes ownership of an object that already exists instead of creating a second
// one beside it.
//
// ClickStack objects are identified only by that ObjectID: there is no
// name-based lookup upstream and the documented `terraform import` syntax is the
// bare id. So without adoption, pointing a manifest at an estate built in the UI
// - or at the Connection and default Sources the platform pre-provisions -
// duplicates every object it describes, invisibly, because the manifest looks
// correct and only the ClickStack sidebar shows the second copy.
//
// This is a managed.Initializer and not part of the ExternalName configuration,
// which is a correctness requirement rather than a style choice. Resolving the
// id in ExternalName.GetIDFn would appear to work and then silently repeat for
// ever, because the annotation would only ever be persisted as a side effect of
// something else: crossplane-runtime writes the managed resource in exactly two
// places, the create path and the late-initialization path (see
// pkg/reconciler/managed/reconciler.go). An adopted resource already exists, so
// the create path never runs; and a resource with
// spec.managementPolicies: ["Observe"] - the policy clickstack.Connection is
// documented to need - does not late-initialize either. Neither write happens,
// the annotation never lands, and every reconcile lists the collection again. An
// initializer runs before Observe and persists the annotation itself, so the
// lookup really does happen once.
type Adopter struct {
	kube client.Client

	// collection is the API path listed to resolve a name, e.g.
	// CollectionSources. Kinds whose collections are not addressable by name get
	// no Adopter at all.
	collection string
}

// NewAdopter returns an Adopter for the given ClickStack collection.
func NewAdopter(kube client.Client, collection string) *Adopter {
	return &Adopter{kube: kube, collection: collection}
}

// AdoptByName returns the initializer factory that config.Resource.InitializerFns
// expects, which the generated controllers invoke with the manager's client.
func AdoptByName(collection string) func(client.Client) managed.Initializer {
	return func(kube client.Client) managed.Initializer {
		return NewAdopter(kube, collection)
	}
}

// Initialize adopts the existing object named in mg's spec, if there is one.
//
// It is a no-op - not an error - whenever adoption cannot or should not happen:
// the resource already has an external name, the ProviderConfig has not opted
// in, it carries no ClickStack credentials, the spec has no name yet, or no
// object carries that name. Every one of those means "go ahead and create".
//
// Failures that are not an absence are returned. A ClickStack API that is
// unreachable or unauthorised blocks the reconcile rather than falling through
// to a create that would duplicate the very object we were asked to adopt.
func (a *Adopter) Initialize(ctx context.Context, mg resource.Managed) error {
	if meta.GetExternalName(mg) != "" {
		return nil
	}

	creds, err := a.credentials(ctx, mg)
	if err != nil {
		return err
	}
	if !creds.AdoptionEnabled() {
		return nil
	}

	name, team := nameAndTeam(mg)
	if name == "" {
		return nil
	}

	api, err := NewFromCredentials(creds)
	if err != nil {
		return errors.Wrap(err, "cannot build a ClickStack client for adoption")
	}
	if api == nil {
		// No ClickStack credentials in this ProviderConfig. The Terraform
		// provider gives a far better error for that than we could.
		return nil
	}
	if api, err = api.WithTeam(team); err != nil {
		return err
	}

	id, err := api.FindIDByName(ctx, a.collection, name)
	if err != nil {
		return errors.Wrapf(err, "cannot look up %q by name for adoption", name)
	}
	if id == "" {
		return nil
	}

	meta.SetExternalName(mg, id)

	return errors.Wrap(a.kube.Update(ctx, mg), "cannot persist the adopted external name")
}

// providerCredentials mirrors the shape of every ProviderConfig's
// spec.credentials. It is decoded from the unstructured ProviderConfig rather
// than from this provider's own API types on purpose: this package is reachable
// from config/groups, which the code generator loads, and the generator must not
// depend on the types it is about to generate.
type providerCredentials struct {
	Source xpv2.CredentialsSource `json:"source"`

	xpv2.CommonCredentialSelectors `json:",inline"`
}

// credentials resolves the ProviderConfig referenced by mg and extracts its
// secret, so that adoption authenticates exactly as the resource it is adopting
// for. Tracking ProviderConfig usage is left to the Terraform setup path, which
// runs on the same resource moments later.
func (a *Adopter) credentials(ctx context.Context, mg resource.Managed) (Credentials, error) {
	gvk, key, err := providerConfigRef(mg)
	if err != nil {
		return nil, err
	}

	pc := &unstructured.Unstructured{}
	pc.SetGroupVersionKind(gvk)
	if err := a.kube.Get(ctx, key, pc); err != nil {
		return nil, errors.Wrap(err, "cannot get referenced ProviderConfig")
	}

	raw, found, err := unstructured.NestedMap(pc.Object, "spec", "credentials")
	if err != nil || !found {
		return nil, errors.Wrap(err, "cannot read ProviderConfig credentials")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.Wrap(err, "cannot re-encode ProviderConfig credentials")
	}
	var selectors providerCredentials
	if err := json.Unmarshal(encoded, &selectors); err != nil {
		return nil, errors.Wrap(err, "cannot decode ProviderConfig credentials")
	}

	data, err := resource.CommonCredentialExtractor(ctx, selectors.Source, a.kube, selectors.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract credentials")
	}

	creds := Credentials{}
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal clickhouse credentials as JSON")
	}
	return creds, nil
}

// providerConfigRef returns the GVK and object key of the ProviderConfig mg
// references, for both the cluster-scoped (legacy) and namespaced (Crossplane v2)
// flavours of the API.
func providerConfigRef(mg resource.Managed) (schema.GroupVersionKind, types.NamespacedName, error) {
	switch managedResource := mg.(type) {
	case resource.LegacyManaged: //nolint:staticcheck // still handling cluster-scoped behavior
		ref := managedResource.GetProviderConfigReference()
		if ref == nil {
			return schema.GroupVersionKind{}, types.NamespacedName{}, errors.New("no providerConfigRef provided")
		}
		// Cluster-scoped ProviderConfigs have a single kind and no namespace.
		return providerConfigGVK(mg, "ProviderConfig"), types.NamespacedName{Name: ref.Name}, nil

	case resource.ModernManaged:
		ref := managedResource.GetProviderConfigReference()
		if ref == nil {
			return schema.GroupVersionKind{}, types.NamespacedName{}, errors.New("no providerConfigRef provided")
		}
		key := types.NamespacedName{Name: ref.Name}
		// ClusterProviderConfig is cluster-scoped; ProviderConfig lives in the
		// managed resource's namespace. A namespace on a cluster-scoped Get is
		// ignored, but being explicit keeps the intent readable.
		if ref.Kind != "ClusterProviderConfig" {
			key.Namespace = mg.GetNamespace()
		}
		return providerConfigGVK(mg, ref.Kind), key, nil

	default:
		return schema.GroupVersionKind{}, types.NamespacedName{}, errors.New("resource is not a managed resource")
	}
}

// providerConfigGVK derives the ProviderConfig group from the managed resource's
// own group by dropping its leading label: a Source in
// clickstack.clickhouse.justtrack.io is configured by a ProviderConfig in
// clickhouse.justtrack.io, and the namespaced clickstack.clickhouse.m.justtrack.io
// by clickhouse.m.justtrack.io. Deriving it keeps both API scopes working from
// one code path without hardcoding either group.
func providerConfigGVK(mg resource.Managed, kind string) schema.GroupVersionKind {
	group := mg.GetObjectKind().GroupVersionKind().Group
	if _, parent, found := strings.Cut(group, "."); found {
		group = parent
	}
	return schema.GroupVersionKind{Group: group, Version: providerConfigVersion, Kind: kind}
}

// nameAndTeam reads spec.forProvider.name and spec.forProvider.team.
//
// The Adopter is shared by every adoptable kind, so the read goes through the
// unstructured form rather than a typed accessor. It runs once per resource,
// before the first Observe, so the conversion is not on a hot path.
func nameAndTeam(mg resource.Managed) (name, team string) {
	converted, err := runtime.DefaultUnstructuredConverter.ToUnstructured(mg)
	if err != nil {
		return "", ""
	}
	name, _, _ = unstructured.NestedString(converted, "spec", "forProvider", "name")
	team, _, _ = unstructured.NestedString(converted, "spec", "forProvider", "team")
	return name, team
}
