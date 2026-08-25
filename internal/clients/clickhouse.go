package clients

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/upjet/v2/pkg/terraform"

	clusterv1beta1 "github.com/justtrackio/provider-clickhouse/apis/cluster/v1beta1"
	namespacedv1beta1 "github.com/justtrackio/provider-clickhouse/apis/namespaced/v1beta1"
	"github.com/justtrackio/provider-clickhouse/internal/clickstack"
)

const (
	// error messages
	errNoProviderConfig     = "no providerConfigRef provided"
	errGetProviderConfig    = "cannot get referenced ProviderConfig"
	errTrackUsage           = "cannot track ProviderConfig usage"
	errExtractCredentials   = "cannot extract credentials"
	errUnmarshalCredentials = "cannot unmarshal clickhouse credentials as JSON"
)

// Credential keys accepted in the ProviderConfig secret. These mirror the
// provider-level attributes of the ClickHouse Terraform provider one-for-one.
//
// The upstream provider declares every one of these as Optional, because it
// serves three independent surfaces that authenticate differently:
//
//   - ClickHouse Cloud (services, ClickPipes, roles, Postgres, UDFs) uses
//     organizationID + tokenKey + tokenSecret.
//   - ClickStack on ClickHouse Cloud is reached through the Cloud API, so it
//     reuses the Cloud credentials and additionally needs clickstackServiceID.
//   - Self-hosted ClickStack (OSS or EE) has its own credentials entirely:
//     clickstackEndpoint + clickstackAPIKey.
//
// We therefore validate credential *sets* rather than individual keys, and
// deliberately allow a ProviderConfig that carries only one set.
const (
	keyOrganizationID      = clickstack.KeyOrganizationID
	keyTokenKey            = clickstack.KeyTokenKey
	keyTokenSecret         = clickstack.KeyTokenSecret
	keyAPIURL              = clickstack.KeyAPIURL
	keyTimeoutSeconds      = clickstack.KeyTimeoutSeconds
	keyClickStackAPIKey    = clickstack.KeyClickStackAPIKey
	keyClickStackEndpoint  = clickstack.KeyClickStackEndpoint
	keyClickStackServiceID = clickstack.KeyClickStackServiceID
)

// cloudCredentialKeys are the ClickHouse Cloud credentials, which are only
// meaningful as a complete set.
var cloudCredentialKeys = []string{keyOrganizationID, keyTokenKey, keyTokenSecret}

// credentials wraps the raw secret contents with presence-aware lookups, so
// that a key present but blank is treated the same as a key that is absent.
type credentials map[string]string

func (c credentials) has(k string) bool { return strings.TrimSpace(c[k]) != "" }

func (c credentials) get(k string) string { return strings.TrimSpace(c[k]) }

// applyCloud validates and applies the ClickHouse Cloud credential set,
// reporting whether it is present.
func (c credentials) applyCloud(config map[string]any) (bool, error) {
	present := make([]string, 0, len(cloudCredentialKeys))
	missing := make([]string, 0, len(cloudCredentialKeys))
	for _, k := range cloudCredentialKeys {
		if c.has(k) {
			present = append(present, k)
			continue
		}
		missing = append(missing, k)
	}

	switch len(present) {
	case 0:
		return false, nil
	case len(cloudCredentialKeys):
		for _, k := range cloudCredentialKeys {
			config[k] = c.get(k)
		}
		return true, nil
	default:
		return false, errors.Errorf("incomplete ClickHouse Cloud credentials: %s must be provided together, missing %s",
			strings.Join(cloudCredentialKeys, ", "), strings.Join(missing, ", "))
	}
}

