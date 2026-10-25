// Package overview 定义首页的只读快照、时间窗口与事实摘要，不执行数据库、网络或工具操作。
package overview

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// ListLimit 限定首页下钻列表，完整记录继续由管理页提供。
	ListLimit = 5
	// RefreshSeconds 是首页可见期间的串行刷新间隔。
	RefreshSeconds = 30
	// StaleSeconds 只标识快照新鲜度，不改变任务或节点状态。
	StaleSeconds = 90
)

// Query 由控制面认证身份与固定时间窗口生成；浏览器不能自报主体或全局范围。
type Query struct {
	SubjectID  string
	GlobalRead bool
	Now        time.Time
	Range      string
	From       time.Time
	To         time.Time
	TodayFrom  time.Time
	TodayTo    time.Time
	Buckets    []Bucket
	// NodeIDs 仅由控制面逐对象授权后填入，浏览器不能传入；元数据读取失败单独标记。
	NodeIDs             []string
	NodeReadUnavailable bool
}

// NewQuery 使用已生效的内置 UTC 口径。系统设置接入前不猜测操作系统或浏览器时区。
func NewQuery(subjectID string, globalRead bool, now time.Time, period string) (Query, error) {
	if strings.TrimSpace(subjectID) == "" || now.IsZero() || period != "24h" && period != "7d" {
		return Query{}, fmt.Errorf("overview query is invalid")
	}
	// 浏览器与 SQLite 统一使用毫秒边界，避免下钻时截去亚毫秒精度而改变半开区间。
	now = now.UTC().Truncate(time.Millisecond)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	q := Query{SubjectID: subjectID, GlobalRead: globalRead, Now: now, Range: period, To: now, TodayFrom: day, TodayTo: day.AddDate(0, 0, 1)}
	q.From = now.Add(-24 * time.Hour)
	if period == "7d" {
		q.From = day.AddDate(0, 0, -6)
	}
	for from := q.From; from.Before(q.To); {
		to := from.Truncate(time.Hour).Add(time.Hour)
		if period == "7d" {
			to = time.Date(from.Year(), from.Month(), from.Day()+1, 0, 0, 0, 0, time.UTC)
		}
		if to.After(q.To) {
			to = q.To
		}
		q.Buckets = append(q.Buckets, Bucket{From: from, To: to})
		from = to
	}
	return q, nil
}

// Section 区分可信数据、依赖不可用和范围受限；非 READY 分区不携带旧数据或伪造零值。
type Section[T any] struct {
	Status string `json:"status"`
	Data   *T     `json:"data,omitempty"`
}

// Ready 构造当前快照内的可信分区。
func Ready[T any](data T) Section[T] { return Section[T]{Status: "READY", Data: &data} }

// Metrics 只包含首页已确认的四个精确状态指标。
type Metrics struct {
	Running        int `json:"running"`
	Waiting        int `json:"waiting"`
	TodaySucceeded int `json:"todaySucceeded"`
	TodayFailed    int `json:"todayFailed"`
}

// Bucket 使用终态时刻与半开区间，取消仅计入总数；首尾桶允许不足整小时或整天。
type Bucket struct {
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Total     int       `json:"total"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
}

// Progress 保留既有工具进度安全投影；百分比不参与成功或失败判定。
type Progress struct {
	Phase          string    `json:"phase"`
	Percent        *float64  `json:"percent,omitempty"`
	CompletedTasks *int64    `json:"completedTasks,omitempty"`
	RunningTasks   *int64    `json:"runningTasks,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Incomplete     bool      `json:"incomplete"`
}

