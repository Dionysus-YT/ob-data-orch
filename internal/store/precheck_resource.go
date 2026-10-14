package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"ob-data-orch/internal/executionbudget"
)

// readPrecheckResourceRequirement 从本次绑定的节点事实和已通过唯一生成器的冻结堆字段计算静态额度。
// 不查询其他主体的任务；缺失资源事实保留未知，不把默认额度当作机器事实。
func readPrecheckResourceRequirement(ctx context.Context, tx *sql.Tx, binding PrecheckBinding, configJSON string) (*executionbudget.PreflightRequirement, error) {
	var factsJSON sql.NullString
	var policyJSON string
	if err := tx.QueryRowContext(ctx, `SELECT a.facts_json, n.execution_budget_policy_json FROM agents a JOIN execution_nodes n ON n.node_id = a.node_id WHERE a.agent_id = ? AND a.node_id = ? AND a.facts_revision = ? AND a.status = 'ACTIVE'`, binding.BindingAgentID, binding.NodeID, binding.NodeFactsRevision).Scan(&factsJSON, &policyJSON); err != nil {
		return nil, ErrPrecheckLeaseRejected
	}
	var config struct {
		Config struct {
			PerformanceConfig PerformanceConfig `json:"performanceConfig"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(configJSON), &config) != nil {
		return nil, ErrPrecheckLeaseRejected
	}
	var policy executionbudget.Policy
	var facts AgentEnvironmentFacts
	if json.Unmarshal([]byte(policyJSON), &policy) == nil && factsJSON.Valid {
		parsed, err := decodeAgentEnvironmentFacts(factsJSON.String)
		if err == nil {
			facts = parsed
		}
	}
	requirement := executionbudget.Preflight(config.Config.PerformanceConfig.JvmMemory, facts.MemoryTotalBytes, policy)
	return &requirement, nil
}

// matchesPrecheckResource 保证上报的资源结论与控制面政策一致；历史报告只用于已有事实读取。
func matchesPrecheckResource(requirement *executionbudget.PreflightRequirement, results []PrecheckCheckResult) bool {
	expected := "UNKNOWN"
	if requirement != nil {
		ready, known := requirement.Qualification()
		if known {
			expected = "FAILED"
			if ready {
				expected = "PASSED"
			}
		}
	}
	for _, result := range results {
		if result.Check == "NODE_RESOURCE" {
			// 探针异常可以安全回报未知并结束失败报告；只有通过/不通过结论需要吻合静态政策。
			return result.Status == "UNKNOWN" || result.Status == expected
		}
	}
	return false
}

// validateSubmissionPrecheckProof 在新提交时复核完整报告，旧任务的历史读取不经过该门禁。
func validateSubmissionPrecheckProof(ctx context.Context, tx *sql.Tx, binding PrecheckBinding) error {
	executionContext, err := readPrecheckExecutionContext(ctx, tx, binding)
	if err != nil {
		return ErrPrecheckInvalid
	}
	var resultJSON string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(result_json, '') FROM precheck_runs WHERE precheck_id = ?`, binding.PrecheckID).Scan(&resultJSON); err != nil {
		return ErrPrecheckInvalid
	}
	results := decodePrecheckResults(resultJSON)
	expected := fixedPrecheckChecks
	if executionContext.OutputKind != "" && executionContext.OutputKind != "LOCAL" {
		expected = storagePrecheckCheckList()
	}
	if !matchesPrecheckShape(results, expected) || !matchesPrecheckResource(executionContext.ResourceRequirement, results) {
		return ErrPrecheckInvalid
	}
	for _, result := range results {
		if result.Status != "PASSED" {
			return ErrPrecheckInvalid
		}
	}
	return nil
}
