// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package clickstack

import (
	"testing"
	"time"
)

func TestAdoptionEnabled(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		creds Credentials
		want  bool
	}{
		"Nil":           {creds: nil, want: false},
		"Absent":        {creds: Credentials{}, want: false},
		"Empty":         {creds: Credentials{KeyAdoptByName: ""}, want: false},
		"Blank":         {creds: Credentials{KeyAdoptByName: "   "}, want: false},
		"False":         {creds: Credentials{KeyAdoptByName: "false"}, want: false},
		"Nonsense":      {creds: Credentials{KeyAdoptByName: "maybe"}, want: false},
		"True":          {creds: Credentials{KeyAdoptByName: valueTrue}, want: true},
		"TitleCaseTrue": {creds: Credentials{KeyAdoptByName: "True"}, want: true},
		"UpperTrue":     {creds: Credentials{KeyAdoptByName: "TRUE"}, want: true},
		"Padded":        {creds: Credentials{KeyAdoptByName: " true "}, want: true},
		"One":           {creds: Credentials{KeyAdoptByName: "1"}, want: true},
		"Yes":           {creds: Credentials{KeyAdoptByName: "yes"}, want: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.creds.AdoptionEnabled(); got != tc.want {
				t.Errorf("AdoptionEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewFromCredentials(t *testing.T) {
	t.Parallel()

	t.Run("SelfHosted", func(t *testing.T) {
		t.Parallel()
		c, err := NewFromCredentials(Credentials{
			KeyClickStackEndpoint: "http://hyperdx:8000",
			KeyClickStackAPIKey:   testAPIKey,
		})
		if err != nil {
			t.Fatalf("NewFromCredentials: %v", err)
		}
		if c == nil || c.cloud {
			t.Fatalf("expected a self-hosted client, got %+v", c)
		}
	})

	t.Run("Cloud", func(t *testing.T) {
		t.Parallel()
		c, err := NewFromCredentials(Credentials{
			KeyClickStackServiceID: "svc",
			KeyOrganizationID:      "org",
			KeyTokenKey:            "tk",
			KeyTokenSecret:         "ts",
		})
		if err != nil {
			t.Fatalf("NewFromCredentials: %v", err)
		}
		if c == nil || !c.cloud {
			t.Fatalf("expected a cloud client, got %+v", c)
		}
	})

	t.Run("SelfHostedWinsOverCloud", func(t *testing.T) {
		t.Parallel()
		// internal/clients rejects a ProviderConfig supplying both, so this only
		// pins the tie-break rather than blessing the combination.
		c, err := NewFromCredentials(Credentials{
			KeyClickStackEndpoint:  "http://hyperdx:8000",
			KeyClickStackAPIKey:    testAPIKey,
			KeyClickStackServiceID: "svc",
		})
		if err != nil {
			t.Fatalf("NewFromCredentials: %v", err)
		}
		if c == nil || c.cloud {
			t.Fatalf("expected the self-hosted client to win, got %+v", c)
		}
	})

	t.Run("NoClickStackCredentials", func(t *testing.T) {
		t.Parallel()
		// A Cloud-only ProviderConfig with no ClickStack surface selected: there
		// is nothing to look up, and the Terraform provider reports that far
		// better than we could.
		c, err := NewFromCredentials(Credentials{
			KeyOrganizationID: "org",
			KeyTokenKey:       "tk",
			KeyTokenSecret:    "ts",
		})
		if err != nil {
			t.Fatalf("NewFromCredentials: %v", err)
		}
		if c != nil {
			t.Errorf("expected no client, got %+v", c)
		}
	})

	t.Run("InvalidEndpoint", func(t *testing.T) {
		t.Parallel()
		if _, err := NewFromCredentials(Credentials{
			KeyClickStackEndpoint: "ftp://nope",
			KeyClickStackAPIKey:   testAPIKey,
		}); err == nil {
			t.Error("expected the endpoint scheme to be rejected")
		}
	})
}

func TestCredentialsTimeout(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		value string
		want  time.Duration
	}{
		"Absent":   {value: "", want: 0},
		"Seconds":  {value: "5", want: 5 * time.Second},
		"Zero":     {value: "0", want: 0},
		"Negative": {value: "-1", want: 0},
		"NotANumber": {
			// Validated where the Terraform configuration is built; failing
			// adoption over it would surface it in a confusing place.
			value: "thirty", want: 0,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			creds := Credentials{}
			if tc.value != "" {
				creds[KeyTimeoutSeconds] = tc.value
			}
			if got := creds.timeout(); got != tc.want {
				t.Errorf("timeout = %v, want %v", got, tc.want)
			}
		})
	}
}