// applySelfHostedClickStack validates and applies the self-hosted ClickStack
// credential pair, reporting whether it is present.
func (c credentials) applySelfHostedClickStack(config map[string]any) (bool, error) {
	hasKey, hasEndpoint := c.has(keyClickStackAPIKey), c.has(keyClickStackEndpoint)
	if hasKey != hasEndpoint {
		return false, errors.Errorf("%s and %s must be provided together for self-hosted ClickStack",
			keyClickStackAPIKey, keyClickStackEndpoint)
	}
	if !hasKey {
		return false, nil
	}
	config[keyClickStackAPIKey] = c.get(keyClickStackAPIKey)
	config[keyClickStackEndpoint] = c.get(keyClickStackEndpoint)
	return true, nil
}

// applyManagedClickStack applies clickstack_service_id, which selects ClickStack
// running on ClickHouse Cloud. It is mutually exclusive with the self-hosted
// pair and useless without the Cloud credentials it authenticates with.
func (c credentials) applyManagedClickStack(config map[string]any, hasCloud, hasSelfHosted bool) error {
	if !c.has(keyClickStackServiceID) {
		return nil
	}
	if hasSelfHosted {
		return errors.Errorf("%s is mutually exclusive with %s/%s: managed ClickStack authenticates with the ClickHouse Cloud credentials, self-hosted ClickStack with its own API key",
			keyClickStackServiceID, keyClickStackAPIKey, keyClickStackEndpoint)
	}
	if !hasCloud {
		return errors.Errorf("%s requires the ClickHouse Cloud credentials (%s) because managed ClickStack is served through the Cloud API",
			keyClickStackServiceID, strings.Join(cloudCredentialKeys, ", "))
	}
	config[keyClickStackServiceID] = c.get(keyClickStackServiceID)
	return nil
}

// applyTuning applies the optional, non-credential provider attributes.
func (c credentials) applyTuning(config map[string]any) error {
	if c.has(keyAPIURL) {
		config[keyAPIURL] = c.get(keyAPIURL)
	}
	if !c.has(keyTimeoutSeconds) {
		return nil
	}
	// timeout_seconds is a number in the Terraform schema, so it must not be
	// passed through as the string it arrives as in the JSON secret.
	t, err := strconv.ParseInt(c.get(keyTimeoutSeconds), 10, 64)
	if err != nil {
		return errors.Wrapf(err, "%s must be an integer number of seconds", keyTimeoutSeconds)
	}
	config[keyTimeoutSeconds] = t
	return nil
}

// buildProviderConfiguration translates the credential map extracted from the
// ProviderConfig secret into the Terraform provider configuration, validating
// the combinations the upstream provider accepts.
//
// Validating here rather than deferring to Terraform is deliberate: a missing
// or contradictory credential set otherwise surfaces as an opaque failure on
// every managed resource's first reconcile, instead of once against the
// ProviderConfig that is actually misconfigured.
func buildProviderConfiguration(raw map[string]string) (map[string]any, error) {
	creds := credentials(raw)
	config := map[string]any{}

	hasCloud, err := creds.applyCloud(config)
	if err != nil {
		return nil, err
	}

	hasSelfHosted, err := creds.applySelfHostedClickStack(config)
	if err != nil {
		return nil, err
	}

	if err := creds.applyManagedClickStack(config, hasCloud, hasSelfHosted); err != nil {
		return nil, err
	}

	if !hasCloud && !hasSelfHosted {
		return nil, errors.Errorf("no usable credentials found: provide either the ClickHouse Cloud set (%s) or the self-hosted ClickStack set (%s, %s)",
			strings.Join(cloudCredentialKeys, ", "), keyClickStackEndpoint, keyClickStackAPIKey)
	}

	if err := creds.applyTuning(config); err != nil {
		return nil, err
	}

	return config, nil
}

