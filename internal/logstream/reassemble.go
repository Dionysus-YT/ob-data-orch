package logstream

import (
	"bytes"
	"strings"
)

// Reassembler creates records only after a full LF or CRLF boundary. It is
// source-local: callers must create one instance per stream/epoch.
type Reassembler struct {
	buffer   []byte
	dropping bool
}

func (r *Reassembler) Push(chunk []byte) ([]string, bool) {
	if len(chunk) == 0 {
		return nil, false
	}
	r.buffer = append(r.buffer, chunk...)
	var records []string
	truncated := false
	for {
		index := bytes.IndexByte(r.buffer, '\n')
		if index < 0 {
			if len(r.buffer) > MaxRecordBytes {
				r.buffer = r.buffer[:0]
				r.dropping = true
				truncated = true
			}
			break
		}
		line := r.buffer[:index]
		r.buffer = r.buffer[index+1:]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if r.dropping || len(line) > MaxRecordBytes {
			r.dropping = false
			truncated = true
			continue
		}
		records = append(records, strings.ToValidUTF8(string(line), "�"))
	}
	return records, truncated
}

// Finish flushes a final complete record when the source ends. If an oversized
// tail never reaches a safe boundary, it becomes a truncation without text.
func (r *Reassembler) Finish() ([]string, bool) {
	if r.dropping || len(r.buffer) > MaxRecordBytes {
		r.buffer = nil
		r.dropping = false
		return nil, true
	}
	if len(r.buffer) == 0 {
		return nil, false
	}
	line := r.buffer
	r.buffer = nil
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return []string{strings.ToValidUTF8(string(line), "�")}, false
}
