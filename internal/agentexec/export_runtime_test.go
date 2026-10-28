package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/logstream"
)

// 合成子进程只模拟固定日志路由和临时文件条件，不运行 Java、数据库或官方工具。
func init() {
	mode := os.Getenv("OBDO_SYNTHETIC_EXPORT_RUNTIME")
	if mode == "" {
		return
	}
	for _, argument := range os.Args[1:] {
		if argument == "-version" {
			fmt.Fprintln(os.Stderr, `java version "1.8.0_491"`)
			os.Exit(0)
		}
	}
	// 给父进程建立进程树监管的确定窗口，避免快速假工具退出抢先于 attach。
	time.Sleep(50 * time.Millisecond)
	property := func(prefix string) string {
		for _, argument := range os.Args[1:] {
			if strings.HasPrefix(argument, prefix) {
				return strings.TrimPrefix(argument, prefix)
			}
		}
		return ""
	}
	value := func(name string) string {
		for i, argument := range os.Args[1:] {
			if argument == name && i+2 < len(os.Args) {
				return os.Args[i+2]
			}
		}
		return ""
	}
	if mode == "LOG" {
		configuration := strings.TrimPrefix(property("-Dlog4j.configurationFile="), "file://")
		if len(configuration) > 2 && configuration[0] == '/' && configuration[2] == ':' {
			configuration = configuration[1:]
		}
		body, err := os.ReadFile(filepath.FromSlash(configuration))
		if err != nil {
			os.Exit(61)
		}
		if value("--log-path") == "" && strings.Contains(string(body), "Routing") {
			_ = os.Mkdir(filepath.Join(value("--file-path"), "logs"), 0700)
			fmt.Fprintln(os.Stderr, "target directory is not empty")
			os.Exit(62)
		}
	}
	if mode == "TEMP" {
		directory := property("-Djava.io.tmpdir=")
		if !filepath.IsAbs(directory) {
			os.Exit(63)
		}
		if err := os.WriteFile(filepath.Join(directory, "synthetic-native-cache.dll"), []byte("synthetic native bytes"), 0600); err != nil {
			os.Exit(64)
		}
	}
	if strings.HasPrefix(mode, "ENCODING") {
		unit := "μs"
		if property("-Dfile.encoding=") != "UTF-8" || mode == "ENCODING_INVALID" {
			// 模拟 Windows 缺省 GBK 的微秒符号；未知字节必须继续阻断，不能用替换字符掩盖。
			unit = string([]byte{0xa6, 0xcc, 's'})
		}
		fmt.Printf("synthetic duration 371.3 %s password=synthetic-direct-java-secret\n", unit)
	}
	count := 1
	if mode == "BURST" {
		count = 256
	}
	for i := 0; i < count; i++ {
		fmt.Printf("synthetic record %d password=synthetic-direct-java-secret\n", i)
	}
	fmt.Println("synthetic final complete")
	os.Exit(0)
}

func Test导出默认日志不在空输出目录创建路由日志且显式路径保留(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "默认受控日志", true: "显式日志目录"}[explicit], func(t *testing.T) {
			launch, workspace := exportRuntimeLaunch(t, "LOG")
			if explicit {
				launch.BusinessArguments = append(launch.BusinessArguments, "--log-path", filepath.Join(t.TempDir(), "explicit-logs"))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			process, err := StartDirectJava(ctx, workspace, launch)
			if err != nil {
				t.Fatal("合成启动失败", err)
			}
			exited, code, err := process.Wait()
			if err != nil || !exited || code != 0 {
				t.Fatal("默认日志污染输出或明确日志路径被改变", exited, code, err)
			}
			entries, err := os.ReadDir(launch.BusinessArguments[1])
			if err != nil || len(entries) != 0 {
				t.Fatal("空输出目录被日志污染", err)
			}
		})
	}
}

