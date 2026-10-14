package agentlocalpreflight

import "ob-data-orch/internal/agentpreflight"

// nodeResourceResult 只复核控制面提供的配置额度，不推断物理内存或替代实时资源分配。
func nodeResourceResult(request agentpreflight.Request) agentpreflight.Result {
	check := agentpreflight.CheckNodeResource
	if request.ResourceRequirement == nil {
		return unavailableResult(check)
	}
	ready, known := request.ResourceRequirement.Qualification()
	if !known {
		return unavailableResult(check)
	}
	if !ready {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: "NODE_RESOURCE_INSUFFICIENT"}
	}
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "NODE_RESOURCE_READY"}
}
