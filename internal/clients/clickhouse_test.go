package clients

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	testOrgID      = "org-1"
	testTokenKey   = "key-1"
	testTokenSec   = "secret-1"
	testCSEndpoint = "http://hyperdx:8000"
	testCSAPIKey   = "api-key-1"
	testCSService  = "svc-1"
)

func TestBuildProviderConfiguration(t *testing.T) {
	cloud := map[string]string{
		keyOrganizationID: testOrgID,
		keyTokenKey:       testTokenKey,
		keyTokenSecret:    testTokenSec,
	}
	selfHosted := map[string]string{
		keyClickStackEndpoint: testCSEndpoint,
		keyClickStackAPIKey:   testCSAPIKey,
	}
	merge := func(ms ...map[string]string) map[string]string {
		out := map[string]string{}
		for _, m := range ms {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}

	cases := map[string]struct {
		reason  string
		creds   map[string]string
		want    map[string]any
		wantErr bool
	}{
		"CloudOnly": {
			reason: "The three ClickHouse Cloud credentials alone are a valid configuration.",
			creds:  cloud,
			want: map[string]any{
				keyOrganizationID: testOrgID,
				keyTokenKey:       testTokenKey,
				keyTokenSecret:    testTokenSec,
			},
		},
		"SelfHostedClickStackOnly": {
			reason: "Self-hosted ClickStack credentials alone are valid; Cloud resources simply cannot be used.",
			creds:  selfHosted,
			want: map[string]any{
				keyClickStackEndpoint: testCSEndpoint,
				keyClickStackAPIKey:   testCSAPIKey,
			},
		},
		"CloudAndSelfHostedClickStack": {
			reason: "Both independent credential sets may be supplied together.",
			creds:  merge(cloud, selfHosted),
			want: map[string]any{
				keyOrganizationID:     testOrgID,
				keyTokenKey:           testTokenKey,
				keyTokenSecret:        testTokenSec,
				keyClickStackEndpoint: testCSEndpoint,
				keyClickStackAPIKey:   testCSAPIKey,
			},
		},
		"ManagedClickStackWithCloud": {
			reason: "clickstack_service_id is valid alongside the Cloud credentials it authenticates with.",
			creds:  merge(cloud, map[string]string{keyClickStackServiceID: testCSService}),
			want: map[string]any{
				keyOrganizationID:      testOrgID,
				keyTokenKey:            testTokenKey,
				keyTokenSecret:         testTokenSec,
				keyClickStackServiceID: testCSService,
			},
		},
		"OptionalTuning": {
			reason: "api_url passes through and timeout_seconds is converted to a number.",
			creds:  merge(cloud, map[string]string{keyAPIURL: "https://api.example.com", keyTimeoutSeconds: "45"}),
			want: map[string]any{
				keyOrganizationID: testOrgID,
				keyTokenKey:       testTokenKey,
				keyTokenSecret:    testTokenSec,
				keyAPIURL:         "https://api.example.com",
				keyTimeoutSeconds: int64(45),
			},
		},
		"BlankValuesTreatedAsAbsent": {
			reason: "Whitespace-only values must not count as provided.",
			creds:  merge(selfHosted, map[string]string{"organization_id": "   "}),
			want: map[string]any{
				keyClickStackEndpoint: testCSEndpoint,
				keyClickStackAPIKey:   testCSAPIKey,
			},
		},
		"PartialCloudCredentials": {
			reason:  "An incomplete Cloud set is a configuration error, not a silently ignored one.",
			creds:   map[string]string{keyOrganizationID: testOrgID, keyTokenKey: testTokenKey},
			wantErr: true,
		},
		"ClickStackEndpointWithoutAPIKey": {
			reason:  "The self-hosted ClickStack pair must be provided together.",
			creds:   map[string]string{keyClickStackEndpoint: testCSEndpoint},
			wantErr: true,
		},
		"ClickStackAPIKeyWithoutEndpoint": {
			reason:  "The self-hosted ClickStack pair must be provided together.",
			creds:   map[string]string{keyClickStackAPIKey: testCSAPIKey},
			wantErr: true,
		},
		"ManagedAndSelfHostedClickStackAreMutuallyExclusive": {
			reason:  "clickstack_service_id cannot be combined with the self-hosted pair.",
			creds:   merge(cloud, selfHosted, map[string]string{keyClickStackServiceID: testCSService}),
			wantErr: true,
		},
		"ManagedClickStackWithoutCloudCredentials": {
			reason:  "Managed ClickStack is served through the Cloud API, so it needs the Cloud credentials.",
			creds:   map[string]string{keyClickStackServiceID: testCSService},
			wantErr: true,
		},
		"NoCredentials": {
			reason:  "An empty secret must fail loudly against the ProviderConfig.",
			creds:   map[string]string{},
			wantErr: true,
		},
		"NonNumericTimeout": {
			reason:  "timeout_seconds must be an integer because the Terraform schema types it as a number.",
			creds:   merge(cloud, map[string]string{keyTimeoutSeconds: "thirty"}),
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := buildProviderConfiguration(tc.creds)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("%s\nbuildProviderConfiguration(...): want error, got nil (config=%v)", tc.reason, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s\nbuildProviderConfiguration(...): unexpected error: %v", tc.reason, err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("%s\nbuildProviderConfiguration(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

// TestBuildProviderConfigurationOmitsUnsetKeys guards against sending empty
// strings to Terraform for attributes the user did not configure. The upstream
// provider distinguishes unset from empty, and an empty organization_id would
// otherwise look like a deliberate override.
func TestBuildProviderConfigurationOmitsUnsetKeys(t *testing.T) {
	got, err := buildProviderConfiguration(map[string]string{
		keyClickStackEndpoint: testCSEndpoint,
		keyClickStackAPIKey:   testCSAPIKey,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range []string{keyOrganizationID, keyTokenKey, keyTokenSecret, keyAPIURL, keyTimeoutSeconds, keyClickStackServiceID} {
		if _, ok := got[k]; ok {
			t.Errorf("key %q must be omitted when not configured, but it was present with value %v", k, got[k])
		}
	}
}
