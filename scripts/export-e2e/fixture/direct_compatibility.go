package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/logstream"
)

// directCompatibility 只对本轮所有权表执行一次固定工具兼容性核查，不创建产品任务或预检查证明。
// 版本待验证时先取得独立组件证据；不得将此回执伪造成 API 端到端成功。
func directCompatibility(ctx context.Context, payload input, python string) (result error) {
	stage := "OWNERSHIP"
	defer func() {
		if result != nil {
			result = errors.New("DIRECT_COMPATIBILITY_" + stage + "_UNAVAILABLE")
		}
	}()
	oracle := payload.Action == "direct-oracle-compatibility"
	database, target, inputDirectory := payload.Database, "dlvt_basic", filepath.Join("ob-data", "dlvt-direct-"+payload.RunID[:12], "dlvt_basic")
	if oracle {
		database, target = payload.Schema, "DLVO_"+strings.ToUpper(payload.RunID[:12])+"_BASIC"
		inputDirectory = filepath.Join("ob-data", "dlvt-oracle-"+payload.RunID[:12], "basic")
	}
	owned := payload
	owned.Action = "direct-inspect"
	frame, err := json.Marshal(owned)
	if err != nil {
		return err
	}
	defer credential.Zero(frame)
	var observed []byte
	if oracle {
		owned.Action = "direct-oracle-inspect"
		var output bytes.Buffer
		err = directOracleFixtureTo(ctx, owned, &output)
		observed = output.Bytes()
	} else {
		inspection := exec.CommandContext(ctx, python, "scripts/direct-load-e2e/fixture.py")
		inspection.Stdin = bytes.NewReader(frame)
		observed, err = inspection.Output()
	}
	if err != nil {
		return errors.New("兼容性夹具所有权不可用")
	}
	var owner struct {
		OwnerVerified bool     `json:"ownerVerified"`
		Rows          *[][]any `json:"rows"`
		Tables        map[string]struct {
			Rows [][]any `json:"rows"`
		} `json:"tables"`
	}
	if json.Unmarshal(observed, &owner) != nil || !owner.OwnerVerified {
		return errors.New("兼容性夹具所有权无效")
	}
	table, exists := owner.Tables["dlvt_basic"]
	if oracle && (owner.Rows == nil || len(*owner.Rows) != 0) || !oracle && (!exists || len(table.Rows) != 0) {
		return errors.New("兼容性夹具不是本轮空表")
	}
	parts := strings.Split(payload.Username, "@")
	if len(parts) != 2 || strings.Contains(parts[1], "#") {
		return errors.New("兼容性直连身份无效")
	}
	toolHome := `E:\workespace\ob-loader-dumper-4.3.5-RELEASE`
	javaPath := `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`
	stage = "TOOL"
	toolVersion, err := agentexec.DirectLoadToolVersion(ctx, toolHome)
	if err != nil {
		return err
	}
	workspace, err := credential.CreateWorkspace("artifacts/direct-load-e2e-tools/compatibility", payload.RunID)
	if err != nil {
		return err
	}
	defer func() {
		if err := agentexec.RemoveStartIntent(workspace); err != nil {
			result = err
		}
		if err := workspace.Cleanup(); err != nil {
			result = err
		}
	}()
	stage = "SECURITY"
	material, err := credential.GenerateSecurityMaterial(payload.Password)
	if err != nil {
		return err
	}
	defer material.Destroy()
	paths, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		return err
	}
	inputPath, err := filepath.Abs(inputDirectory)
	if err != nil {
		return err
	}
	c := directload.Config{DataSourceID: "compatibility-source", NodeID: "compatibility-node", Database: database, Table: target, Scenario: "OBSERVER_DIRECT", RPCPort: 2882, FilePath: inputPath, Format: "CSV", ExternalData: true, Thread: "1", Parallel: "1", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	stage = "INPUT"
	manifest, err := directloadfiles.Scan(ctx, c, []string{filepath.Dir(inputPath)})
	if err != nil {
		return err
	}
	fields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: payload.Host}},
		{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(payload.Port)}},
		{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: payload.Username}},
		{Name: "--tenant", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: parts[1]}},
		{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: database}},
		{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}}},
	}
	fields = append(fields, directload.Fields(c)...)
	stage = "COMMAND"
	generator, err := commandgen.NewDirectLoad()
	if err != nil {
		return err
	}
	command, err := generator.Generate(commandgen.Request{Tool: "OBLOADER", ToolVersion: directload.ToolVersion, MetadataVersion: directload.MetadataVersion, CapabilityVersion: directload.CapabilityVersion, ConnectionKind: commandgen.ConnectionObserverDirect, DataSourceFactVersion: "compatibility-v1", NodeFactVersion: "compatibility-v1", TargetPlatform: commandgen.PlatformWindowsAMD64, Fields: fields})
	if err != nil {
		return err
	}
	stage = "OUTPUT_POLICY"
	java, err := os.ReadFile(javaPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(java)
	policy, err := logstream.NewBytePolicy("direct-compatibility-v1", [][]byte{payload.Password}, [][]byte{[]byte(payload.Host), []byte(payload.Username)})
	if err != nil {
		return err
	}
	defer policy.Destroy()
	hadoopHome := os.Getenv("HADOOP_HOME")
	if !filepath.IsAbs(hadoopHome) {
		return errors.New("本机固定 Hadoop 环境缺失")
	}
	for _, name := range []string{"hadoop.dll", "winutils.exe"} {
		if info, e := os.Stat(filepath.Join(hadoopHome, "bin", name)); e != nil || !info.Mode().IsRegular() {
			return errors.New("本机固定 Hadoop 文件缺失")
		}
	}
	environment := []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, "JAVA_HOME=" + filepath.Dir(filepath.Dir(javaPath)), "HADOOP_HOME=" + hadoopHome, "PATH=" + filepath.Dir(javaPath) + ";" + filepath.Join(hadoopHome, "bin") + `;C:\Windows\system32`}
	var records []logstream.Record
	launch := agentexec.DirectJavaLaunch{Tool: directload.TaskType, Intent: agentexec.StartIntent{ExecutionID: payload.RunID, TaskID: "compat-" + payload.RunID, LeaseID: "compat-lease-" + payload.RunID, LeaseEpoch: 1, EnvelopeDigest: command.ConfigFingerprint, CreatedAt: time.Now().UTC()}, BootID: "compat-boot-" + payload.RunID, JavaPath: javaPath, JavaSHA256: hex.EncodeToString(digest[:]), ToolHome: toolHome, SecurityConfiguration: paths.SecurityConfiguration, BusinessArguments: command.ArgvTemplate, Environment: environment, Output: agentexec.DirectJavaOutput{Policy: policy, Stdout: agentexec.DirectJavaLogStream{SourceKind: logstream.SourceLoaderStdout, StreamID: "compat-stdout", SourceEpoch: 1, ParserVersion: "obloader-direct-4.3.5-v1"}, Stderr: agentexec.DirectJavaLogStream{SourceKind: logstream.SourceLoaderStderr, StreamID: "compat-stderr", SourceEpoch: 1, ParserVersion: "obloader-direct-4.3.5-v1"}, Sink: func(r logstream.Record) error { records = append(records, r); return nil }}}
	stage = "LAUNCH"
	process, err := agentexec.StartDirectJava(ctx, workspace, launch)
	if err != nil {
		return err
	}
	exited, code, waitErr := process.Wait()
	commit := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] Load task on table "` + regexp.QuoteMeta(target) + `" is committed successfully! Elapsed: [^\r\n]+$`)
	exit := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] System exit 0\s*$`)
	committed, terminal, complete := false, false, true
	for _, record := range records {
		complete = complete && record.Kind == logstream.RecordLog && (record.IntegrityCode == "" || record.IntegrityCode == "COMPLETE")
		committed = committed || commit.MatchString(record.Message)
		terminal = terminal || committed && exit.MatchString(record.Message)
	}
	receipt := map[string]any{"scope": "ISOLATED_FIXED_TOOL_COMPATIBILITY_COMPONENT", "toolVersion": toolVersion, "toolSHA256": agentexec.DirectLoadToolSHA256, "ownerVerified": true, "inputManifest": manifest, "processExited": exited, "exitCode": code, "outputComplete": complete && waitErr == nil, "sameTableCommitObserved": committed, "terminalObserved": terminal, "records": records}
	date := strings.Split(payload.Database, "_")[2]
	output := filepath.Join("artifacts", "direct-load-e2e-"+date+"-"+payload.RunID[:12], "compatibility-tool.json")
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("兼容性证据写入失败")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "COMPONENT_OBSERVED", "processExited": exited, "exitCode": code, "outputComplete": complete && waitErr == nil, "sameTableCommitObserved": committed, "terminalObserved": terminal, "independentTargetVerificationRequired": true})
}
