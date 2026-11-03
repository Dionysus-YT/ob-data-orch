package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

// oracleExportFixture 只操作既有 TEST Oracle 源内本轮独占前缀的固定对象；不接收 SQL 或额外权限。
func oracleExportFixture(ctx context.Context, payload input) error {
	return oracleExportFixtureTo(ctx, payload, os.Stdout)
}

func oracleExportFixtureTo(ctx context.Context, payload input, output io.Writer) error {
	const connector = `E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`
	content, err := os.ReadFile(connector)
	if err != nil {
		return errors.New("Oracle 固定连接器不可用")
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != "3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4" {
		return errors.New("Oracle 固定连接器摘要变化")
	}
	var frame bytes.Buffer
	defer func() { credential.Zero(frame.Bytes()) }()
	for _, value := range [][]byte{[]byte("OEO1"), []byte(payload.Action), []byte(payload.RunID), []byte(payload.Schema), []byte(payload.Host), []byte(strconv.Itoa(payload.Port)), []byte(payload.Username), payload.Password} {
		if len(value) > 4096 {
			return errors.New("Oracle 固定夹具输入越界")
		}
		_ = binary.Write(&frame, binary.BigEndian, uint32(len(value)))
		_, _ = frame.Write(value)
	}
	command := exec.CommandContext(ctx, `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`, "-Xmx64m", "-Dfile.encoding=UTF-8", "-cp", `artifacts/oracle-export-fixture-tools/classes;`+connector, "OracleExportObjectsFixture")
	command.Env = []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, `PATH=C:\Windows\system32`}
	command.Stdin, command.Stdout = bytes.NewReader(frame.Bytes()), output
	return command.Run()
}

// oracleExportJDBC 使用当前内嵌固定探针核对自有十类对象，不创建或伪造平台预检查证明。
func oracleExportJDBC(ctx context.Context, payload input) (result error) {
	owned := payload
	owned.Action = "oracle-export-inspect"
	var inspection bytes.Buffer
	if err := oracleExportFixtureTo(ctx, owned, &inspection); err != nil {
		return errors.New("Oracle 夹具所有权不可用")
	}
	var fact struct {
		OwnerVerified bool `json:"ownerVerified"`
		Objects       []struct {
			Type string `json:"objectType"`
			Name string `json:"name"`
		} `json:"objects"`
	}
	if json.Unmarshal(inspection.Bytes(), &fact) != nil || !fact.OwnerVerified || len(fact.Objects) != 10 {
		return errors.New("Oracle 夹具所有权不匹配")
	}
	workspace, err := credential.CreateWorkspace("artifacts/oracle-export-fixture-tools/probes", payload.RunID)
	if err != nil {
		return err
	}
	defer func() {
		if err := workspace.Cleanup(); err != nil {
			result = err
		}
	}()
	probe, probeDigest, err := jdbcprobe.Install(workspace)
	if err != nil {
		return err
	}
	const java = `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`
	const connector = `E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`
	javaBytes, err := os.ReadFile(java)
	if err != nil {
		return err
	}
	javaDigest := sha256.Sum256(javaBytes)
	runtime := jdbcprobe.Runtime{JavaPath: java, JavaSHA256: hex.EncodeToString(javaDigest[:]), ConnectorPath: connector, ConnectorSHA256: "3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4", ProbePath: probe, ProbeSHA256: probeDigest, Environment: []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, `PATH=C:\Windows\system32`}}
	username := []byte(payload.Username)
	defer credential.Zero(username)
	connection := jdbcprobe.Request{Host: payload.Host, Port: payload.Port, Username: username, Password: payload.Password}
	catalog, err := jdbcprobe.ListObjects(ctx, runtime, jdbcprobe.CatalogRequest{Connection: connection, CompatibilityMode: jdbcprobe.CompatibilityModeOracle, Database: payload.Schema, ObjectType: "ALL"})
	if err != nil {
		return errors.New("Oracle 固定目录不可用")
	}
	prefix := "ODOO_" + strings.ToUpper(payload.RunID[:12]) + "_"
	for index := range catalog.Groups {
		var names []string
		for _, name := range catalog.Groups[index].Objects {
			if strings.HasPrefix(name, prefix) {
				names = append(names, name)
			}
		}
		catalog.Groups[index].Objects = names
	}
	request := jdbcprobe.BatchPreflightRequest{Connection: connection, CompatibilityMode: jdbcprobe.CompatibilityModeOracle, Database: payload.Schema}
	for _, object := range fact.Objects {
		request.Objects = append(request.Objects, jdbcprobe.PreflightObject{Type: object.Type, Name: object.Name})
	}
	batch, err := jdbcprobe.TestBatchPreflight(ctx, runtime, request)
	if err != nil {
		return errors.New("Oracle 固定对象检查不可用")
	}
	individual := map[string]jdbcprobe.ObjectAccess{}
	if batch.ObjectAccess != jdbcprobe.ObjectAccessible {
		// 汇总失败时仅分解本轮自有对象，定位真实驱动差异，不扩大目录或读取业务数据。
		for _, object := range request.Objects {
			single := request
			single.Objects = []jdbcprobe.PreflightObject{object}
			checked, err := jdbcprobe.TestBatchPreflight(ctx, runtime, single)
			if err != nil {
				return errors.New("Oracle 固定单对象检查不可用")
			}
			individual[object.Type] = checked.ObjectAccess
		}
	}
	missing := request
	missing.Objects = nil
	for _, kind := range []string{"PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE"} {
		missing.Objects = append(missing.Objects, jdbcprobe.PreflightObject{Type: kind, Name: prefix + "MISSING"})
	}
	missingResult, err := jdbcprobe.TestBatchPreflight(ctx, runtime, missing)
	if err != nil {
		return errors.New("Oracle 固定缺失对象检查不可用")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "CHECKED", "ownerVerified": true, "groups": catalog.Groups, "batchObjectAccess": batch.ObjectAccess, "individualObjectAccess": individual, "missingObjectAccess": missingResult.ObjectAccess})
}
