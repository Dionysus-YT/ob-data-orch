package agentexec

import (
	"reflect"
	"testing"
)

func Test冻结堆替换默认参数且不接受第二份堆配置(t *testing.T) {
	profile := []string{"-server", "-Xms4G", "-Xmx4G", "main"}
	actual, err := applyFrozenJVMHeap(profile, []string{"--mem", "8G"}, nil)
	if err != nil || !reflect.DeepEqual(actual, []string{"-server", "-Xms8388608K", "-Xmx8388608K", "main"}) {
		t.Fatalf("堆替换失败: %v %v", actual, err)
	}
	if profile[1] != "-Xms4G" {
		t.Fatal("修改了共享启动模板")
	}
	for _, options := range [][]string{{"-Xms1G"}, {"-Xmx16G"}, {"-XX:MaxHeapSize=1024"}, {"-XX:InitialHeapSize=1024"}} {
		if _, err := applyFrozenJVMHeap(profile, nil, options); err == nil {
			t.Fatal("第二份堆配置未被拒绝")
		}
	}
	if _, err := applyFrozenJVMHeap(profile, []string{"--mem", "1G", "--mem", "2G"}, nil); err == nil {
		t.Fatal("重复内存参数未被拒绝")
	}
}
