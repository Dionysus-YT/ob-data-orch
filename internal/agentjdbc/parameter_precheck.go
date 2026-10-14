package agentjdbc

import (
	"context"
	"errors"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

// runParameterPreflight 与普通连接探针使用同一已确认租约和一次槽位解析，但不复用普通检查结果。
func (p *PrecheckProbe) runParameterPreflight(ctx context.Context, workspace credential.Workspace, runtime jdbcprobe.Runtime, request agentpreflight.Request, connection Connection) (agentpreflight.Result, agentpreflight.Result) {
	objects := make([]jdbcprobe.PreflightObject, 0, len(request.Objects))
	for index, name := range request.Objects {
		kind := "TABLE"
		if len(request.ObjectTypes) != 0 {
			kind = request.ObjectTypes[index]
		}
		objects = append(objects, jdbcprobe.PreflightObject{Type: kind, Name: name})
	}
	input := jdbcprobe.ParameterPreflightRequest{
		BatchPreflightRequest: jdbcprobe.BatchPreflightRequest{
			Connection:        jdbcprobe.Request{Host: connection.Host, Port: connection.Port, Username: connection.Username, Password: connection.Password},
			CompatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode), Database: request.Database, Objects: objects,
			IncludeColumnNames: append([]string(nil), request.IncludeColumnNames...), ExcludeColumnNames: append([]string(nil), request.ExcludeColumnNames...), Partitions: append([]string(nil), request.Partitions...),
		},
		WeakRead: request.WeakRead, EnableHiddenPk: request.EnableHiddenPk, AddExtraMessage: request.AddExtraMessage,
	}
	if request.AddExtraMessage {
		input.SysConnection = jdbcprobe.Request{Host: connection.Host, Port: connection.Port, Username: connection.SysUsername, Password: connection.SysPassword}
	}
	run := p.RunParameters
	if run == nil {
		run = func(ctx context.Context, _ credential.Workspace, runtime jdbcprobe.Runtime, input jdbcprobe.ParameterPreflightRequest) (jdbcprobe.PreflightResult, error) {
			return jdbcprobe.TestParameterPreflight(ctx, runtime, input)
		}
	}
	result, err := run(ctx, workspace, runtime, input)
	if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
		return failedDatabaseResult(), unavailableObjectResult()
	}
	if err != nil {
		return unavailableDatabaseResult(), unavailableObjectResult()
	}
	switch result.ObjectAccess {
	case jdbcprobe.ObjectAccessible:
		return connectedDatabaseResult(), agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusPassed, EvidenceCode: "PARAMETER_REQUIREMENTS_VERIFIED"}
	case jdbcprobe.ObjectNotAccessible:
		return connectedDatabaseResult(), agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusFailed, EvidenceCode: "PARAMETER_REQUIREMENTS_REJECTED"}
	default:
		return connectedDatabaseResult(), unavailableObjectResult()
	}
}
