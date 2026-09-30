package store

import (
	"context"
	"database/sql"
)

// AgentRuntimeConfiguration 在同一事务中核验当前机器身份并返回其绑定节点的受限配置。
// 调用方不能指定其他节点；撤销身份后也不能继续取得配置。
func (s *Store) AgentRuntimeConfiguration(ctx context.Context, digest []byte) (AgentEnrollmentRuntimeConfiguration, error) {
	var result AgentEnrollmentRuntimeConfiguration
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	agent, err := authenticateAgentDigest(ctx, tx, digest)
	if err != nil {
		return result, err
	}
	result, err = loadAgentEnrollmentRuntimeConfiguration(ctx, tx, agent.NodeID)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}
