package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/exportdomain"
	"ob-data-orch/internal/logstream"
)

// oracleExportComponent 仅运行七个固定自有夹具场景；不创建或替代平台任务、租约及预检查证明。
func oracleExportComponent(ctx context.Context, payload input) (result error) {
	stage := "OWNERSHIP"
	defer func() {
		if result != nil {
			result = errors.New("DIRECT_COMPATIBILITY_ORACLE_EXPORT_" + stage + "_UNAVAILABLE")
		}
	}()
	owned := payload
	owned.Action = "oracle-export-inspect"
	var inspection bytes.Buffer
	if err := oracleExportFixtureTo(ctx, owned, &inspection); err != nil {
		return err
	}
	var owner struct {
		OwnerVerified bool `json:"ownerVerified"`
		Objects       []struct {
			Type string `json:"objectType"`
			Name string `json:"name"`
		} `json:"objects"`
	}
	if json.Unmarshal(inspection.Bytes(), &owner) != nil || !owner.OwnerVerified || len(owner.Objects) != 10 {
		return errors.New("自有夹具不匹配")
	}
	caseID := strings.TrimPrefix(payload.Action, "oracle-export-component-")
	selectedType := map[string]string{"package": "PACKAGE", "trigger": "TRIGGER", "synonym": "SYNONYM", "public-synonym": "PUBLIC_SYNONYM", "type": "TYPE"}[caseID]
	if selectedType == "" && caseID != "mixed-ddl" && caseID != "mixed-csv" {
		return errors.New("场景越界")
	}
	objects := map[string][]string{}
	for _, object := range owner.Objects {
		if selectedType == "" || object.Type == selectedType {
			objects[object.Type] = []string{object.Name}
		}
	}
	output, err := filepath.Abs(filepath.Join("ob-data", "odo-oracle-"+payload.RunID[:12], "component-"+caseID))
	if err != nil {
		return err
	}
	stage = "COMMAND"
	toolOutput := "/" + filepath.ToSlash(output)
	draft := exportdomain.Draft{Database: payload.Schema, ScopeKind: "SPECIFIED", ObjectsByType: objects, ContentKind: "DDL_ONLY", FilePath: toolOutput, LogPath: toolOutput + "-logs"}
	capability := "export-odp-ddl-v1"
	if caseID == "mixed-csv" {
		thread := 1
		draft.Thread, draft.JvmMemory = &thread, "1G"
		draft.ContentKind, draft.Format, capability = "DDL_AND_DATA", "CSV", "export-odp-ddl-csv-v1"
	}
	if err := exportdomain.ValidateObjectCompatibility(draft, "ORACLE"); err != nil {
		return err
	}
	fields, err := exportdomain.BuildGeneralizedFields(draft)
	if err != nil {
		return err
	}
	stringField := func(name string, source commandgen.ValueSource, value string) commandgen.FieldInput {
		return commandgen.FieldInput{Name: name, Source: source, Value: commandgen.Value{Kind: commandgen.ValueString, String: value}}
	}
	fields = append(fields, stringField("--host", commandgen.SourceDataSource, payload.Host), commandgen.FieldInput{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: int64(payload.Port)}}, stringField("--user", commandgen.SourceDataSource, payload.Username), stringField("--database", commandgen.SourceUser, payload.Schema), commandgen.FieldInput{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}}})
	generator, err := commandgen.NewGeneralized()
	if err != nil {
		return err
	}
	command, err := generator.Generate(commandgen.Request{Tool: "OBDUMPER", ToolVersion: "4.3.5-RELEASE", MetadataVersion: "obdumper-4.3.5-slice-v13-oracle-objects", CapabilityVersion: capability, ConnectionKind: commandgen.ConnectionODP, DataSourceFactVersion: "oracle-export-component-v1", NodeFactVersion: "oracle-export-component-v1", TargetPlatform: commandgen.PlatformWindowsAMD64, Fields: fields})
	if err != nil {
		var invalid *commandgen.ValidationError
		if errors.As(err, &invalid) {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "BLOCKED", "code": "COMMAND_VALIDATION", "issues": invalid.Issues})
		}
		return err
	}
	stage = "OUTPUT"
	if err := os.Mkdir(filepath.Dir(output), 0700); err != nil && !os.IsExist(err) {
		return err
	}
	parent, err := os.Lstat(filepath.Dir(output))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return errors.New("输出根目录不匹配")
	}
	// 每个固定场景只运行一次；不覆盖既有输出或借用非本轮目录。
	if err := os.Mkdir(output, 0700); err != nil {
		return err
	}
	stage = "RUNTIME"
	const toolHome = `E:\workespace\ob-loader-dumper-4.3.5-RELEASE`
	const javaPath = `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`
	if _, err := agentexec.DirectLoadToolVersion(ctx, toolHome); err != nil {
		return err
	}
	java, err := os.ReadFile(javaPath)
	if err != nil {
		return err
	}
	javaDigest := sha256.Sum256(java)
	workspace, err := credential.CreateWorkspace("artifacts/oracle-export-fixture-tools/components", payload.RunID+"-"+caseID)
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
	security, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		return err
	}
	policy, err := logstream.NewBytePolicy("oracle-export-component-v1", [][]byte{payload.Password}, [][]byte{[]byte(payload.Host), []byte(payload.Username), []byte(payload.Schema)})
	if err != nil {
		return err
	}
	defer policy.Destroy()
	hadoopHome := os.Getenv("HADOOP_HOME")
	if !filepath.IsAbs(hadoopHome) {
		return errors.New("固定 Hadoop 环境缺失")
	}
	environment := []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, "JAVA_HOME=" + filepath.Dir(filepath.Dir(javaPath)), "HADOOP_HOME=" + hadoopHome, "PATH=" + filepath.Dir(javaPath) + ";" + filepath.Join(hadoopHome, "bin") + `;C:\Windows\system32`}
	var records []logstream.Record
	launch := agentexec.DirectJavaLaunch{Tool: "OBDUMPER", Intent: agentexec.StartIntent{ExecutionID: payload.RunID + "-" + caseID, TaskID: "oracle-export-component-" + caseID, LeaseID: "component-lease-" + payload.RunID, LeaseEpoch: 1, EnvelopeDigest: command.ConfigFingerprint, CreatedAt: time.Now().UTC()}, BootID: "component-boot-" + payload.RunID, JavaPath: javaPath, JavaSHA256: hex.EncodeToString(javaDigest[:]), ToolHome: toolHome, SecurityConfiguration: security.SecurityConfiguration, BusinessArguments: command.ArgvTemplate, Environment: environment, Output: agentexec.DirectJavaOutput{Policy: policy, Stdout: agentexec.DirectJavaLogStream{StreamID: "component-stdout", SourceEpoch: 1, ParserVersion: "direct-java-v1"}, Stderr: agentexec.DirectJavaLogStream{StreamID: "component-stderr", SourceEpoch: 1, ParserVersion: "direct-java-v1"}, Sink: func(record logstream.Record) error { records = append(records, record); return nil }}}
	// 组件没有平台租约，只能使用本辅助进程的两分钟截止点限制启动；该值不形成产品证明。
	launch.LeaseExpiresAt, _ = ctx.Deadline()
	stage = "LAUNCH"
	process, err := agentexec.StartDirectJava(ctx, workspace, launch)
	if err != nil {
		code := "LAUNCH_UNAVAILABLE"
		for _, known := range []struct {
			err  error
			code string
		}{{agentexec.ErrDirectJavaLaunchInvalid, "LAUNCH_INVALID"}, {agentexec.ErrDirectJavaStartFailed, "LAUNCH_START_FAILED"}, {agentexec.ErrStartIntentInvalid, "INTENT_INVALID"}, {agentexec.ErrStartIntentExists, "INTENT_EXISTS"}} {
			if errors.Is(err, known.err) {
				code = known.code
			}
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "BLOCKED", "code": code})
	}
	exited, code, waitErr := process.Wait()
	terminalPattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d{3})? \[INFO\] System exit 0\s*$`)
	terminal, complete, errorsObserved := false, waitErr == nil, false
	for _, record := range records {
		complete = complete && record.Kind == logstream.RecordLog && (record.IntegrityCode == "" || record.IntegrityCode == "COMPLETE")
		terminal = terminal || record.SourceKind == logstream.SourceStdout && terminalPattern.MatchString(record.Message)
		lower := strings.ToLower(record.Message)
		errorsObserved = errorsObserved || strings.Contains(lower, "[error]") || strings.Contains(lower, "[fatal]")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "COMPONENT_OBSERVED", "scope": "FIXED_TOOL_COMPONENT", "caseId": caseID, "ownerVerified": true, "objectTypes": objects, "toolVersion": "4.3.5-RELEASE", "toolSHA256": agentexec.DirectLoadToolSHA256, "configFingerprint": command.ConfigFingerprint, "processExited": exited, "exitCode": code, "outputComplete": complete, "terminalObserved": terminal, "errorsObserved": errorsObserved, "records": records, "independentContentVerificationRequired": true})
}
