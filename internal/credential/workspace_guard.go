package credential

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrWorkspaceResidual 表示私有材料尚未完成清理，必须先核对残留再接收新任务。
var ErrWorkspaceResidual = errors.New("执行私有材料存在待核对残留")

// 护栏在写入任何秘密前持久化；内存登记仅豁免本进程仍在负责的工作区。
// 重启会丢失豁免，清理失败也会撤销豁免，不能靠更换 execution 标识绕过残留。
var workspaceGuards = struct {
	sync.Mutex
	active map[string]bool
}{active: make(map[string]bool)}

func workspaceGuardPath(executionRoot string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(executionRoot)), "workspace-guards", filepath.Base(executionRoot))
}

// CheckWorkspaceRoot 在领取或创建工作区前检查持久护栏；读取失败同样阻断。
// 本进程内正常并行执行的工作区仍可共存，其他进程或重启遗留不能被默认为安全。
func CheckWorkspaceRoot(root string) error {
	if root == "" {
		return ErrWorkspaceResidual
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ErrWorkspaceResidual
	}
	workspaceGuards.Lock()
	defer workspaceGuards.Unlock()
	return checkWorkspaceRootLocked(absRoot)
}

func checkWorkspaceRootLocked(root string) error {
	directory := filepath.Join(root, "workspace-guards")
	info, err := os.Lstat(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return ErrWorkspaceResidual
	}
	entries, err := os.ReadDir(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrWorkspaceResidual
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !workspaceGuards.active[filepath.Join(directory, entry.Name())] {
			return ErrWorkspaceResidual
		}
	}
	// 兼容升级前没有护栏的私有工作区；只列平台自有 execution 目录，不读取秘密文件。
	executions := filepath.Join(root, "executions")
	if info, err := os.Lstat(executions); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return ErrWorkspaceResidual
	}
	entries, err = os.ReadDir(executions)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrWorkspaceResidual
	}
	for _, entry := range entries {
		path := filepath.Join(executions, entry.Name())
		if !entry.IsDir() || !workspaceGuards.active[workspaceGuardPath(path)] {
			return ErrWorkspaceResidual
		}
	}
	return nil
}

func registerWorkspaceGuard(executionRoot string) error {
	workspaceGuards.Lock()
	defer workspaceGuards.Unlock()
	root := filepath.Dir(filepath.Dir(executionRoot))
	if err := checkWorkspaceRootLocked(root); err != nil {
		return err
	}
	if err := os.Mkdir(executionRoot, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrWorkspaceExists
		}
		return ErrWorkspaceResidual
	}
	path := workspaceGuardPath(executionRoot)
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		_ = os.Remove(executionRoot)
		return ErrWorkspaceResidual
	}
	if err := writePrivateFile(path, []byte("PRIVATE_WORKSPACE_PENDING_CLEANUP\n")); err != nil {
		_ = os.Remove(executionRoot)
		return ErrWorkspaceResidual
	}
	workspaceGuards.active[path] = true
	return nil
}

// Isolate 撤销工作区的活动豁免，保留先于秘密写入的持久护栏和已有材料。
// 它不删除、读取或返回秘密路径；后续领取在同一根目录失败关闭。
func (w Workspace) Isolate() {
	if !w.valid() {
		return
	}
	workspaceGuards.Lock()
	delete(workspaceGuards.active, workspaceGuardPath(w.executionRoot))
	workspaceGuards.Unlock()
}

func releaseWorkspaceGuard(executionRoot string) error {
	workspaceGuards.Lock()
	defer workspaceGuards.Unlock()
	path := workspaceGuardPath(executionRoot)
	delete(workspaceGuards.active, path)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrWorkspaceResidual
	}
	return nil
}
