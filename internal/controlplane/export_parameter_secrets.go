package controlplane

import (
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/store"
)

// commandSysReference 只从唯一命令生成器读取已经冻结的 sys 引用。
func commandSysReference(result commandgen.Result) commandgen.CredentialReference {
	for _, slot := range result.SecretSlots {
		if slot.SlotID == "sys-password" && slot.SecurityProperty == "oceanbase.jdbc.sys.password" {
			return slot.CredentialReference
		}
	}
	return commandgen.CredentialReference{}
}

// taskSysSnapshot 将引用写入任务私有快照；编码失败返回空值，由提交验证失败关闭。
func taskSysSnapshot(snapshot string, result commandgen.Result) string {
	reference := commandSysReference(result)
	if reference.CredentialID == "" {
		return snapshot
	}
	content, err := store.FreezeTaskSysCredential(snapshot, reference.CredentialID, reference.Revision)
	if err != nil {
		return ""
	}
	return content
}
