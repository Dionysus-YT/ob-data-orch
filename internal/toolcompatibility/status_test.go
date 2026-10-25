package toolcompatibility

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/parammeta"
)

// Test工具状态来自实际目录并区分导出选择和导入映射 核对实际加载资源及三个模块的不同语义。
func Test工具状态来自实际目录并区分导出选择和导入映射(t *testing.T) {
	service, err := New(time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.Read(context.Background())
	if err != nil || item.Source != "BUILT_IN_CATALOGS" || item.LoadedAt.After(item.CapturedAt) || len(item.Tools) != 2 {
		t.Fatal("工具状态无效")
	}
	loaders := []func() (*parammeta.Catalog, error){parammeta.LoadDefault, parammeta.LoadLegacyGeneralized, parammeta.LoadGeneralized, parammeta.LoadObjectSelection, parammeta.LoadCombinedObjectSelection, parammeta.LoadDDLTextFormats, parammeta.LoadExpertParameters}
	export := item.Tools[0].Modules[0]
	if export.TaskType != "OBDUMPER_EXPORT" || export.MetadataSelection != "TASK_CONFIGURATION" || len(export.Catalogs) != len(loaders) {
		t.Fatal("导出被误投影为单一活动元数据")
	}
	for i, load := range loaders {
		catalog, err := load()
		if err != nil || export.Catalogs[i].MetadataVersion != catalog.MetadataVersion() || item.Tools[0].DefaultTargetVersion != catalog.ToolVersion() {
			t.Fatal("投影与实际导出目录不一致")
		}
		if historical := export.Catalogs[i].Usage == "HISTORICAL_REPLAY"; historical != (i == 1 || i == 3) {
			t.Fatal("历史专用目录混入新建选择")
		}
	}
	loader := item.Tools[1]
	for _, version := range []string{importdomain.ToolVersion, directload.ToolVersion} {
		if loader.DefaultTargetVersion != version {
			t.Fatal("工具目标与导入模块不一致")
		}
	}
	if loader.Modules[0].TaskType != "OBLOADER_NORMAL_IMPORT" || loader.Modules[1].TaskType != "OBLOADER_DIRECT_LOAD" || loader.Modules[0].Catalogs[0].MetadataVersion != importdomain.MetadataVersion || loader.Modules[1].Catalogs[0].MetadataVersion != directload.MetadataVersion {
		t.Fatal("普通与旁路目录混用")
	}
	encoded, _ := json.Marshal(item)
	for _, forbidden := range []string{"password", "filePath", "nodeId", "installedVersion", "activeMetadataVersion", "sourceDocuments"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("工具投影包含额外事实或推测状态")
		}
	}
}

// Test工具读取深副本不会改写运行基线 验证外部修改响应列表不会污染后续读取。
func Test工具读取深副本不会改写运行基线(t *testing.T) {
	service, _ := New(time.Now().Add(-time.Second))
	item, _ := service.Read(context.Background())
	item.Tools[0].SupportedVersions[0] = "synthetic-other"
	item.Tools[0].Modules[0].Catalogs[0].MetadataVersion = "synthetic-other"
	item.Tools[1].Modules[0].TaskType = "OBDUMPER_EXPORT"
	current, _ := service.Read(context.Background())
	if current.Tools[0].SupportedVersions[0] != "4.3.5-RELEASE" || current.Tools[0].Modules[0].Catalogs[0].MetadataVersion == "synthetic-other" || current.Tools[1].Modules[0].TaskType != "OBLOADER_NORMAL_IMPORT" {
		t.Fatal("读取者改写了运行基线")
	}
}

// Test工具缺依赖坏时间及取消不返回默认值 验证未知事实不能被内置产品目标兜底。
func Test工具缺依赖坏时间及取消不返回默认值(t *testing.T) {
	for _, at := range []time.Time{{}, time.Now().Add(time.Hour)} {
		if _, err := New(at); !errors.Is(err, ErrUnavailable) {
			t.Fatal("坏载入时间未拒绝")
		}
	}
	for _, service := range []*Service{nil, {}} {
		if _, err := service.Read(context.Background()); !errors.Is(err, ErrUnavailable) {
			t.Fatal("缺失服务返回猜测基线")
		}
	}
	service, _ := New(time.Now().Add(-time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Read(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal("取消后仍返回状态")
	}
}
