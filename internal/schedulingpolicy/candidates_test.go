package schedulingpolicy

import (
	"reflect"
	"testing"
	"time"
)

func Test候选只稳定提前新鲜心跳且不把未知未来当在线(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	fresh, boundary, future := now.Add(-time.Second), now.Add(-time.Minute), now.Add(time.Second)
	heartbeats := []*time.Time{nil, &fresh, &boundary, &future, &fresh}
	indices, err := CandidateOrderIndices(OnlineFirst, heartbeats, now, time.Minute)
	if err != nil || !reflect.DeepEqual(indices, []int{1, 4, 0, 2, 3}) {
		t.Fatalf("顺序=%v %v", indices, err)
	}
	indices, err = CandidateOrderIndices(ExistingOrder, heartbeats, time.Time{}, 0)
	if err != nil || !reflect.DeepEqual(indices, []int{0, 1, 2, 3, 4}) {
		t.Fatal("旧顺序被改变")
	}
	for _, order := range []string{"AUTO_SELECT", "UNKNOWN"} {
		if _, err = CandidateOrderIndices(order, heartbeats, now, time.Minute); err == nil {
			t.Fatal("未知策略未阻断")
		}
	}
	if _, err = CandidateOrderIndices(OnlineFirst, heartbeats, now, 0); err == nil {
		t.Fatal("未知阈值未阻断")
	}
}
