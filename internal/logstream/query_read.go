package logstream

import "context"

// ReadIndexedBatch 仅供内部授权查询端口读取已登记批次，沿用摘要验证和二次脱敏。
// 调用方须先过滤对象权限；本方法不接收浏览器路径，并与分段封存、清理互斥。
func (store *PersistentStore) ReadIndexedBatch(ctx context.Context, taskID string, batch IndexedBatch) ([]Record, error) {
	if store == nil || store.index == nil {
		return nil, ErrInvalidInput
	}
	store.readMu.RLock()
	defer store.readMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.ensureRecovered(ctx); err != nil {
		return nil, err
	}
	// 元数据取得后分段可能已封存；在读锁内重取同一批次，避免使用过期的 .open 路径。
	position := batchPosition(batch)
	current, found, err := store.index.NextTaskLogBatch(ctx, taskID, position, position, true)
	if err != nil {
		return nil, err
	}
	if !found || current.BatchID != batch.BatchID {
		return nil, ErrRetentionExpired
	}
	return store.readBatch(current)
}