func Test导出原生临时文件绑定私有运行目录并随工作区清理(t *testing.T) {
	launch, workspace := exportRuntimeLaunch(t, "TEMP")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process, err := StartDirectJava(ctx, workspace, launch)
	if err != nil {
		t.Fatal(err)
	}
	if exited, code, err := process.Wait(); err != nil || !exited || code != 0 {
		t.Fatal("原生临时目录未生效", exited, code, err)
	}
	path := filepath.Join(workspace.RuntimeDirectory(), "synthetic-native-cache.dll")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("临时文件不在私有目录", err)
	}
	if result, err := CleanupSynthetic(workspace, true); err != nil || !result.Cleaned {
		t.Fatal("临时材料未按终态清理", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("临时材料仍有残留")
	}
}

func Test导出快速退出且接收较慢仍排空并脱敏末尾日志(t *testing.T) {
	launch, workspace := exportRuntimeLaunch(t, "BURST")
	var mu sync.Mutex
	var records []logstream.Record
	launch.Output.Sink = func(record logstream.Record) error {
		mu.Lock()
		defer mu.Unlock()
		if len(records) == 0 {
			time.Sleep(50 * time.Millisecond)
		}
		records = append(records, record)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process, err := StartDirectJava(ctx, workspace, launch)
	if err != nil {
		t.Fatal(err)
	}
	if exited, code, err := process.Wait(); err != nil || !exited || code != 0 {
		t.Fatal("快速退出丢失输出或误报等待失败", exited, code, err)
	}
	if len(records) != 257 || records[len(records)-1].Message != "synthetic final complete" {
		t.Fatal("末尾日志未完整排空", len(records))
	}
	for _, record := range records {
		if strings.Contains(record.Message, "synthetic-direct-java-secret") {
			t.Fatal("第一层脱敏未生效")
		}
	}
}

func Test导出日志缺省编码固定UTF8且非法字节仍失败关闭(t *testing.T) {
	for _, mode := range []string{"ENCODING", "ENCODING_INVALID"} {
		t.Run(mode, func(t *testing.T) {
			launch, workspace := exportRuntimeLaunch(t, mode)
			var records []logstream.Record
			launch.Output.Sink = func(record logstream.Record) error {
				_, err := logstream.SealBatch(logstream.Batch{StreamID: record.StreamID, SourceEpoch: record.SourceEpoch, FirstSeq: record.SourceSeq, LastSeq: record.SourceSeq, PolicyVersion: record.PolicyVersion, Records: []logstream.Record{record}})
				if err == nil {
					records = append(records, record)
				}
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			process, err := StartDirectJava(ctx, workspace, launch)
			if err != nil {
				t.Fatal(err)
			}
			exited, code, err := process.Wait()
			if mode == "ENCODING_INVALID" {
				if !exited || code != 0 || !errors.Is(err, ErrDirectJavaOutputFailed) {
					t.Fatal("非法字节未阻断，或丢失进程退出事实")
				}
				return
			}
			if !exited || code != 0 || err != nil || len(records) != 3 || !strings.Contains(records[0].Message, "μs") || strings.Contains(records[0].Message, "synthetic-direct-java-secret") {
				t.Fatal("缺省编码未固定，或完整脱敏记录无法进入队列", exited, code, err, len(records))
			}
		})
	}
}

func exportRuntimeLaunch(t *testing.T, mode string) (DirectJavaLaunch, credential.Workspace) {
	t.Helper()
	workspace, security := directJavaWorkspace(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	home := t.TempDir()
	for _, name := range []string{"lib", "conf"} {
		if err := os.Mkdir(filepath.Join(home, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "conf", "log4j2.xml"), []byte(`<Configuration><Routing name="synthetic-task.workspace"/></Configuration>`), 0600); err != nil {
		t.Fatal(err)
	}
	return DirectJavaLaunch{Tool: "OBDUMPER", Intent: validStartIntent(), BootID: "boot-1", JavaPath: executable, JavaSHA256: hex.EncodeToString(digest[:]), ToolHome: home, SecurityConfiguration: security, BusinessArguments: []string{"--file-path", t.TempDir(), "--csv"}, Environment: []string{"SystemRoot=" + os.Getenv("SystemRoot"), "OBDO_SYNTHETIC_EXPORT_RUNTIME=" + mode}, Output: directJavaOutput(t)}, workspace
}
