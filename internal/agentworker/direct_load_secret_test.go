package agentworker

import (
	"context"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
)

func Test旁路秘密槽位只在独立许可和当前数据库检查中使用一次(t *testing.T) {
	for _, scenario := range []struct {
		kind    string
		enabled bool
	}{{"DIRECT_LOAD_FILES", false}, {"DIRECT_LOAD_FILES", true}, {"DIRECT_LOAD_PREFLIGHT", false}, {"DIRECT_LOAD_PREFLIGHT", true}} {
		now := time.Now().UTC()
		p := &protocolStub{grant: directGrant(now, scenario.kind), slot: agentwire.DatabaseConnectionSlot{Host: "127.0.0.1", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}
		r := &leaseSecretResolver{directLoadProbeEnabled: scenario.enabled, protocol: p, clock: func() time.Time { return now }, bootID: "synthetic-boot", grant: p.grant, active: true}
		permit := &secretResolvePermit{check: agentpreflight.CheckDatabaseConnectivity}
		r.allow(permit)
		ctx := context.WithValue(context.Background(), secretResolvePermitContextKey{}, permit)
		connection, err := r.ResolveDatabaseConnection(ctx, p.grant.Binding)
		allowed := scenario.enabled && scenario.kind == "DIRECT_LOAD_PREFLIGHT"
		if (err == nil) != allowed {
			t.Fatal("旁路秘密独立门禁错误")
		}
		credential.Zero(connection.Username)
		credential.Zero(connection.Password)
		if _, err := r.ResolveDatabaseConnection(ctx, p.grant.Binding); err == nil {
			t.Fatal("同租约旁路秘密重复解析")
		}
		want := 0
		if allowed {
			want = 1
		}
		if p.resolveCount() != want {
			t.Fatal("旁路文件检查或未授权检查触发了解密")
		}
	}
}
