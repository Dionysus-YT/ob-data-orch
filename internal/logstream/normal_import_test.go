package logstream

import "testing"

func Test普通导入物理来源与缺口保持工具身份(t *testing.T) {
	for _, kind := range []SourceKind{SourceLoaderStdout, SourceLoaderStderr} {
		physical, err := persistedSourceKindFromRecord(kind)
		if err != nil {
			t.Fatal(err)
		}
		read, err := storedSourceKind(physical, "obloader-4.3.5-v1")
		if err != nil || read != kind {
			t.Fatal("导入身份丢失", err)
		}
		gap := GapNotice{StreamID: "synthetic", SourceKind: kind, SourceEpoch: 1, FirstSeq: 1, LastSeq: 2, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "synthetic", ParserVersion: "obloader-4.3.5-v1"}
		if _, err := GapDigest(gap); err != nil {
			t.Fatal("导入缺口被拒绝", err)
		}
		gap.SourceKind = "ARBITRARY"
		if _, err := GapDigest(gap); err == nil {
			t.Fatal("未知来源被接受")
		}
	}
}
