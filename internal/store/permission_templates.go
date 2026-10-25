package store

import (
	"context"
	"database/sql"
)

// templatePermissionPredicate 只接收源码固定模式；主体通过绑定参数传入，范围与角色在同次 SQL 中复验。
// READ 供内部类型检查读取；派生与修改还必须分别复验 USE、MANAGE，不能互相推导。
func templatePermissionPredicate(mode string) string {
	use := `(EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=pa.subject_id AND pg.grant_code='ROLE_BASIC_USER') AND (owner_subject_id=pa.subject_id OR EXISTS(SELECT 1 FROM subject_object_scopes ps WHERE ps.subject_id=pa.subject_id AND ps.scope_type='TEMPLATE_USE' AND ps.object_id=template_id)))`
	manage := `(EXISTS(SELECT 1 FROM subject_grants pg WHERE pg.subject_id=pa.subject_id AND pg.grant_code='ROLE_OPERATOR') AND (owner_subject_id=pa.subject_id OR EXISTS(SELECT 1 FROM subject_object_scopes ps WHERE ps.subject_id=pa.subject_id AND ps.scope_type='TEMPLATE_MANAGE' AND ps.object_id=template_id)))`
	permission := use + " OR " + manage
	if mode == "USE" {
		permission = use
	}
	if mode == "MANAGE" {
		permission = manage
	}
	return `EXISTS(SELECT 1 FROM auth_subjects pa WHERE pa.subject_id=? AND pa.account_status='ACTIVE' AND ((NOT ` + managedPermissionExists + ` AND owner_subject_id=pa.subject_id) OR (` + managedPermissionExists + ` AND (` + permission + `))))`
}

func checkTemplateManagement(ctx context.Context, tx *sql.Tx, subjectID, templateID string) error {
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM export_config_templates WHERE template_id=? AND `+templatePermissionPredicate("MANAGE")+`)`, templateID, subjectID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrDataSourceNotFound
	}
	return nil
}
