package agentwire

import (
	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/precheckcontract"
)

func validDirectLoadContext(c PrecheckExecutionContext) bool {
	return c.NormalImport == nil && agentpreflight.ValidDirectLoadContext(c.DirectLoad, c.CheckKind, c.CompatibilityMode, c.TargetPlatform, c.AllowedRoots) && c.Database == c.DirectLoad.Database && c.ContentKind == "DATA_ONLY" &&
		c.OutputPath == "" && c.LogPath == "" && c.OutputKind == "" && c.StorageTarget == nil && !c.SkipCheckDir && !c.WeakRead && !c.EnableHiddenPk && !c.AddExtraMessage &&
		len(c.Objects) == 0 && len(c.ObjectTypes) == 0 && len(c.IncludeColumnNames) == 0 && len(c.ExcludeColumnNames) == 0 && len(c.Partitions) == 0
}

func validDirectLoadGrant(g PrecheckGrant, nodeID string) bool {
	expected := precheckcontract.DirectLoadChecks(g.Context.CheckKind)
	if len(expected) == 0 || len(expected) != len(g.CheckSet) {
		return false
	}
	for i, name := range expected {
		if string(g.CheckSet[i]) != name {
			return false
		}
	}
	return validOpaqueValue(g.PrecheckID, 256) && validOpaqueValue(g.LeaseID, 256) && g.LeaseEpoch > 0 && !g.ExpiresAt.IsZero() && validOpaqueValue(g.BindingDigest, 256) && validPrecheckBinding(g.Binding) && g.Binding.PrecheckID == g.PrecheckID && g.Binding.NodeID == nodeID && g.Context.DirectLoad.NodeID == nodeID && validDirectLoadContext(g.Context)
}
