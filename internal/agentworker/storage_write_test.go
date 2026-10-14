package agentworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
)

func Test认证与写权限各解析一次且旧令牌重复阶段迟到均拒绝(t *testing.T) {
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	grant := testGrant(clock.Now())
	grant.Context.OutputKind = agentpreflight.OutputKindOSS
	grant.Context.StorageTarget = &agentwire.PrecheckStorageTarget{Provider: "OSS"}
	protocol := &protocolStub{}
	resolver := &leaseSecretResolver{protocol: protocol, clock: clock.Now, grant: grant, active: true}
	var previous context.Context
	for _, check := range []agentpreflight.CheckID{agentpreflight.CheckStorageAuth, agentpreflight.CheckStorageWrite} {
		protocol.storageSlot = agentwire.StorageCredentialSlot{Provider: "OSS", AccessKey: []byte("synthetic-access"), SecretKey: []byte("synthetic-secret")}
		permit := &secretResolvePermit{check: check}
		resolver.allow(permit)
		ctx := context.WithValue(context.Background(), secretResolvePermitContextKey{}, permit)
		if previous != nil {
			if _, err := resolver.ResolveStorageCredential(previous, grant.Binding); !errors.Is(err, ErrSecretResolution) {
				t.Fatal("旧阶段令牌仍可解析")
			}
		}
		material, err := resolver.ResolveStorageCredential(ctx, grant.Binding)
		if err != nil {
			t.Fatal(err)
		}
		material.Destroy()
		if _, err := resolver.ResolveStorageCredential(ctx, grant.Binding); !errors.Is(err, ErrSecretResolution) {
			t.Fatal("同阶段重复解析")
		}
		resolver.disallow(permit)
		if _, err := resolver.ResolveStorageCredential(ctx, grant.Binding); !errors.Is(err, ErrSecretResolution) {
			t.Fatal("迟到解析被接受")
		}
		previous = ctx
	}
	repeat := &secretResolvePermit{check: agentpreflight.CheckStorageWrite}
	resolver.allow(repeat)
	if _, err := resolver.ResolveStorageCredential(context.WithValue(context.Background(), secretResolvePermitContextKey{}, repeat), grant.Binding); !errors.Is(err, ErrSecretResolution) {
		t.Fatal("重复写权限阶段重置解析次数")
	}
	if len(protocol.callNames()) != 2 {
		t.Fatal("解析次数超出两个固定阶段")
	}
	resolver.close()
}
