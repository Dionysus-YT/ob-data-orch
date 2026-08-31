package exportdomain

import "ob-data-orch/internal/store"

// Draft 是 v5 或 v6 草稿归一化后的统一命令输入模型。
// 它只描述已经通过产品领域规则的配置事实，不包含 HTTP、身份、凭据明文或执行状态。
type Draft struct {
	DataSourceID  string
	NodeID        string
	Database      string
	ScopeKind     string // ALL | SPECIFIED
	ObjectType    string // TABLE | VIEW；ALL 范围时为空
	Objects       []string
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
	// 条件筛选与一致性快照。
	Where    string
	Snapshot bool
	// 分区筛选与数据类型排除。
	Partition        string
	ExcludeDataTypes []string
	// 仅 MySQL CSV/CUT 数据导出的 DATE/DATETIME 值格式已启用。
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
		n.QuerySql == "" && len(n.IncludeColumns) == 0 && len(n.ExcludeColumns) == 0 &&
		!n.ExcludeVirtualColumns && n.FlashbackScn == nil && n.FlashbackTimestamp == "" &&
		n.Thread == nil && n.PageSize == nil && n.ParallelMacro == nil && n.FetchSize == nil && n.JvmMemory == "" && n.BlockSize == "" &&
		!n.DropObject && !n.RetainSchema && !n.CompactSchema && n.Where == "" && !n.Snapshot &&
		n.Partition == "" && len(n.ExcludeDataTypes) == 0 && n.TimestampFormats == (store.TimestampFormatConfig{})
}
