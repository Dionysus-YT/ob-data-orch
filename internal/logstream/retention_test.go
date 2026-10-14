package logstream

import "testing"

func Test普通日志保留配置失败关闭(t *testing.T) {
	for _, value := range []int{0, 7, 30, 365} {
		days, err := RetentionDays(value)
		if err != nil || days < 7 || days > 365 {
			t.Fatalf("有效配置 %d=%d err=%v", value, days, err)
		}
	}
	for _, value := range []int{-1, 1, 6, 366, 10000} {
		if _, err := RetentionDays(value); err == nil {
			t.Fatalf("非法保留天数被接受：%d", value)
		}
	}
}
