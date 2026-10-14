package nodefacts

import (
	"strings"
	"testing"
	"time"
)

func Test诊断事实拒绝路径输出矛盾时间和非法队列(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-time.Minute)
	versions := &RuntimeVersions{Java: &InstalledVersion{Version: "1.8.0_491", Source: "JAVA_RELEASE"}, OBLoader: &InstalledVersion{Version: "4.3.5-RELEASE", Source: "JAR_MANIFEST"}}
	active := &ExecutionObservation{Active: []ActiveExecution{{ExecutionID: "execution-1", LeaseID: "lease-1", LeaseEpoch: 1, LastEventSeq: 3}}}
	queue := &LogQueue{PendingBatches: 1, PendingBytes: 128, LastUploadResult: "SUCCEEDED", LastUploadAt: &now}
	if err := Validate(&started, now, versions, active, queue); err != nil {
		t.Fatal(err)
	}
	if err := Validate(nil, now, nil, nil, nil); err != nil {
		t.Fatal("旧 Agent 的缺项不能被拒绝")
	}
	for _, value := range []string{"C:\\synthetic\\java", "version\noutput", "1.0 token", strings.Repeat("1", 65)} {
		if ValidVersion(value) {
			t.Fatal("任意输出被当作版本")
		}
	}
	for _, invalid := range []*LogQueue{
		{PendingBatches: -1, LastUploadResult: "UNKNOWN"}, {PendingBytes: 1 << 31, LastUploadResult: "UNKNOWN"},
		{LastUploadResult: "SUCCEEDED"}, {LastUploadResult: "UNKNOWN", LastUploadAt: &now}, {LastUploadResult: "raw-output"},
	} {
		if Validate(nil, now, nil, nil, invalid) == nil {
			t.Fatal("非法日志队列通过校验")
		}
	}
	for _, invalid := range []*ExecutionObservation{
		{}, {Active: []ActiveExecution{{ExecutionID: "execution", LeaseID: "/private/path", LeaseEpoch: 1}}},
		{Active: append(active.Active, active.Active...)}, {Active: []ActiveExecution{{ExecutionID: "execution", LeaseID: "lease", LeaseEpoch: 0}}},
	} {
		if Validate(nil, now, nil, invalid, nil) == nil {
			t.Fatal("非法执行观测通过校验")
		}
	}
	future := now.Add(time.Second)
	if Validate(&future, now, nil, nil, nil) == nil {
		t.Fatal("矛盾启动时间通过校验")
	}
	oldUpload := started.Add(-time.Second)
	queue.LastUploadAt = &oldUpload
	if Validate(&started, now, nil, nil, queue) == nil {
		t.Fatal("另一进程的上传结果被接受")
	}
	versions.Java.Source = "JAR_MANIFEST"
	if Validate(nil, now, versions, nil, nil) == nil {
		t.Fatal("版本来源混用通过校验")
	}
}
