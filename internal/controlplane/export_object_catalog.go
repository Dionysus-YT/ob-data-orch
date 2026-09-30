package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// exportObjectCatalogRequest 只接受导出向导固定对象类型和有界名称筛选。
// 数据源连接材料一律由受控节点租约解析，不接受浏览器提供连接串、SQL 或驱动。
type exportObjectCatalogRequest struct {
	NodeID     string `json:"nodeId"`
	Database   string `json:"database"`
	ObjectType string `json:"objectType"`
	Keyword    string `json:"keyword"`
}

func parseExportObjectCatalogAction(path string) (string, bool) {
	const prefix = "/api/v1/data-sources/"
	const suffix = ":search-export-objects"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, validDataSourceConnectionTestPathID(id)
}

// startExportObjectCatalogQuery 在已授权数据源和节点范围内冻结一次短时元数据查询。
// 它不建立数据库连接，也不修改基础连接测试事实；Agent 仍需通过现有租约和秘密槽位。
func (s *Server) startExportObjectCatalogQuery(w http.ResponseWriter, r *http.Request, principal identity.Principal, dataSourceID string) {
	if s.connectionTests == nil || s.authorizer == nil || s.csrf == nil || !s.agentJDBCConnectionTestEnabled || s.connectionTestTTL <= 0 || s.heartbeatTTL <= 0 {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_OBJECT_CATALOG_UNAVAILABLE", "当前环境尚未启用节点侧对象查询", true)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, http.StatusUnauthorized, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, dataSourceID) != nil {
		notFound(w, r)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, http.StatusBadRequest, "REVISION_REQUIRED", "需要有效的数据源版本号", false)
		return
	}
	var request exportObjectCatalogRequest
	if !decodeBrowserJSON(w, r, &request) {
		return
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	if !validExportCatalogRequest(request) {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_OBJECT_CATALOG_FIELDS_INVALID", "对象查询条件无效", false)
		return
	}
	if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, request.NodeID) != nil {
		notFound(w, r)
		return
	}
	queryID := newOpaqueID()
	if queryID == "" {
		writeError(w, http.StatusServiceUnavailable, "IDENTIFIER_UNAVAILABLE", "服务暂时不可用", true)
		return
	}
	now := time.Now().UTC()
	digestInput, err := json.Marshal(struct {
		Operation    string `json:"operation"`
		DataSourceID string `json:"dataSourceId"`
		Revision     int64  `json:"revision"`
		NodeID       string `json:"nodeId"`
		Database     string `json:"database"`
		ObjectType   string `json:"objectType"`
		Keyword      string `json:"keyword"`
	}{"EXPORT_OBJECT_CATALOG", dataSourceID, revision, request.NodeID, request.Database, request.ObjectType, request.Keyword})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_OBJECT_CATALOG_UNAVAILABLE", "当前无法创建对象查询", true)
		return
	}
	digest := sha256.Sum256(digestInput)
	result, err := s.connectionTests.RequestDataSourceConnectionTest(r.Context(), store.DataSourceConnectionTestCreate{
		ConnectionTestID: queryID, DataSourceID: dataSourceID, CreatorSubjectID: principal.ID,
		ExpectedDataSourceRevision: revision, NodeID: request.NodeID, VerificationSource: "AGENT_JDBC",
		OperationKind: "EXPORT_OBJECT_CATALOG", CatalogDatabase: request.Database,
		CatalogObjectType: request.ObjectType, CatalogKeyword: request.Keyword,
		RequestID: requestID(w), IdempotencyKey: key, RequestDigest: hex.EncodeToString(digest[:]),
		CreatedAt: now, HeartbeatFreshAfter: now.Add(-s.heartbeatTTL), ValidUntil: now.Add(s.connectionTestTTL),
	})
	if errors.Is(err, store.ErrDataSourceNotFound) || errors.Is(err, store.ErrExecutionNodeNotFound) {
		notFound(w, r)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusPreconditionFailed, "DATA_SOURCE_REVISION_CONFLICT", "数据源已发生变化，请刷新后重试", false)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	}
	if errors.Is(err, store.ErrDataSourceConnectionTestInvalid) || errors.Is(err, store.ErrDataSourceConnectionTestLeaseRejected) {
		writeError(w, http.StatusUnprocessableEntity, "EXPORT_OBJECT_CATALOG_NODE_UNAVAILABLE", "数据源或执行节点当前不满足对象查询条件", false)
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_OBJECT_CATALOG_UNAVAILABLE", "当前无法创建对象查询", true)
		return
	}
	run, err := s.connectionTests.GetDataSourceConnectionTestRun(r.Context(), result.ConnectionTestID)
	if err != nil || run.OperationKind != "EXPORT_OBJECT_CATALOG" || run.CreatorSubjectID != principal.ID {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_OBJECT_CATALOG_UNAVAILABLE", "当前无法核对对象查询状态", true)
		return
	}
	status := http.StatusAccepted
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"requestId": requestID(w), "item": catalogQueryResponse(run, now)})
}

// validExportCatalogRequest 将数据库目录限制为空数据库名，对象目录则必须指定数据库。
func validExportCatalogRequest(request exportObjectCatalogRequest) bool {
	if !validDataSourceConnectionTestPathID(request.NodeID) || !validCatalogInput(request.Keyword, 100, true) {
		return false
	}
	if request.ObjectType == "DATABASE" {
		return request.Database == ""
	}
	return (request.ObjectType == "TABLE" || request.ObjectType == "VIEW") && validCatalogInput(request.Database, 256, false)
}

// getExportObjectCatalogQuery 仅向发起者和当前仍可读取该数据源的主体返回短时对象名结果。
func (s *Server) getExportObjectCatalogQuery(w http.ResponseWriter, r *http.Request, principal identity.Principal, queryID string) {
	if s.connectionTests == nil || s.authorizer == nil {
		writeError(w, http.StatusServiceUnavailable, "EXPORT_OBJECT_CATALOG_UNAVAILABLE", "对象查询暂时不可用", true)
		return
	}
	run, err := s.connectionTests.GetDataSourceConnectionTestRun(r.Context(), queryID)
	if err != nil || run.OperationKind != "EXPORT_OBJECT_CATALOG" || run.CreatorSubjectID != principal.ID ||
		identity.Can(r.Context(), s.authorizer, principal, identity.ScopeDataSourceRead, run.DataSourceID) != nil ||
		identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeUse, run.NodeID) != nil {
		notFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID(w), "item": catalogQueryResponse(run, time.Now().UTC())})
}

func catalogQueryResponse(run store.DataSourceConnectionTestRun, now time.Time) map[string]any {
	status := run.Status
	if !run.ValidUntil.After(now) || (status == "PENDING" && !run.CreatedAt.Add(store.ExportObjectCatalogClaimTimeout).After(now)) {
		status = "EXPIRED"
	}
	objects := []string{}
	truncated := false
	if status == "SUCCEEDED" {
		objects = append(objects, run.CatalogObjects...)
		truncated = run.CatalogTruncated
	}
	return map[string]any{
		"id": run.ConnectionTestID, "status": status, "dataSourceId": run.DataSourceID,
		"nodeId": run.NodeID, "database": run.CatalogDatabase, "objectType": run.CatalogObjectType,
		"keyword": run.CatalogKeyword, "objects": objects, "truncated": truncated,
		"validUntil": run.ValidUntil.UTC().Format(time.RFC3339Nano),
	}
}

func validCatalogInput(value string, maximum int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || len(value) > maximum || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
