package store

// ObserverDirectCommandIdentity 只为已登记直连源生成 username@tenant，不携带 ODP 集群后缀。
// 调用方仍须限定受控命令用途，不得将组合身份写入日志或审计。
func ObserverDirectCommandIdentity(username, tenantName string) (string, bool) {
	return PrivateODPCommandIdentity(username, tenantName, "")
}

// validLocalConnectionKind 限定本地两种连接路径；直连不能夹带代理集群路由。
func validLocalConnectionKind(kind, cluster string) bool {
	return kind == "ODP" || kind == "OBSERVER_DIRECT" && cluster == ""
}

// composeLocalJDBCIdentity 仅在当前授权绑定下组装短时身份，返回字节必须由调用方清零。
func composeLocalJDBCIdentity(kind, username, tenant, cluster string) ([]byte, bool) {
	if !validLocalConnectionKind(kind, cluster) {
		return nil, false
	}
	return composePrivateODPJDBCIdentity(username, tenant, cluster)
}
