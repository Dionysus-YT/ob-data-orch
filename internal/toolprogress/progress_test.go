package toolprogress

import (
	"strings"
	"testing"
)

const line = "2026-10-19 10:20:30.123 [INFO] ----------\tFinished Tasks: 7\tRunning Tasks: 3\tProgress: 63.25%\t----------"

func Test固定工具进度区分类型并保留零值和完整度口径(t *testing.T) {
	for _, tool := range []string{"OBDUMPER", "OBLOADER"} {
		fact := Parse(tool, line)
		if fact == nil || *fact.BPS != 6325 || fact.Completed != 7 || fact.Running != 3 || !fact.ValidFor(tool) {
			t.Fatal("未取得固定工具子任务进度", tool, fact)
		}
		other := "OBDUMPER"
		if tool == other {
			other = "OBLOADER"
		}
		if fact.ValidFor(other) {
			t.Fatal("进度跨工具生效")
		}
	}
	for _, amount := range []string{"0.00", "100.00"} {
		message := "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 0 Running Tasks: 0 Progress: " + amount + "% ----------"
		if Parse("OBLOADER", message) == nil {
			t.Fatal("真实零值或数据阶段完成被丢弃")
		}
	}
	merge := "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 1 Running Tasks: 0 Progress: 100.00% Finished File Merge Tasks: 0 Running File Merge Tasks: 2 ----------"
	if Parse("OBDUMPER", merge) == nil || Parse("OBLOADER", merge) != nil {
		t.Fatal("文件合并模板跨工具解释")
	}
}

func Test非固定输出越界和格式漂移不产生进度(t *testing.T) {
	for _, message := range []string{
		"63%", "SELECT Progress: 63.25%", "[INFO] ---------- Finished Tasks: 7 Running Tasks: 3 Progress: 63.25% ----------",
		"2026-10-19 10:20:30 [ERROR] ---------- Finished Tasks: 7 Running Tasks: 3 Progress: 63.25% ----------",
		"2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 7 Running Tasks: 3 Progress: 100.01% ----------",
		"2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 7 Running Tasks: 3 Progress: NaN% ----------",
		"2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 9007199254740992 Running Tasks: 3 Progress: 63.25% ----------",
		"2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: -1 Running Tasks: 3 Progress: 63.25% ----------",
		line + "\nadditional line", line + " extra value",
		strings.Replace(line, "\t", "\r", 1), strings.Replace(line, "\t", "\f", 1), strings.Replace(line, "\t", "\v", 1),
	} {
		if Parse("OBLOADER", message) != nil {
			t.Fatal("不可信输出产生了进度")
		}
	}
	if Parse("OTHER", line) != nil {
		t.Fatal("未知工具获得进度")
	}
}

func Test阶段只来自对应工具完成标记且没有凭空计算百分比(t *testing.T) {
	for _, item := range []struct{ tool, action, phase string }{{"OBDUMPER", "Dump", "EXPORT"}, {"OBLOADER", "Load", "IMPORT"}} {
		for _, part := range []string{"schema", "record"} {
			fact := Parse(item.tool, "2026-10-19 10:20:30 [INFO] "+item.action+" "+part+" finished. Total Elapsed: 1.00 s")
			if fact == nil || fact.BPS != nil || !fact.ValidFor(item.tool) {
				t.Fatal("阶段被错误解释为数值进度", fact)
			}
		}
	}
	if Parse("OBLOADER", "2026-10-19 10:20:30 [INFO] Dump schema finished. Total Elapsed: 1 s") != nil {
		t.Fatal("导入接受了导出阶段")
	}
	if Parse("OBLOADER", "2026-10-19 10:20:30 [INFO] Load schema finished. Total Elapsed: 1\v s") != nil {
		t.Fatal("控制字符产生了阶段事实")
	}
}
