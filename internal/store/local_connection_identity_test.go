package store

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func Test本地连接身份路径与租户失败关闭(t *testing.T) {
	for _, item := range []struct {
		kind, user, tenant, cluster string
		valid                       bool
	}{
		{"OBSERVER_DIRECT", "synthetic_user", "synthetic_tenant", "", true},
		{"OBSERVER_DIRECT", "synthetic_user", "synthetic_tenant", "synthetic_cluster", false},
		{"OBSERVER_DIRECT", "synthetic_user", "", "", false},
		{"OBSERVER_DIRECT", "synthetic_user@foreign", "synthetic_tenant", "", false},
		{"PUBLIC_CLOUD", "synthetic_user", "synthetic_tenant", "", false},
		{"ODP", "synthetic_user", "synthetic_tenant", "synthetic_cluster", true},
	} {
		identity, valid := composeLocalJDBCIdentity(item.kind, item.user, item.tenant, item.cluster)
		if valid != item.valid {
			t.Fatal("连接身份边界不符", item.kind)
		}
		if item.kind == "OBSERVER_DIRECT" && valid && !bytes.Equal(identity, []byte("synthetic_user@synthetic_tenant")) {
			t.Fatal("直连丢失租户或引入代理后缀")
		}
		clear(identity)
	}
}

func Test直连基础连接槽位保留租户且不修改凭据(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`UPDATE data_sources SET connection_kind='OBSERVER_DIRECT',cluster_name='' WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	created := requestSyntheticDataSourceConnectionTest(t, s, "direct-connection", "G2_SYNTHETIC")
	grant := claimAndAcknowledgeDataSourceConnectionTest(t, s, created.ConnectionTestID, testTime.Add(time.Minute))
	connection, err := s.ResolveDataSourceConnectionTestDatabaseConnection(context.Background(), DataSourceConnectionTestSecretResolutionRequest{AgentID: "agent-1", ConnectionTestID: created.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "direct-connection-resolve", RequestDigest: strings.Repeat("d", 64), Now: testTime.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal("直连槽位未接通", err)
	}
	defer connection.Destroy()
	if !bytes.Equal(connection.Username, []byte("synthetic_user@synthetic-tenant")) || connection.Revision != 1 {
		t.Fatal("直连身份或凭据修订不符")
	}
}
