// Package toolprogress 解析固定 4.3.5 工具日志中的进度事实；不计算 ETA，也不判定任务终态。
package toolprogress

import (
	"regexp"
	"strconv"
	"strings"
)

// Fact 只含固定阶段与工具打印的数据子任务进度；BPS 是百分比的万分位表示，避免浮点解析误差。
// Completed/Running 是工具子任务数，不是表数、记录数、字节数或数据库写入结果。
type Fact struct {
	Phase     string `json:"phase"`
	BPS       *int64 `json:"bps,omitempty"`
	Completed int64  `json:"completed"`
	Running   int64  `json:"running"`
}

const maxSafeCount = int64(9007199254740991)

var prefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] (.*)$`)
var progress = regexp.MustCompile(`^----------[ \t]+Finished Tasks: ([0-9]+)[ \t]+Running Tasks: ([0-9]+)[ \t]+Progress: ([0-9]{1,3})\.([0-9]{2})%(?:[ \t]+Finished File Merge Tasks: ([0-9]+)[ \t]+Running File Merge Tasks: ([0-9]+))?[ \t]+----------$`)
var finished = regexp.MustCompile(`^(Dump|Load) (schema|record) finished\. Total Elapsed: [^\r\n]+$`)

// Parse 只接受带固定时间/INFO 前缀的官方输出模板；任意数字、用户文本或格式漂移均不产生进度。
func Parse(tool, message string) *Fact {
	// 固定模板只有单行正文；控制字符不能把多个片段重新解释成一条可信事实。
	if tool != "OBDUMPER" && tool != "OBLOADER" || len(message) > 4096 || strings.ContainsAny(message, "\r\n\f\v") {
		return nil
	}
	parts := prefix.FindStringSubmatch(message)
	if len(parts) != 2 {
		return nil
	}
	phase := "EXPORT"
	if tool == "OBLOADER" {
		phase = "IMPORT"
	}
	if match := progress.FindStringSubmatch(parts[1]); len(match) != 0 {
		completed, err1 := strconv.ParseInt(match[1], 10, 64)
		running, err2 := strconv.ParseInt(match[2], 10, 64)
		whole, _ := strconv.ParseInt(match[3], 10, 64)
		fraction, _ := strconv.ParseInt(match[4], 10, 64)
		bps := whole*100 + fraction
		if err1 != nil || err2 != nil || completed > maxSafeCount || running > maxSafeCount || bps > 10000 || tool == "OBLOADER" && match[5] != "" {
			return nil
		}
		return &Fact{Phase: phase + "_DATA", BPS: &bps, Completed: completed, Running: running}
	}
	if match := finished.FindStringSubmatch(parts[1]); len(match) == 3 && (match[1] == "Load") == (tool == "OBLOADER") {
		return &Fact{Phase: phase + "_" + strings.ToUpper(match[2]) + "_COMPLETE"}
	}
	return nil
}

// ValidFor 同时复验工具类型、固定阶段与数值边界；不接受跨工具或部分数值事实。
func (f Fact) ValidFor(tool string) bool {
	phase := "EXPORT"
	if tool == "OBLOADER" {
		phase = "IMPORT"
	} else if tool != "OBDUMPER" {
		return false
	}
	if f.BPS != nil {
		return f.Phase == phase+"_DATA" && *f.BPS >= 0 && *f.BPS <= 10000 && f.Completed >= 0 && f.Completed <= maxSafeCount && f.Running >= 0 && f.Running <= maxSafeCount
	}
	return (f.Phase == phase+"_SCHEMA_COMPLETE" || f.Phase == phase+"_RECORD_COMPLETE") && f.Completed == 0 && f.Running == 0
}

// Equal 用于控制面复算 Agent 的固定结构化事实，不能只信任上传的百分比。
func (f Fact) Equal(other Fact) bool {
	return f.Phase == other.Phase && f.Completed == other.Completed && f.Running == other.Running && ((f.BPS == nil && other.BPS == nil) || (f.BPS != nil && other.BPS != nil && *f.BPS == *other.BPS))
}
