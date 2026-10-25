package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"ob-data-orch/internal/credential"
)

// directOracleFixture 只连接既有 TEST Oracle 源，固定操作当前用户内带本轮所有权的两个表。
// 此测试类不打包到产品探针，不接受 SQL、类名、连接串或额外 JDBC 驱动。
func directOracleFixture(ctx context.Context, payload input) error {
	return directOracleFixtureTo(ctx, payload, os.Stdout)
}

// directOracleFixtureTo 允许隔离组件在内存中核对同一固定夹具输出，不产生产品证明。
func directOracleFixtureTo(ctx context.Context, payload input, output io.Writer) error {
	if !oneOf(payload.Action, "direct-oracle-probe", "direct-oracle-create", "direct-oracle-inspect", "direct-oracle-cleanup", "direct-oracle-execution-create", "direct-oracle-execution-inspect", "direct-oracle-execution-cleanup") {
		return errors.New("Oracle 旁路夹具动作无效")
	}
	const connector = `E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`
	jar, err := os.ReadFile(connector)
	if err != nil {
		return errors.New("Oracle 固定连接器不可用")
	}
	digest := sha256.Sum256(jar)
	if hex.EncodeToString(digest[:]) != "3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4" {
		return errors.New("Oracle 固定连接器摘要变化")
	}
	var frame bytes.Buffer
	defer func() { credential.Zero(frame.Bytes()) }()
	for _, value := range [][]byte{[]byte("DLO1"), []byte(payload.Action), []byte(payload.RunID), []byte(payload.Schema), []byte(payload.Host), []byte(strconv.Itoa(payload.Port)), []byte(payload.Username), payload.Password} {
		if len(value) > 4096 {
			return errors.New("Oracle 旁路夹具输入越界")
		}
		_ = binary.Write(&frame, binary.BigEndian, uint32(len(value)))
		_, _ = frame.Write(value)
	}
	mainClass := "DirectLoadOracleFixture"
	if strings.HasPrefix(payload.Action, "direct-oracle-execution-") {
		mainClass = "DirectLoadOracleExecutionFixture"
	}
	command := exec.CommandContext(ctx, `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`, "-Xmx64m", "-Dfile.encoding=UTF-8", "-cp", `artifacts/direct-load-oracle-tools/classes;`+connector, mainClass)
	command.Env = []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, `PATH=C:\Windows\system32`}
	command.Stdin, command.Stdout = bytes.NewReader(frame.Bytes()), output
	return command.Run()
}
