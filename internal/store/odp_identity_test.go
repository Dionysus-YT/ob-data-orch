package store

import (
	"bytes"
	"strings"
	"testing"
)

func TestComposePrivateODPJDBCIdentityUsesCanonicalPrivateODPFormat(t *testing.T) {
	identity, ok := composePrivateODPJDBCIdentity("synthetic-user", "synthetic-tenant", "synthetic-cluster")
	if !ok {
		t.Fatal("composePrivateODPJDBCIdentity() rejected a valid private ODP identity")
	}
	defer func() {
		for index := range identity {
			identity[index] = 0
		}
	}()
	if !bytes.Equal(identity, []byte("synthetic-user@synthetic-tenant#synthetic-cluster")) {
		t.Fatalf("private ODP identity = %q", identity)
	}
}

func TestPrivateODPCommandIdentityUsesCanonicalPrivateODPFormat(t *testing.T) {
	identity, ok := PrivateODPCommandIdentity("synthetic-user", "synthetic-tenant", "synthetic-cluster")
	if !ok || identity != "synthetic-user@synthetic-tenant#synthetic-cluster" {
		t.Fatalf("PrivateODPCommandIdentity() = %q, %t", identity, ok)
	}
}

func TestComposePrivateODPJDBCIdentityFailsClosedForAmbiguousOrOversizedParts(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		tenantName  string
		clusterName string
	}{
		{name: "missing tenant", username: "user", clusterName: "cluster"},
		{name: "missing cluster", username: "user", tenantName: "tenant"},
		{name: "embedded separator", username: "user@tenant", tenantName: "tenant", clusterName: "cluster"},
		{name: "colon separator", username: "user", tenantName: "tenant:zone", clusterName: "cluster"},
		{name: "whitespace", username: "user", tenantName: "tenant name", clusterName: "cluster"},
		{name: "control character", username: "user", tenantName: "tenant", clusterName: "cluster\nname"},
		{name: "invalid utf8", username: string([]byte{0xff}), tenantName: "tenant", clusterName: "cluster"},
		{name: "oversized", username: strings.Repeat("u", maximumODPJDBCIdentityBytes), tenantName: "tenant", clusterName: "cluster"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, ok := composePrivateODPJDBCIdentity(test.username, test.tenantName, test.clusterName)
			if ok || identity != nil {
				t.Fatalf("composePrivateODPJDBCIdentity() = %q, %t", identity, ok)
			}
		})
	}
}
