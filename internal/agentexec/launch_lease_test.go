package agentexec

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test直接启动在版本探测之后重新验证租约且不写启动意图(t *testing.T) {
	launch, workspace := exportRuntimeLaunch(t, "TEMP")
	now := time.Now()
	launch.LeaseExpiresAt = now.Add(time.Minute)
	calls := 0
	launch.Clock = func() time.Time {
		calls++
		if calls > 1 {
			return launch.LeaseExpiresAt
		}
		return now
	}
	if process, err := StartDirectJava(context.Background(), workspace, launch); process != nil || !errors.Is(err, ErrDirectJavaLaunchInvalid) {
		t.Fatal("过期租约启动了受控进程", err)
	}
	if calls < 2 {
		t.Fatal("没有在版本探测后复核租约")
	}
	if _, found, err := ReadStartIntent(workspace); err != nil || found {
		t.Fatal("过期启动仍写入执行意图", err)
	}
}

func Test直接启动拒绝缺失或已过期租约(t *testing.T) {
	launch, workspace := exportRuntimeLaunch(t, "TEMP")
	for _, expiry := range []time.Time{{}, time.Now().Add(-time.Second)} {
		launch.LeaseExpiresAt = expiry
		if process, err := StartDirectJava(context.Background(), workspace, launch); process != nil || !errors.Is(err, ErrDirectJavaLaunchInvalid) {
			t.Fatal("无效租约未阻断", err)
		}
	}
}

func Test启动意图写入后租约到期仍不创建进程(t *testing.T) {
	launch, workspace := exportRuntimeLaunch(t, "TEMP")
	now := time.Now()
	launch.LeaseExpiresAt = now.Add(time.Minute)
	calls := 0
	launch.Clock = func() time.Time {
		calls++
		if calls >= 3 {
			return launch.LeaseExpiresAt
		}
		return now
	}
	if process, err := StartDirectJava(context.Background(), workspace, launch); process != nil || !errors.Is(err, ErrDirectJavaLaunchInvalid) {
		t.Fatal("进程创建前到期未阻断", err)
	}
	if calls != 3 {
		t.Fatal("缺少实际创建进程前的租约复核", calls)
	}
	if _, found, err := ReadStartIntent(workspace); err != nil || !found {
		t.Fatal("启动意图没有保留供无进程核对", err)
	}
}
