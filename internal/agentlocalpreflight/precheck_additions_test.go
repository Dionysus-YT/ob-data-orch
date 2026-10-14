package agentlocalpreflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/agentpreflight"
)

func Test存储辅助目录验证临时与日志且不阻断独立空间(t *testing.T) {
	root := t.TempDir()
	request := testRequest(filepath.Join(root, "unused"), root)
	request.OutputKind = agentpreflight.OutputKindOSS
	request.StorageTarget = &agentpreflight.StorageTarget{TmpPath: testRequest(filepath.Join(root, "tmp"), root).OutputPath}
	request.LogPath = testRequest(filepath.Join(root, "logs"), root).OutputPath
	probe := Probe{MinimumAvailableBytes: 64, AvailableBytes: func(string) (uint64, error) { return 128, nil }}
	if result := probe.outputPathResult(request); result.Status != agentpreflight.StatusPassed {
		t.Fatal("安全辅助目录未通过", result)
	}
	request.LogPath = testRequest(filepath.Join(filepath.Dir(root), "outside-logs"), root).OutputPath
	if result := probe.outputPathResult(request); result.Status != agentpreflight.StatusFailed {
		t.Fatal("越界日志被接受")
	}
	if result := probe.availableSpaceResult(request); result.Status != agentpreflight.StatusPassed {
		t.Fatal("日志失败错误阻断临时卷空间")
	}
	request.StorageTarget.TmpPath = ""
	request.LogPath = ""
	if result := probe.outputPathResult(request); result.Status != agentpreflight.StatusUnknown {
		t.Fatal("缺少临时目录被放行")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("辅助目录检查遗留文件")
	}
}

type syntheticWriteProbe struct {
	enabled, verified bool
	err               error
	retained          *[][]byte
}

func (p syntheticWriteProbe) RequiresStorageCredentials() bool { return p.enabled }
func (p syntheticWriteProbe) ProbeWrite(_ context.Context, _ string, key, secret []byte, _ agentpreflight.StorageTarget) (bool, error) {
	if p.retained != nil {
		*p.retained = [][]byte{key, secret}
	}
	return p.verified, p.err
}

func Test写权限未装配不解析秘密且清理未知不得通过(t *testing.T) {
	for _, item := range []struct {
		prober CredentialStorageWriteProber
		code   string
		calls  int
	}{
		{nil, "STORAGE_WRITE_UNAVAILABLE", 0}, {syntheticWriteProbe{}, "STORAGE_WRITE_UNAVAILABLE", 0},
		{syntheticWriteProbe{enabled: true, verified: true}, "STORAGE_WRITE_VERIFIED", 1},
		{syntheticWriteProbe{enabled: true}, "STORAGE_WRITE_REJECTED", 1},
		{syntheticWriteProbe{enabled: true, err: ErrStorageWriteCleanupUnconfirmed}, "STORAGE_WRITE_CLEANUP_UNCONFIRMED", 1},
		{syntheticWriteProbe{enabled: true, err: errors.New("合成内部异常")}, "STORAGE_WRITE_UNAVAILABLE", 1},
	} {
		var retained [][]byte
		if prober, ok := item.prober.(syntheticWriteProbe); ok {
			prober.retained = &retained
			item.prober = prober
		}
		resolver := &recordingStorageCredentialResolver{credential: agentpreflight.StorageCredential{Provider: "OSS", AccessKey: []byte("synthetic-access"), SecretKey: []byte("synthetic-secret")}}
		result, err := (Probe{StorageWrite: item.prober, StorageCredentials: resolver}).storageWriteResult(context.Background(), storageRequest(""))
		if err != nil || result.EvidenceCode != item.code || resolver.calls != item.calls {
			t.Fatal("写权限事实投影或解析次数错误", result, err)
		}
		if item.calls > 0 {
			if len(retained) != 2 {
				t.Fatal("未捕获短时字节所有权")
			}
			for _, bytes := range retained {
				for _, value := range bytes {
					if value != 0 {
						t.Fatal("探测秘密未清零")
					}
				}
			}
		}
	}
}
