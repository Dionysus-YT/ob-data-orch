package jdbcprobe

import (
	"encoding/binary"
	"testing"
)

func Test选择协议不改变既有协议且拒绝非法名称(t *testing.T) {
	base := BatchPreflightRequest{IncludeColumnNames: []string{"id"}, Partitions: []string{"sp0"}}
	input := encodeBatchPreflightRequest(base)
	defer zero(input)
	if binary.BigEndian.Uint32(input[:4]) != 7 {
		t.Fatal("列与分区未使用固定扩展协议")
	}
	parameter := encodeParameterPreflightRequest(ParameterPreflightRequest{BatchPreflightRequest: base, WeakRead: true})
	defer zero(parameter)
	if binary.BigEndian.Uint32(parameter[:4]) != 8 {
		t.Fatal("参数与选择未使用固定扩展协议")
	}
	base.IncludeColumnNames, base.Partitions = nil, nil
	legacy := encodeBatchPreflightRequest(base)
	defer zero(legacy)
	if binary.BigEndian.Uint32(legacy[:4]) != 5 {
		t.Fatal("无筛选时改变了既有协议")
	}
	base.IncludeColumnNames = []string{"id", "id"}
	if validSelection(base) {
		t.Fatal("重复名称被发送给 JDBC")
	}
}
