// Package toolcompatibility 投影内置工具与参数目录，不发布元数据或操作节点环境。
package toolcompatibility

import (
	"context"
	"errors"
	"time"

	"ob-data-orch/internal/parammeta"
)

// ErrUnavailable 表示内置目录无法可靠读取，不能以产品目标或猜测默认值替代。
var ErrUnavailable = errors.New("工具与兼容状态暂时不可用")

// CatalogVersion 区分按任务配置选择的目录和仅用于历史重放的目录。
type CatalogVersion struct {
	MetadataVersion string `json:"metadataVersion"`
	Usage           string `json:"usage"`
}

// Module 保留三类任务各自的选择规则，OBLOADER 共用工具不代表共用参数。
type Module struct {
	TaskType          string           `json:"taskType"`
	MetadataSelection string           `json:"metadataSelection"`
	Catalogs          []CatalogVersion `json:"catalogs"`
}

// Tool 仅表达控制面内置的受控目标，不包含节点安装或可执行结论。
type Tool struct {
	Tool                 string   `json:"tool"`
	SupportedVersions    []string `json:"supportedVersions"`
	DefaultTargetVersion string   `json:"defaultTargetVersion"`
	Modules              []Module `json:"modules"`
}

// Snapshot 是无秘密、无业务对象和无文件路径的只读投影。
type Snapshot struct {
	CapturedAt time.Time `json:"capturedAt"`
	LoadedAt   time.Time `json:"loadedAt"`
	Source     string    `json:"source"`
	Tools      []Tool    `json:"tools"`
}

// Service 持有实际加载的内置目录快照，后续读取返回深副本。
type Service struct{ snapshot Snapshot }

// New 加载命令生成器同源的受控目录；没有可激活列表或全平台单一导出活动版本。
func New(loadedAt time.Time) (*Service, error) {
	if loadedAt.IsZero() || loadedAt.After(time.Now()) {
		return nil, ErrUnavailable
	}
	loaders := []func() (*parammeta.Catalog, error){parammeta.LoadDefault, parammeta.LoadLegacyGeneralized, parammeta.LoadGeneralized, parammeta.LoadObjectSelection, parammeta.LoadCombinedObjectSelection, parammeta.LoadDDLTextFormats, parammeta.LoadExpertParameters, parammeta.LoadSchemaBoolean, parammeta.LoadNormalImport, parammeta.LoadDirectLoad}
	catalogs := make([]*parammeta.Catalog, len(loaders))
	for i, load := range loaders {
		catalog, err := load()
		if err != nil || catalog == nil {
			return nil, ErrUnavailable
		}
		catalogs[i] = catalog
	}
	exportCatalogCount := len(catalogs) - 2
	tools := []Tool{
		{Tool: "OBDUMPER", SupportedVersions: []string{catalogs[0].ToolVersion()}, DefaultTargetVersion: catalogs[0].ToolVersion(), Modules: []Module{{TaskType: "OBDUMPER_EXPORT", MetadataSelection: "TASK_CONFIGURATION"}}},
		{Tool: "OBLOADER", SupportedVersions: []string{catalogs[exportCatalogCount].ToolVersion()}, DefaultTargetVersion: catalogs[exportCatalogCount].ToolVersion(), Modules: []Module{{TaskType: "OBLOADER_NORMAL_IMPORT", MetadataSelection: "FIXED"}, {TaskType: "OBLOADER_DIRECT_LOAD", MetadataSelection: "FIXED"}}},
	}
	for i, catalog := range catalogs {
		tool, module := 0, 0
		if i >= exportCatalogCount {
			tool, module = 1, i-exportCatalogCount
		}
		if catalog.Tool() != tools[tool].Tool || catalog.ToolVersion() != tools[tool].DefaultTargetVersion || catalog.MetadataVersion() == "" {
			return nil, ErrUnavailable
		}
		usage := "TASK_CONFIGURATION"
		// v6 与五类对象 v8 只由已冻结草稿显式重放，新建不静默采用旧目录。
		if i == 1 || i == 3 {
			usage = "HISTORICAL_REPLAY"
		}
		tools[tool].Modules[module].Catalogs = append(tools[tool].Modules[module].Catalogs, CatalogVersion{MetadataVersion: catalog.MetadataVersion(), Usage: usage})
	}
	return &Service{snapshot: Snapshot{LoadedAt: loadedAt.UTC(), Source: "BUILT_IN_CATALOGS", Tools: tools}}, nil
}

// Read 不探测节点、解析凭据或修改任何目录；取消及无效启动事实保持失败关闭。
func (s *Service) Read(ctx context.Context) (Snapshot, error) {
	if ctx == nil || ctx.Err() != nil || s == nil || s.snapshot.LoadedAt.IsZero() || len(s.snapshot.Tools) != 2 {
		return Snapshot{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if s.snapshot.LoadedAt.After(now) {
		return Snapshot{}, ErrUnavailable
	}
	item := s.snapshot
	item.CapturedAt = now
	item.Tools = make([]Tool, len(s.snapshot.Tools))
	for i, tool := range s.snapshot.Tools {
		item.Tools[i] = tool
		item.Tools[i].SupportedVersions = append([]string(nil), tool.SupportedVersions...)
		item.Tools[i].Modules = make([]Module, len(tool.Modules))
		for j, module := range tool.Modules {
			item.Tools[i].Modules[j] = module
			item.Tools[i].Modules[j].Catalogs = append([]CatalogVersion(nil), module.Catalogs...)
		}
	}
	return item, nil
}
