package exportdomain

import "ob-data-orch/internal/store"

// Draft 是 v5 或 v6 草稿归一化后的统一命令输入模型。
// 它只描述已经通过产品领域规则的配置事实，不包含 HTTP、身份、凭据明文或执行状态。
type Draft struct {
	DataSourceID  string
	NodeID        string
	Database      string
	ScopeKind     string // ALL | SPECIFIED | QUERY_RESULT
	ObjectType    string // 单类型范围的对象类型；ALL 或混选时为空
	Objects       []string
	ObjectsByType map[string][]string // 混选时按类型保存名称，避免同名对象失去归属
	ExcludeTables []string
	ContentKind   string // DATA_ONLY | DDL_ONLY | DDL_AND_DATA
	Format        string // CSV | CUT | SQL | POS | PARQUET | ORC | AVRO；仅 DDL 时为空
	FilePath      string
	LogPath       string
	SkipCheckDir  bool

	// 格式序列化、压缩、文件布局、筛选与资源参数。
	CsvOptions      store.CsvOptions
	CutOptions      store.CutOptions
	Compress        bool
	CompressionAlgo string
	// CompressionLevel 按官方算法分范围：zstd 1-22、zlib -1~9；gzip/snappy 不支持。
	CompressionLevel *int64
	NoNestedDir      bool
	MaxFileSize      *int64
	RetainEmptyFiles bool
	QuerySql         string
	QueryResultLimit *int64
	IncludeColumns   []string
	ExcludeColumns   []string
	// ExcludeVirtualColumns、闪回和资源参数保持与 OBDUMPER 领域契约一致。
	ExcludeVirtualColumns bool
	FlashbackScn          *int64
	FlashbackTimestamp    string
	Thread                *int
	PageSize              *int
	ParallelMacro         *int
	FetchSize             *int
	JvmMemory             string
	// BlockSize 是数字（MB）或数字+MB/ROW 后缀。
	BlockSize string
	// ControlFilePath 只对 POS 格式有效。
	ControlFilePath string
	// TmpPath 是对象存储 Multipart 本地临时分块目录。
	TmpPath string
	// OutputKind 是 LOCAL 或受控对象存储类型。
	OutputKind string
	// DDL 行为仅在包含 DDL 的内容中活动。
	DropObject    bool
	RetainSchema  bool
	CompactSchema bool
	// CompactSchemaValue 保留显式 false；nil 继续表示继承工具默认，历史 flag 仍由原版本解释。
	CompactSchemaValue *bool
	// AddExtraMessage 依赖当前数据源的 sys 凭据及固定权限预检查。
	AddExtraMessage bool
	// SequencePolicy 仅包含序列 DDL 时活动；命令值使用官方小写枚举。
	SequencePolicy string
	// 条件筛选与一致性快照。
	Where    string
	Snapshot bool
	// WeakRead 与 EnableHiddenPk 必须通过固定对象及参数预检查，不能以连接成功替代。
	WeakRead       bool
	EnableHiddenPk bool
	// 分区筛选与数据类型排除。
	Partition        string
	ExcludeDataTypes []string
	// TimestampFormats 按格式与兼容模式校验值格式和 Oracle 会话格式。
	TimestampFormats store.TimestampFormatConfig
	// 对象存储输出的凭据引用；本地输出保持为空。
	StorageCredential *store.StorageCredentialBinding
}

// IsFrozenSingleTableCSV 判断归一结果是否仍属于首条切片的冻结形状。
// 该形状必须继续使用 v5 元数据和原有能力版本，保证历史指纹与 argv 不变。
func (n Draft) IsFrozenSingleTableCSV() bool {
	return (n.OutputKind == "" || n.OutputKind == "LOCAL") && n.TmpPath == "" && n.ScopeKind == "SPECIFIED" && n.ObjectType == "TABLE" && len(n.Objects) == 1 && len(n.ExcludeTables) == 0 && n.ContentKind == "DATA_ONLY" && n.Format == "CSV" && n.HasZeroOptions()
}

// HasZeroOptions 判断全部泛化选项是否保持零值。
func (n Draft) HasZeroOptions() bool {
	return n.CsvOptions == (store.CsvOptions{}) && n.CutOptions == (store.CutOptions{}) && !n.Compress && n.CompressionAlgo == "" && n.CompressionLevel == nil &&
		!n.NoNestedDir && n.MaxFileSize == nil && !n.RetainEmptyFiles &&
		n.QuerySql == "" && n.QueryResultLimit == nil && len(n.IncludeColumns) == 0 && len(n.ExcludeColumns) == 0 &&
		!n.ExcludeVirtualColumns && n.FlashbackScn == nil && n.FlashbackTimestamp == "" &&
		n.Thread == nil && n.PageSize == nil && n.ParallelMacro == nil && n.FetchSize == nil && n.JvmMemory == "" && n.BlockSize == "" &&
		!n.DropObject && !n.RetainSchema && !n.CompactSchema && n.CompactSchemaValue == nil && !n.AddExtraMessage && n.SequencePolicy == "" && n.Where == "" && !n.Snapshot && !n.WeakRead && !n.EnableHiddenPk &&
		n.Partition == "" && len(n.ExcludeDataTypes) == 0 && n.TimestampFormats == (store.TimestampFormatConfig{})
}