// Task 是首页有限列表的安全事实，不包含配置、命令、凭据、路径或日志正文。
type Task struct {
	ID                     string     `json:"id"`
	DisplayName            string     `json:"displayName"`
	Type                   string     `json:"type"`
	State                  string     `json:"state"`
	ReconciliationRequired bool       `json:"reconciliationRequired"`
	StageEvidence          string     `json:"stageEvidence"`
	ProgressEvidence       string     `json:"progressEvidence"`
	Progress               *Progress  `json:"progress,omitempty"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	// FinishedAt 仅用于异常条目的原始终态时间，不额外扩大首页任务 DTO。
	FinishedAt *time.Time `json:"-"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Node 沿用节点模块的四维状态与接收资格，不传输安装路径、机器身份或其他用户任务。
type Node struct {
	ID                 string     `json:"id"`
	DisplayName        string     `json:"displayName"`
	ManagementState    string     `json:"managementState"`
	HeartbeatStatus    string     `json:"heartbeatStatus"`
	EnvironmentStatus  string     `json:"environmentStatus"`
	CapacityStatus     string     `json:"capacityStatus"`
	AcceptsNewTasks    bool       `json:"acceptsNewTasks"`
	UnavailableReasons []string   `json:"unavailableReasons"`
	LastHeartbeatAt    *string    `json:"lastHeartbeatAt"`
	CapacityTotal      *int       `json:"capacityTotal,omitempty"`
	CapacityUsed       *int       `json:"capacityUsed,omitempty"`
	ObservedAt         *time.Time `json:"observedAt,omitempty"`
	CPUUsagePercent    *int       `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent *int       `json:"memoryUsagePercent,omitempty"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// Nodes 仅统计逐项授权后的节点，Items 限定为优先需要关注的有限列表。
type Nodes struct {
	Total       int    `json:"total"`
	Unavailable int    `json:"unavailable"`
	Items       []Node `json:"items"`
}

// AttentionItem 来自任务终态或节点当前事实，同一对象只生成一个主条目，不推断跨来源因果。
type AttentionItem struct {
	Kind                   string    `json:"kind"`
	ObjectID               string    `json:"objectId"`
	DisplayName            string    `json:"displayName"`
	OccurredAt             time.Time `json:"occurredAt"`
	Source                 string    `json:"source"`
	ReconciliationRequired bool      `json:"reconciliationRequired"`
}

// Attention 明确采集边界；没有独立调度/工具日志来源时不声称覆盖全部异常。
type Attention struct {
	From     time.Time       `json:"from"`
	To       time.Time       `json:"to"`
	Coverage string          `json:"coverage"`
	Items    []AttentionItem `json:"items"`
}

// Snapshot 是一次只读事务产生的首页投影，所有分区共享同一时间和权限边界。
type Snapshot struct {
	SnapshotAt             time.Time          `json:"snapshotAt"`
	TimeZone               string             `json:"timeZone"`
	TimeZoneSource         string             `json:"timeZoneSource"`
	Range                  string             `json:"range"`
	TodayFrom              time.Time          `json:"todayFrom"`
	TodayTo                time.Time          `json:"todayTo"`
	RefreshIntervalSeconds int                `json:"refreshIntervalSeconds"`
	StaleAfterSeconds      int                `json:"staleAfterSeconds"`
	Summary                string             `json:"summary"`
	Metrics                Section[Metrics]   `json:"metrics"`
	Trend                  Section[[]Bucket]  `json:"trend"`
	Nodes                  Section[Nodes]     `json:"nodes"`
	MyTasks                Section[[]Task]    `json:"myTasks"`
	Attention              Section[Attention] `json:"attention"`
}

// Facts 是已完成对象授权的首页输入；nil 分区表示本次读取失败，不能回填旧值。
type Facts struct {
	// TasksRestricted 与 MyTasksRestricted 来自当前权限投影，不用空数组伪装可见零值。
	TasksRestricted   bool
	MyTasksRestricted bool
	Metrics           *Metrics
	Trend             *[]Bucket
	MyTasks           *[]Task
	Failures          *[]Task
	Nodes             *[]Node
}

// Build 从同一快照事实构造受限首页，不创建独立事件或改变原任务状态。
func Build(query Query, facts Facts) Snapshot {
	snapshot := Snapshot{SnapshotAt: query.Now, TimeZone: "UTC", TimeZoneSource: "BUILTIN_UTC", Range: query.Range,
		TodayFrom: query.TodayFrom, TodayTo: query.TodayTo, RefreshIntervalSeconds: RefreshSeconds, StaleAfterSeconds: StaleSeconds,
		Metrics: Section[Metrics]{Status: "UNAVAILABLE"}, Trend: Section[[]Bucket]{Status: "UNAVAILABLE"},
		MyTasks: Section[[]Task]{Status: "UNAVAILABLE"}, Nodes: Section[Nodes]{Status: "UNAVAILABLE"}, Attention: Section[Attention]{Status: "UNAVAILABLE"}}
	if facts.Metrics != nil {
		snapshot.Metrics = Ready(*facts.Metrics)
	}
	if facts.Trend != nil {
		snapshot.Trend = Ready(*facts.Trend)
	}
	if facts.MyTasks != nil {
		snapshot.MyTasks = Ready(*facts.MyTasks)
	}
	if facts.Nodes != nil {
		// 没有可见节点时不区分未注册与隐藏对象，避免通过空态枚举无权节点。
		snapshot.Nodes.Status = "RESTRICTED"
		if len(*facts.Nodes) > 0 {
			snapshot.Nodes = Ready(BuildNodes(*facts.Nodes))
		}
	}
	if facts.Failures != nil {
		attention := Attention{From: query.Now.Add(-24 * time.Hour), To: query.Now, Coverage: "TASK_FACTS_ONLY", Items: []AttentionItem{}}
		for _, task := range *facts.Failures {
			at := task.UpdatedAt
			if task.FinishedAt != nil {
				at = *task.FinishedAt
			}
			attention.Items = append(attention.Items, AttentionItem{Kind: "TASK_FAILED", ObjectID: task.ID, DisplayName: task.DisplayName, OccurredAt: at, Source: "TASK_STATE", ReconciliationRequired: task.ReconciliationRequired})
		}
		if facts.Nodes != nil && len(*facts.Nodes) > 0 {
			attention.Coverage = "TASK_AND_NODE_FACTS"
			for _, node := range *facts.Nodes {
				if !node.AcceptsNewTasks {
					// 当前节点资格没有历史状态转移事件，记录本次核对时间而非伪造异常发生时间。
					attention.Items = append(attention.Items, AttentionItem{Kind: "NODE_UNAVAILABLE", ObjectID: node.ID, DisplayName: node.DisplayName, OccurredAt: query.Now, Source: "NODE_STATE"})
				}
			}
		}
		sort.SliceStable(attention.Items, func(i, j int) bool {
			a, b := attention.Items[i], attention.Items[j]
			if a.Kind != b.Kind {
				return a.Kind == "TASK_FAILED"
			}
			if !a.OccurredAt.Equal(b.OccurredAt) {
				return a.OccurredAt.After(b.OccurredAt)
			}
			return a.ObjectID < b.ObjectID
		})
		if len(attention.Items) > ListLimit {
			attention.Items = attention.Items[:ListLimit]
		}
		snapshot.Attention = Ready(attention)
	}
	if facts.TasksRestricted {
		snapshot.Metrics = Section[Metrics]{Status: "RESTRICTED"}
		snapshot.Trend = Section[[]Bucket]{Status: "RESTRICTED"}
		snapshot.Attention = Section[Attention]{Status: "RESTRICTED"}
	}
	if facts.MyTasksRestricted {
		snapshot.MyTasks = Section[[]Task]{Status: "RESTRICTED"}
	}
	snapshot.Summary = Describe(snapshot)
	return snapshot
}

// BuildNodes 对已授权节点排序，不用 CPU/内存猜测健康或异常。
func BuildNodes(items []Node) Nodes {
	result := Nodes{Total: len(items), Items: append([]Node{}, items...)}
	for _, node := range items {
		if !node.AcceptsNewTasks {
			result.Unavailable++
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.AcceptsNewTasks != b.AcceptsNewTasks {
			return !a.AcceptsNewTasks
		}
		usedA, usedB := -1, -1
		if a.CapacityUsed != nil {
			usedA = *a.CapacityUsed
		}
		if b.CapacityUsed != nil {
			usedB = *b.CapacityUsed
		}
		if usedA != usedB {
			return usedA > usedB
		}
		atA, atB := time.Time{}, time.Time{}
		if a.ObservedAt != nil {
			atA = *a.ObservedAt
		}
		if b.ObservedAt != nil {
			atB = *b.ObservedAt
		}
		if !atA.Equal(atB) {
			return atA.After(atB)
		}
		return a.ID < b.ID
	})
	if len(result.Items) > ListLimit {
		result.Items = result.Items[:ListLimit]
	}
	return result
}

// Describe 只总结本次可信分区；部分失败或范围受限时明确缺口，不生成运行正常结论。
func Describe(snapshot Snapshot) string {
	parts := []string{}
	if snapshot.Metrics.Status == "READY" && snapshot.Metrics.Data != nil {
		m := snapshot.Metrics.Data
		parts = append(parts, fmt.Sprintf("当前授权范围内今日失败 %d 个、运行中 %d 个、等待调度 %d 个任务", m.TodayFailed, m.Running, m.Waiting))
	}
	if snapshot.Nodes.Status == "READY" && snapshot.Nodes.Data != nil {
		parts = append(parts, fmt.Sprintf("%d 个执行节点当前不能接收新任务", snapshot.Nodes.Data.Unavailable))
	}
	if snapshot.Metrics.Status == "UNAVAILABLE" || snapshot.Trend.Status == "UNAVAILABLE" || snapshot.Nodes.Status == "UNAVAILABLE" || snapshot.MyTasks.Status == "UNAVAILABLE" || snapshot.Attention.Status == "UNAVAILABLE" {
		parts = append([]string{"快照数据不完整，部分分区暂不可用"}, parts...)
	}
	if snapshot.Nodes.Status == "RESTRICTED" {
		parts = append(parts, "执行节点范围不可见")
	}
	return strings.Join(parts, "；") + "。"
}
