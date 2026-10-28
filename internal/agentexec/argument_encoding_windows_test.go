//go:build windows

package agentexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindows工具参数必须在代码页内无替换往返(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		page  uint32
		want  bool
	}{
		{"ASCII", "SELECT id FROM synthetic_table", 936, true},
		{"中文与微秒", "中文 μs", 936, true},
		{"补充字符拒绝", "label = '🙂'", 936, false},
		{"其他代码页拒绝中文", "中文", 1252, false},
		{"UTF8往返", "中文🙂", 65001, true},
		{"非法字节", string([]byte{0xff}), 936, false},
		{"NUL", "synthetic\x00value", 936, false},
		{"未知代码页", "synthetic", 0xffffffff, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if toolArgumentRepresentable(test.value, test.page) != test.want {
				t.Fatal("参数可表示性判定不符合预期")
			}
		})
	}
}

func TestWindows不可表示参数在创建工具进程前拒绝(t *testing.T) {
	if toolArgumentRepresentable("🙂", windows.GetACP()) {
		t.Skip("当前系统代码页可表示该合成字符，代码页固定负例由上项验证")
	}
	launch, workspace := exportRuntimeLaunch(t, "BURST")
	launch.BusinessArguments = append(launch.BusinessArguments, "--where", "label = '🙂'")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process, err := StartDirectJava(ctx, workspace, launch)
	if process != nil {
		_ = process.Cancel()
		_, _, _ = process.Wait()
		t.Fatal("不可表示参数仍创建了工具进程")
	}
	if !errors.Is(err, ErrDirectJavaLaunchInvalid) {
		t.Fatal("不可表示参数未在固定启动边界拒绝")
	}
}
