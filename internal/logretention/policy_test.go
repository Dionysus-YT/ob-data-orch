package logretention

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryPolicy struct {
	record  Record
	readErr error
	changes chan struct{}
}

func (m *memoryPolicy) ReadCurrentLogRetention(context.Context) (Record, error) {
	return m.record, m.readErr
}
func (m *memoryPolicy) ReadAuthorizedLogRetention(context.Context, string) (Record, error) {
	return m.record, m.readErr
}
func (m *memoryPolicy) ChangeLogRetention(_ context.Context, input Change, _ int) (Receipt, error) {
	if m.changes != nil {
		m.changes <- struct{}{}
	}
	now := time.Now().UTC()
	days := input.RetentionDays
	m.record = Record{Days: &days, Revision: m.record.Revision + 1, ModifiedBy: "synthetic-actor", ModifiedAt: &now}
	return Receipt{Revision: m.record.Revision, Changed: true}, nil
}

func Test策略来源及损坏数据失败关闭(t *testing.T) {
	for _, item := range []struct {
		deployment, days int
		source           string
	}{{0, 30, "BUILT_IN_DEFAULT"}, {60, 60, "DEPLOYMENT_CONFIG"}} {
		value, err := Resolve(Record{Revision: 1}, item.deployment)
		if err != nil || value.RetentionDays != item.days || value.Source != item.source || value.ModifiedAt != nil || value.RestartRequired {
			t.Fatalf("来源=%+v %v", value, err)
		}
	}
	now := time.Now().UTC()
	days := 7
	value, err := Resolve(Record{Days: &days, Revision: 2, ModifiedBy: "actor", ModifiedAt: &now}, 60)
	if err != nil || value.RetentionDays != 7 || value.Source != "PLATFORM_SETTING" {
		t.Fatalf("覆盖=%+v %v", value, err)
	}
	for _, record := range []Record{{Revision: 0}, {Days: &days, Revision: 2}, {Revision: 1, ModifiedBy: "actor"}} {
		if _, err := Resolve(record, 30); err == nil {
			t.Fatal("损坏策略被接受")
		}
	}
}

func Test策略保存与清理批次互斥且等待可取消(t *testing.T) {
	repo := &memoryPolicy{record: Record{Revision: 1}, changes: make(chan struct{}, 1)}
	service, _ := New(repo, 0)
	entered, release := make(chan struct{}), make(chan struct{})
	batchDone := make(chan error, 1)
	go func() {
		batchDone <- service.WithPolicy(context.Background(), func(days int) error {
			if days != 30 {
				return ErrInvalid
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Change(cancelled, Change{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消=%v", err)
	}
	changed := make(chan error, 1)
	go func() { _, err := service.Change(context.Background(), Change{RetentionDays: 365}); changed <- err }()
	select {
	case <-repo.changes:
		t.Fatal("旧批次未结束即保存")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-batchDone; err != nil {
		t.Fatal(err)
	}
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if err := service.WithPolicy(context.Background(), func(days int) error {
		if days != 365 {
			t.Fatalf("新批次仍使用旧期限 %d", days)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	repo.readErr = ErrUnavailable
	called := false
	if err := service.WithPolicy(context.Background(), func(int) error { called = true; return nil }); err == nil || called {
		t.Fatal("读取失败仍清理")
	}
}
