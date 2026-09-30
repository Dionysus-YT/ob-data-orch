package credential

// PreparePrivateDirectory 创建并限制运行数据目录，仅允许运行账户与明确管理员访问。
func PreparePrivateDirectory(path string) error { return ensurePrivateDirectory(path) }
