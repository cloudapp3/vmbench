package netio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeNetworkIdentityMarksTranslatedAddress(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	result, err := probeNetworkIdentity(context.Background(), "v4", deps)
	if err != nil {
		t.Fatalf("probeNetworkIdentity() error = %v", err)
	}
	if result.PublicIPv4 == nil || result.PublicIPv4.ASN != 64500 || result.PublicIPv4.CountryCode != "US" {
		t.Fatalf("PublicIPv4 = %+v, want enriched identity", result.PublicIPv4)
	}
	if len(result.NAT) != 1 || result.NAT[0].Status != "translated" {
		t.Fatalf("NAT = %+v, want translated", result.NAT)
	}
	if result.NAT[0].LocalIP != "10.0.0.2" || result.NAT[0].PublicIP != "198.51.100.20" {
		t.Fatalf("NAT evidence = %+v, want local and public IP", result.NAT[0])
	}
	assertIdentityProviderStatus(t, result, "local_interfaces", NetworkIdentityProviderOK)
	assertIdentityProviderStatus(t, result, "ipify_v4", NetworkIdentityProviderOK)
	assertIdentityProviderStatus(t, result, "ipwhois_v4", NetworkIdentityProviderOK)
}

func TestProbeNetworkIdentityMarksDirectAddress(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	deps.localAddresses = func() ([]LocalGlobalAddress, []error) {
		return []LocalGlobalAddress{{Interface: "eth0", Address: "198.51.100.20", IPVersion: "v4"}}, nil
	}
	result, err := probeNetworkIdentity(context.Background(), "v4", deps)
	if err != nil {
		t.Fatalf("probeNetworkIdentity() error = %v", err)
	}
	if len(result.NAT) != 1 || result.NAT[0].Status != "direct" {
		t.Fatalf("NAT = %+v, want direct", result.NAT)
	}
}

func TestProbeNetworkIdentityKeepsUnknownWhenLocalEvidenceIsPartial(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	deps.localAddresses = func() ([]LocalGlobalAddress, []error) {
		return []LocalGlobalAddress{{Interface: "eth0", Address: "10.0.0.2", IPVersion: "v4", Private: true}}, []error{errors.New("eth1: permission denied")}
	}
	result, err := probeNetworkIdentity(context.Background(), "v4", deps)
	if err != nil {
		t.Fatalf("probeNetworkIdentity() error = %v", err)
	}
	if len(result.NAT) != 1 || result.NAT[0].Status != "unknown" {
		t.Fatalf("NAT = %+v, want unknown", result.NAT)
	}
	assertIdentityProviderStatus(t, result, "local_interfaces", NetworkIdentityProviderPartial)
}

func TestProbeNetworkIdentityKeepsFamilyProviderErrorsSeparate(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	deps.publicIP = func(_ context.Context, family string) (string, error) {
		if family == "v6" {
			return "", errors.New("IPv6 unavailable")
		}
		return "198.51.100.20", nil
	}
	result, err := probeNetworkIdentity(context.Background(), "dual", deps)
	if err != nil {
		t.Fatalf("probeNetworkIdentity() error = %v", err)
	}
	if result.PublicIPv4 == nil || result.PublicIPv6 != nil {
		t.Fatalf("public identities = v4:%+v v6:%+v", result.PublicIPv4, result.PublicIPv6)
	}
	assertIdentityProviderStatus(t, result, "ipify_v4", NetworkIdentityProviderOK)
	assertIdentityProviderStatus(t, result, "ipify_v6", NetworkIdentityProviderError)
	assertIdentityProviderStatus(t, result, "ipwhois_v6", NetworkIdentityProviderSkipped)
	if len(result.NAT) != 2 || result.NAT[1].Status != "unknown" {
		t.Fatalf("NAT = %+v, want v6 unknown", result.NAT)
	}
}

func TestProbeNetworkIdentityReturnsPartialResultWhenPublicIPUnavailable(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	deps.publicIP = func(context.Context, string) (string, error) {
		return "", errors.New("provider offline")
	}
	result, err := probeNetworkIdentity(context.Background(), "v4", deps)
	if err == nil || !strings.Contains(err.Error(), "no requested public IP") {
		t.Fatalf("probeNetworkIdentity() error = %v, want public IP failure", err)
	}
	if result == nil || len(result.LocalGlobalAddresses) != 1 {
		t.Fatalf("result = %+v, want retained local evidence", result)
	}
	assertIdentityProviderStatus(t, result, "ipify_v4", NetworkIdentityProviderError)
	assertIdentityProviderStatus(t, result, "ipwhois_v4", NetworkIdentityProviderSkipped)
}

func TestProbeNetworkIdentityMetadataFailureDoesNotDiscardPublicIP(t *testing.T) {
	deps := validNetworkIdentityDependencies()
	deps.metadata = func(context.Context, string) (publicIPMetadata, error) {
		return publicIPMetadata{}, errors.New("metadata offline")
	}
	result, err := probeNetworkIdentity(context.Background(), "v4", deps)
	if err != nil {
		t.Fatalf("probeNetworkIdentity() error = %v", err)
	}
	if result.PublicIPv4 == nil || result.PublicIPv4.IP != "198.51.100.20" {
		t.Fatalf("PublicIPv4 = %+v, want observed address", result.PublicIPv4)
	}
	assertIdentityProviderStatus(t, result, "ipwhois_v4", NetworkIdentityProviderError)
}

func TestQueryPublicIPFromURLValidatesRequestedFamily(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("198.51.100.20\n"))
	}))
	defer server.Close()

	if _, err := queryPublicIPFromURL(context.Background(), server.Client(), server.URL, "v6"); err == nil || !strings.Contains(err.Error(), "invalid v6") {
		t.Fatalf("queryPublicIPFromURL() error = %v, want family mismatch", err)
	}
}

func TestQueryPublicIPMetadataFailsClosedOnIncompleteMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"ip":"198.51.100.20","country":"United States","country_code":"US","connection":{"asn":64500}}`))
	}))
	defer server.Close()

	_, err := queryPublicIPMetadataFromURL(context.Background(), server.Client(), server.URL, "198.51.100.20")
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("queryPublicIPMetadataFromURL() error = %v, want incomplete metadata", err)
	}
}

func validNetworkIdentityDependencies() networkIdentityDependencies {
	return networkIdentityDependencies{
		localAddresses: func() ([]LocalGlobalAddress, []error) {
			return []LocalGlobalAddress{{Interface: "eth0", Address: "10.0.0.2", IPVersion: "v4", Private: true}}, nil
		},
		publicIP: func(context.Context, string) (string, error) {
			return "198.51.100.20", nil
		},
		metadata: func(context.Context, string) (publicIPMetadata, error) {
			return publicIPMetadata{ASN: 64500, Org: "Example Net", ISP: "Example ISP", Country: "United States", CountryCode: "us"}, nil
		},
	}
}

func assertIdentityProviderStatus(t *testing.T, result *NetworkIdentityResult, id, want string) {
	t.Helper()
	for _, provider := range result.Providers {
		if provider.ID == id {
			if provider.Status != want {
				t.Fatalf("provider %s status = %q, want %q (provider=%+v)", id, provider.Status, want, provider)
			}
			return
		}
	}
	t.Fatalf("provider %s not found in %+v", id, result.Providers)
}
