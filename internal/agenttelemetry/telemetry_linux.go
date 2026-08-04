package agenttelemetry

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type linuxSystemSampler struct {
	mu          sync.Mutex
	initialized bool
	idle        uint64
	total       uint64
}

func newSystemSampler() systemSampler {
	return &linuxSystemSampler{}
}

// Sample 从 procfs 获取 Linux 机器总体 CPU 与内存；读取失败时仅省略对应指标，不伪造数值。
func (s *linuxSystemSampler) Sample() (*int, *int) {
	if s == nil {
		return nil, nil
	}
	memory := linuxMemoryUsage()
	idle, total, ok := linuxCPUTimes()
	if !ok {
		return nil, memory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		s.initialized, s.idle, s.total = true, idle, total
		return nil, memory
	}
	deltaIdle, deltaTotal := idle-s.idle, total-s.total
	s.idle, s.total = idle, total
	if deltaTotal == 0 || deltaIdle > deltaTotal {
		return nil, memory
	}
	value := int(((deltaTotal - deltaIdle) * 100) / deltaTotal)
	return &value, memory
}

func linuxCPUTimes() (uint64, uint64, bool) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	var total uint64
	for _, field := range fields[1:] {
		value, parseErr := strconv.ParseUint(field, 10, 64)
		if parseErr != nil {
			return 0, 0, false
		}
		total += value
	}
	idle, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if len(fields) > 5 {
		wait, waitErr := strconv.ParseUint(fields[5], 10, 64)
		if waitErr != nil {
			return 0, 0, false
		}
		idle += wait
	}
	return idle, total, true
}

func linuxMemoryUsage() *int {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer file.Close()
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value
		}
	}
	total, available := values["MemTotal"], values["MemAvailable"]
	if total == 0 || available > total {
		return nil
	}
	value := int(((total - available) * 100) / total)
	return &value
}

func dataRootUsage(root string) (uint64, uint64, error) {
	var status syscall.Statfs_t
	if err := syscall.Statfs(root, &status); err != nil || status.Bsize <= 0 {
		return 0, 0, errors.New("data root usage is unavailable")
	}
	blockSize := uint64(status.Bsize)
	return status.Blocks * blockSize, status.Bavail * blockSize, nil
}
