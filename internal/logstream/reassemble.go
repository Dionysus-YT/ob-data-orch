package logstream

import (
	"bytes"
	"strings"
)

// Reassembler 只在完整 LF 或 CRLF 边界后生成记录。
// 每个来源流和 epoch 都必须使用独立实例，避免跨来源拼接原始字节。
type Reassembler struct {
	buffer   []byte
	dropping bool
}

// Push 保留既有字符串调用入口；真实工具管道应改用 PushBytes。
func (r *Reassembler) Push(chunk []byte) ([]string, bool) {
	byteRecords, truncated := r.PushBytes(chunk)
	records := make([]string, 0, len(byteRecords))
	for _, record := range byteRecords {
		records = append(records, strings.ToValidUTF8(string(record), "�"))
		zero(record)
	}
	return records, truncated
}

// PushBytes 只返回完整记录的原始字节副本，供调用方先脱敏后再转换为文本。
// 已消费的内部缓冲区会立即清零，避免重组器长期保留工具原文。
func (r *Reassembler) PushBytes(chunk []byte) ([][]byte, bool) {
	if len(chunk) == 0 {
		return nil, false
	}
	r.buffer = append(r.buffer, chunk...)
	var records [][]byte
	truncated := false
	for {
		index := bytes.IndexByte(r.buffer, '\n')
		if index < 0 {
			if len(r.buffer) > MaxRecordBytes {
				zero(r.buffer)
				r.buffer = r.buffer[:0]
				r.dropping = true
				truncated = true
			}
			break
		}
		line := r.buffer[:index]
		lineCopy := append([]byte(nil), line...)
		zero(r.buffer[:index+1])
		r.buffer = r.buffer[index+1:]
		if len(lineCopy) > 0 && lineCopy[len(lineCopy)-1] == '\r' {
			lineCopy = lineCopy[:len(lineCopy)-1]
		}
		if r.dropping || len(lineCopy) > MaxRecordBytes {
			zero(lineCopy)
			r.dropping = false
			truncated = true
			continue
		}
		records = append(records, lineCopy)
	}
	return records, truncated
}

// Finish 保留既有字符串调用入口；真实工具管道应改用 FinishBytes。
func (r *Reassembler) Finish() ([]string, bool) {
	byteRecords, truncated := r.FinishBytes()
	records := make([]string, 0, len(byteRecords))
	for _, record := range byteRecords {
		records = append(records, strings.ToValidUTF8(string(record), "�"))
		zero(record)
	}
	return records, truncated
}

// FinishBytes 在来源结束时释放最后一条完整记录；没有安全边界的超长尾部只产生截断信号。
func (r *Reassembler) FinishBytes() ([][]byte, bool) {
	if r.dropping || len(r.buffer) > MaxRecordBytes {
		zero(r.buffer)
		r.buffer = nil
		r.dropping = false
		return nil, true
	}
	if len(r.buffer) == 0 {
		return nil, false
	}
	line := append([]byte(nil), r.buffer...)
	zero(r.buffer)
	r.buffer = nil
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return [][]byte{line}, false
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
