package outputpath

import "testing"

func TestWindows导出命令只接受正斜杠盘符路径(t *testing.T) {
	for _, value := range []string{"E:\\workespace\\ob-data-orch\\tmp", "file:///E:/workespace/ob-data-orch/tmp", "/E:/workespace/../tmp", "/E://workespace/tmp"} {
		if IsExportOutputPath(PlatformWindowsAMD64, value) {
			t.Fatalf("IsExportOutputPath() 错误接受 %q", value)
		}
	}
	if !IsExportOutputPath(PlatformWindowsAMD64, "/E:/workespace/ob-data-orch/tmp") {
		t.Fatal("IsExportOutputPath() 拒绝正斜杠盘符路径")
	}
}

func TestWindows白名单比较兼容既有格式但不改变导出命令格式(t *testing.T) {
	output := "/E:/workespace/ob-data-orch/tmp/job-001"
	if !IsAllowedRootPath(PlatformWindowsAMD64, `E:\workespace\ob-data-orch\tmp`) {
		t.Fatal("IsAllowedRootPath() 拒绝既有 Windows 白名单")
	}
	if !WithinAllowedRoot(PlatformWindowsAMD64, output, `E:\workespace\ob-data-orch\tmp`) {
		t.Fatal("WithinAllowedRoot() 未识别等价的 Windows 白名单")
	}
	if WithinAllowedRoot(PlatformWindowsAMD64, "/E:/workespace/ob-data-orch/tmp-other", "/E:/workespace/ob-data-orch/tmp") {
		t.Fatal("WithinAllowedRoot() 接受白名单外目录")
	}
	path, ok := LocalFilesystemPath(PlatformWindowsAMD64, output)
	if !ok || path != `E:\workespace\ob-data-orch\tmp\job-001` {
		t.Fatalf("LocalFilesystemPath() = %q, %t", path, ok)
	}
}
