package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/store"
)

// Test调度设置从实际接口保存到三类候选读取并跨重启 仅使用临时 SQLite 与合成身份和心跳。
// 身份入口和 CSRF 使用固定隔离依赖；授权、策略事务、审计与候选投影使用实际实现，不连接外部服务。
func Test调度设置从实际接口保存到三类候选读取并跨重启(t *testing.T) {
	metadata, path := openSyntheticStore(t)
	seedSyntheticMetadata(t, path)
	ctx := context.Background()
	now := time.Now().UTC()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE execution_nodes SET normalized_name='a' WHERE node_id='node-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE agents SET last_heartbeat_at=? WHERE agent_id='agent-1'`, now.Add(-10*time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO execution_nodes(node_id,display_name,normalized_name,platform,management_state,allowed_roots_json,revision,created_by,created_at,updated_at) VALUES('node-2','合成在线节点','b','WINDOWS_AMD64','ENABLED','[]',1,'subject-1',?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO agents(agent_id,node_id,credential_digest,credential_revision,status,protocol_version,boot_id,last_heartbeat_at,capacity_total,capacity_used,facts_revision,created_at) VALUES('agent-2','node-2',?,1,'ACTIVE','synthetic-v1','synthetic-boot-2',?,1,0,1,?)`, []byte{10, 11, 12}, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = metadata.BootstrapPermissionOwner(ctx, "subject-1", now); err != nil {
		t.Fatal(err)
	}
	handler := func(database *store.Store) http.Handler {
		return controlplane.NewHandlerWithDependencies(buildinfo.Info{}, controlplane.Dependencies{Identity: integrationIdentityProvider{}, Authorizer: database, CSRF: integrationCSRF{}, NodeManagement: database, NodeCandidates: database, SchedulingPolicy: database, SchedulingTiming: database, SchedulingCandidates: database, HeartbeatTTL: 2 * time.Minute})
	}
	h := handler(metadata)
	readIDs := func(url string) string {
		t.Helper()
		raw := request(t, h, "GET", url, "", nil, 200)
		var response struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		result := ""
		for _, item := range response.Items {
			result += item.ID + ","
		}
		return result
	}
	if ids := readIDs("/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT"); ids != "node-1,node-2," {
		t.Fatalf("升级初值改变顺序=%s", ids)
	}
	headers := map[string]string{"If-Match": `"rev-1"`, "Idempotency-Key": "synthetic-closure-candidates", "X-CSRF-Token": "synthetic-only"}
	body := `{"order":"ONLINE_FIRST","reason":"合成候选闭环","confirmed":true}`
	request(t, h, "PATCH", "/api/v1/settings/scheduling/candidates", body, headers, 200)
	for _, url := range []string{"/api/v1/execution-nodes?eligibleFor=OBDUMPER_EXPORT", "/api/v1/execution-nodes?candidateOrder=platform"} {
		if ids := readIDs(url); ids != "node-2,node-1," {
			t.Fatalf("策略未联动=%s", ids)
		}
	}
	if ids := readIDs("/api/v1/execution-nodes"); ids != "node-1,node-2," {
		t.Fatal("改写管理列表顺序")
	}
	headers["Idempotency-Key"] = "synthetic-closure-concurrency"
	request(t, h, "PATCH", "/api/v1/settings/scheduling", `{"maxConcurrentExecutions":1,"reason":"合成并发闭环","confirmed":true}`, headers, 200)
	headers["Idempotency-Key"] = "synthetic-closure-timing"
	request(t, h, "PATCH", "/api/v1/settings/scheduling/timing", `{"heartbeatTimeoutSeconds":60,"startupTimeoutSeconds":30,"pollingIntervalSeconds":5,"reason":"合成时间闭环","confirmed":true}`, headers, 200)
	if err = metadata.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, err = store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer metadata.Close()
	h = handler(metadata)
	if ids := readIDs("/api/v1/execution-nodes?candidateOrder=platform"); ids != "node-2,node-1," {
		t.Fatal("重启丢失策略")
	}
	headers["Idempotency-Key"] = "synthetic-closure-candidates"
	raw := request(t, h, "PATCH", "/api/v1/settings/scheduling/candidates", body, headers, 200)
	var replay struct {
		Item struct {
			Replayed bool `json:"replayed"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &replay) != nil || !replay.Item.Replayed {
		t.Fatal("重启后未重放原回执")
	}
	maximum, err := metadata.ReadAuthorizedSchedulingPolicy(ctx, "subject-1")
	if err != nil || maximum.Revision != 2 || maximum.MaxConcurrentExecutions == nil || *maximum.MaxConcurrentExecutions != 1 {
		t.Fatal("独立并发事务丢失")
	}
	timing, err := metadata.ReadCurrentSchedulingTiming(ctx, 2*time.Minute)
	if err != nil || timing.HeartbeatTimeoutSeconds != 60 || timing.StartupTimeoutSeconds == nil || *timing.StartupTimeoutSeconds != 30 {
		t.Fatal("独立时间事务丢失")
	}
	var audits int
	if err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action IN ('CANDIDATE_ORDER_CHANGED','SCHEDULING_POLICY_CHANGED','SCHEDULING_TIMING_CHANGED') AND result='SUCCEEDED'`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("审计或重放重复=%d %v", audits, err)
	}
}