// TerraformSetupBuilder builds Terraform a terraform.SetupFn function which
// returns Terraform provider setup configuration
func TerraformSetupBuilder(version, providerSource, providerVersion string) terraform.SetupFn {
	return func(ctx context.Context, client client.Client, mg resource.Managed) (terraform.Setup, error) {
		ps := terraform.Setup{
			Version: version,
			Requirement: terraform.ProviderRequirement{
				Source:  providerSource,
				Version: providerVersion,
			},
		}

		pcSpec, err := resolveProviderConfig(ctx, client, mg)
		if err != nil {
			return terraform.Setup{}, errors.Wrap(err, "cannot resolve provider config")
		}

		data, err := resource.CommonCredentialExtractor(ctx, pcSpec.Credentials.Source, client, pcSpec.Credentials.CommonCredentialSelectors)
		if err != nil {
			return ps, errors.Wrap(err, errExtractCredentials)
		}
		creds := map[string]string{}
		if err := json.Unmarshal(data, &creds); err != nil {
			return ps, errors.Wrap(err, errUnmarshalCredentials)
		}

		config, err := buildProviderConfiguration(creds)
		if err != nil {
			return ps, err
		}
		ps.Configuration = config

		return ps, nil
	}
}

func toSharedPCSpec(pc *clusterv1beta1.ProviderConfig) (*namespacedv1beta1.ProviderConfigSpec, error) {
	if pc == nil {
		return nil, nil
	}
	data, err := json.Marshal(pc.Spec)
	if err != nil {
		return nil, err
	}

	var mSpec namespacedv1beta1.ProviderConfigSpec
	err = json.Unmarshal(data, &mSpec)
	return &mSpec, err
}

func resolveProviderConfig(ctx context.Context, crClient client.Client, mg resource.Managed) (*namespacedv1beta1.ProviderConfigSpec, error) {
	switch managed := mg.(type) {
	case resource.LegacyManaged: //nolint:staticcheck // still handling cluster-scoped behavior
		return resolveLegacy(ctx, crClient, managed)
	case resource.ModernManaged:
		return resolveModern(ctx, crClient, managed)
	default:
		return nil, errors.New("resource is not a managed resource")
	}
}

func resolveLegacy(ctx context.Context, client client.Client, mg resource.LegacyManaged) (*namespacedv1beta1.ProviderConfigSpec, error) { //nolint:staticcheck // still handling cluster-scoped behavior
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}
	pc := &clusterv1beta1.ProviderConfig{}
	if err := client.Get(ctx, types.NamespacedName{Name: configRef.Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	t := resource.NewLegacyProviderConfigUsageTracker(client, &clusterv1beta1.ProviderConfigUsage{})
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	return toSharedPCSpec(pc)
}

func resolveModern(ctx context.Context, crClient client.Client, mg resource.ModernManaged) (*namespacedv1beta1.ProviderConfigSpec, error) {
	configRef := mg.GetProviderConfigReference()
	if configRef == nil {
		return nil, errors.New(errNoProviderConfig)
	}

	pcRuntimeObj, err := crClient.Scheme().New(namespacedv1beta1.SchemeGroupVersion.WithKind(configRef.Kind))
	if err != nil {
		return nil, errors.Wrap(err, "unknown GVK for ProviderConfig")
	}
	pcObj, ok := pcRuntimeObj.(client.Object)
	if !ok {
		// This indicates a programming error, types are not properly generated
		return nil, errors.New(" is not an Object")
	}

	// Namespace will be ignored if the PC is a cluster-scoped type
	if err := crClient.Get(ctx, types.NamespacedName{Name: configRef.Name, Namespace: mg.GetNamespace()}, pcObj); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	var pcSpec namespacedv1beta1.ProviderConfigSpec
	pcu := &namespacedv1beta1.ProviderConfigUsage{}
	switch pc := pcObj.(type) {
	case *namespacedv1beta1.ProviderConfig:
		pcSpec = pc.Spec
		if pcSpec.Credentials.SecretRef != nil {
			pcSpec.Credentials.SecretRef.Namespace = mg.GetNamespace()
		}
	case *namespacedv1beta1.ClusterProviderConfig:
		pcSpec = pc.Spec
	default:
		return nil, errors.New("unknown provider config type")
	}
	t := resource.NewProviderConfigUsageTracker(crClient, pcu)
	if err := t.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}
	return &pcSpec, nil
}
