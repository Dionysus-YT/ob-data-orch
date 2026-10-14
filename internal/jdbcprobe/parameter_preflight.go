package jdbcprobe

import (
	"context"
	"encoding/binary"
)

const parameterPreflightProtocolVersion = 6

// ParameterPreflightRequest 只允许三个固定参数检查和冻结对象，不接收 SQL、URL 或驱动。
// SysConnection 复用业务连接的端点；仅 AddExtraMessage 时短时携带 sys 身份和密码。
type ParameterPreflightRequest struct {
	BatchPreflightRequest
	WeakRead        bool
	EnableHiddenPk  bool
	AddExtraMessage bool
	SysConnection   Request
}

// TestParameterPreflight 使用固定 v6 或含选择列表的 v8 标准输入协议复核参数与元数据。
// 普通 v1/v3/v5 探针结果不能代替这些检查；输入与输出缓冲返回前清零。
func TestParameterPreflight(ctx context.Context, runtime Runtime, request ParameterPreflightRequest) (PreflightResult, error) {
	if !validRuntime(runtime) {
		return PreflightResult{}, ErrInvalidRuntime
	}
	if !validParameterPreflightRequest(request) {
		return PreflightResult{}, ErrInvalidRequest
	}
	input := encodeParameterPreflightRequest(request)
	defer zero(input)
	output, err := runProbe(ctx, runtime, input)
	if err != nil {
		return PreflightResult{}, err
	}
	defer zero(output)
	return parsePreflightResponse(output)
}

func validParameterPreflightRequest(request ParameterPreflightRequest) bool {
	if !request.WeakRead && !request.EnableHiddenPk && !request.AddExtraMessage {
		return false
	}
	base := request.BatchPreflightRequest
	if !validSelection(base) {
		return false
	}
	if len(base.Objects) == 0 {
		// ALL 由探针按冻结数据库完整枚举表，仅用于固定参数检查。
		if !validRequest(base.Connection) || !validCompatibilityMode(base.CompatibilityMode) || !validObjectIdentifier(base.Database) {
			return false
		}
	} else if !validBatchPreflightRequest(base) {
		return false
	}
	if request.AddExtraMessage {
		return validRequest(request.SysConnection) && request.SysConnection.Host == request.Connection.Host && request.SysConnection.Port == request.Connection.Port
	}
	return len(request.SysConnection.Username) == 0 && len(request.SysConnection.Password) == 0
}

func encodeParameterPreflightRequest(request ParameterPreflightRequest) []byte {
	base := encodeBatchPreflightRequest(request.BatchPreflightRequest)
	input := make([]byte, len(base), len(base)+12+len(request.SysConnection.Username)+len(request.SysConnection.Password))
	copy(input, base)
	zero(base)
	binary.BigEndian.PutUint32(input[:4], parameterPreflightProtocolVersion)
	if request.BatchPreflightRequest.HasSelection() {
		binary.BigEndian.PutUint32(input[:4], 8)
	}
	flags := uint32(0)
	if request.WeakRead {
		flags |= 1
	}
	if request.EnableHiddenPk {
		flags |= 2
	}
	if request.AddExtraMessage {
		flags |= 4
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], flags)
	input = append(input, header[:]...)
	for _, value := range [][]byte{request.SysConnection.Username, request.SysConnection.Password} {
		binary.BigEndian.PutUint32(header[:], uint32(len(value)))
		input = append(input, header[:]...)
		input = append(input, value...)
	}
	return input
}
