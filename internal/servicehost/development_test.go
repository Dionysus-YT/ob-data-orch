package servicehost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentStopMarkerCancelsOnlyOwnedContext(t *testing.T) {
	parent := context.Background()
	directory := t.TempDir()
	ctx, cancel := DevelopmentContext(parent, directory)
	defer cancel()
	if err := os.WriteFile(filepath.Join(directory, "dev.stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("未接收开发停止标记")
	}
	if parent.Err() != nil {
		t.Fatal("影响了父上下文")
	}
}
