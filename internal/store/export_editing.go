package store

import "encoding/json"

// 编辑标记与独立能力版本必须成对存在，不能把编辑摘要伪装为已生成的执行指纹。
func validEditingExportMarker(configJSON, capability string) bool {
	var document struct {
		DraftState string        `json:"draftState"`
		Config     *ExportConfig `json:"config"`
	}
	if json.Unmarshal([]byte(configJSON), &document) != nil {
		return false
	}
	return (document.DraftState == "EDITING") == (capability == ExportEditingCapabilityVersion) &&
		(document.DraftState == "" || document.DraftState == "EDITING" && document.Config != nil)
}
