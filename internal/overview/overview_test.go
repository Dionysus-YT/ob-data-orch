package overview

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func Test权限受限分区不携带旧事实或可信零值(t *testing.T) {
	q, _ := NewQuery("synthetic-subject", false, time.Now().UTC(), "24h")
	metrics := Metrics{Running: 7}
	tasks := []Task{{ID: "hidden-task"}}
	buckets := q.Buckets
	view := Build(q, Facts{TasksRestricted: true, MyTasksRestricted: true, Metrics: &metrics, Trend: &buckets, MyTasks: &tasks, Failures: &tasks})
	if view.Metrics.Status != "RESTRICTED" || view.Metrics.Data != nil || view.Trend.Data != nil || view.MyTasks.Status != "RESTRICTED" || view.MyTasks.Data != nil || view.Attention.Status != "RESTRICTED" || view.Attention.Data != nil {
		t.Fatal("权限受限混入旧快照", view)
	}
}

func Test首页时间窗口使用UTC且首尾精确相接(t *testing.T) {
	now := time.Date(2026, 10, 23, 8, 17, 24, 123000000, time.FixedZone("合成时区", 8*3600))
	for _, period := range []string{"24h", "7d"} {
		q, err := NewQuery("subject", false, now, period)
		if err != nil || q.TodayFrom.Hour() != 0 || q.TodayFrom.Location() != time.UTC {
			t.Fatalf("窗口无效：%#v %v", q, err)
		}
		if !q.Buckets[0].From.Equal(q.From) || !q.Buckets[len(q.Buckets)-1].To.Equal(q.Now) {
			t.Fatal("首尾窗口不匹配")
		}
		for i, b := range q.Buckets {
			if !b.From.Before(b.To) || i > 0 && !q.Buckets[i-1].To.Equal(b.From) {
				t.Fatal("分桶有缺口或重叠")
			}
		}
		if period == "24h" && (q.Now.Sub(q.From) != 24*time.Hour || len(q.Buckets) != 25) {
			t.Fatal("最近24小时没有保留首尾部分桶")
		}
		if period == "7d" && (len(q.Buckets) != 7 || q.From != q.TodayFrom.AddDate(0, 0, -6)) {
			t.Fatal("自然日窗口不正确")
		}
	}
	for _, period := range []string{"", "30d", "24H", "../"} {
		if _, err := NewQuery("subject", false, now, period); err == nil {
			t.Fatal("未知范围被接纳")
		}
	}
	if _, err := NewQuery("", false, now, "24h"); err == nil {
		t.Fatal("缺失主体未拒绝")
	}
}

func Test首页部分失败不回填零值且受限节点不泄露计数(t *testing.T) {
	q, _ := NewQuery("subject", false, time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC), "24h")
	emptyNodes := []Node{}
	emptyTasks := []Task{}
	view := Build(q, Facts{Nodes: &emptyNodes, MyTasks: &emptyTasks, Failures: &emptyTasks})
	if view.Metrics.Status != "UNAVAILABLE" || view.Nodes.Status != "RESTRICTED" || view.Nodes.Data != nil || !strings.Contains(view.Summary, "不完整") {
		t.Fatalf("未知与受限被错误表示：%#v", view)
	}
	encoded, _ := json.Marshal(view.Nodes)
	if strings.Contains(string(encoded), "total") || strings.Contains(string(encoded), "items") {
		t.Fatal("受限分区泄露数量")
	}
	if view.Attention.Data.Coverage != "TASK_FACTS_ONLY" {
		t.Fatal("未取得节点范围却声称覆盖节点")
	}
}

func Test首页节点排序与异常沿用原始事实(t *testing.T) {
	q, _ := NewQuery("subject", false, time.Now().UTC(), "24h")
	used := 3
	nodes := []Node{{ID: "ready", AcceptsNewTasks: true}, {ID: "unknown"}, {ID: "busy", CapacityUsed: &used}}
	list := BuildNodes(nodes)
	if list.Unavailable != 2 || list.Items[0].ID != "busy" || list.Items[2].ID != "ready" {
		t.Fatalf("排序或计数错误：%#v", list)
	}
	finished := q.Now.Add(-time.Hour)
	failures := []Task{{ID: "failed", DisplayName: "合成任务", State: "FAILED", UpdatedAt: q.Now, FinishedAt: &finished, ReconciliationRequired: true}}
	view := Build(q, Facts{Nodes: &nodes, Failures: &failures})
	if view.Attention.Data.Items[0].OccurredAt != finished || !view.Attention.Data.Items[0].ReconciliationRequired {
		t.Fatal("任务异常没有使用终态时间或丢失核对标记")
	}
	for _, item := range view.Attention.Data.Items {
		if item.Kind == "NODE_UNAVAILABLE" && item.OccurredAt != q.Now {
			t.Fatal("当前节点事实冒充历史异常事件")
		}
	}
}
