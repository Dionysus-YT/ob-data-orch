// 本辅助程序只读本轮合成夹具，校验输入到命令边界；不解析秘密、不启动工具或连接数据库。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
)

type parameterCase struct {
	ID    string `json:"id"`
	Input struct {
		ConfigVersion string              `json:"configVersion"`
		Config        importdomain.Config `json:"config"`
	} `json:"input"`
}

var errCasesFailed = errors.New("夹具未通过")

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, errCasesFailed) {
			fmt.Println(`{"status":"FAILED","code":"PARAMETER_FIXTURE_VALIDATION_FAILED"}`)
		}
		os.Exit(1)
	}
}

func run() error {
	var run struct {
		RunID     string `json:"runId"`
		InputRoot string `json:"inputRoot"`
	}
	raw, err := os.ReadFile("artifacts/normal-import-retest-20261019/run.json")
	if err != nil || json.Unmarshal(raw, &run) != nil || len(run.RunID) != 32 {
		return fmt.Errorf("所有权记录无效")
	}
	root, err := filepath.Abs(filepath.Join("ob-data", "nivt-normal-20261019-"+run.RunID[:12]))
	if err != nil || root != run.InputRoot {
		return fmt.Errorf("夹具根目录不匹配")
	}
	raw, err = os.ReadFile(filepath.Join(root, "parameter-cases.json"))
	var cases []parameterCase
	if err != nil || json.Unmarshal(raw, &cases) != nil || len(cases) != 28 {
		return fmt.Errorf("夹具用例不完整")
	}
	g, err := commandgen.NewNormalImport()
	if err != nil {
		return err
	}
	results := []map[string]any{}
	failed := false
	for _, item := range cases {
		result := map[string]any{"id": item.ID, "status": "FAILED"}
		c := item.Input.Config
		if item.Input.ConfigVersion != importdomain.ConfigVersion || !strings.HasPrefix(c.FilePath, root+string(os.PathSeparator)+"parameters"+string(os.PathSeparator)) {
			return fmt.Errorf("夹具路径越界")
		}
		c.DataSourceID, c.NodeID, c.Database = "synthetic-source", "synthetic-node", "synthetic_db"
		n, issues := importdomain.Normalize(c, "MYSQL", "WINDOWS_AMD64", true)
		if len(issues) != 0 {
			result["code"] = "CONFIG_REJECTED"
			failed = true
			results = append(results, result)
			continue
		}
		fields := []commandgen.FieldInput{
			{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic.invalid"}},
			{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 2883}},
			{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_user@synthetic_tenant#synthetic_cluster"}},
			{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: n.Database}},
			{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}}},
			{Name: "--no-sys", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
		}
		if n.Cleanup != "NONE" {
			fields = append(fields, commandgen.FieldInput{Name: "--yes", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
		generated, generateErr := g.Generate(commandgen.Request{Tool: "OBLOADER", ToolVersion: importdomain.ToolVersion, MetadataVersion: importdomain.MetadataVersion, CapabilityVersion: importdomain.CapabilityVersion, ConnectionKind: commandgen.ConnectionODP, TargetPlatform: commandgen.PlatformWindowsAMD64, DataSourceFactVersion: "synthetic-source-1", NodeFactVersion: "synthetic-node-1", Fields: append(fields, importdomain.Fields(n)...)})
		if generateErr != nil || !executionargv.ValidNormalImport(generated.ArgvTemplate) {
			result["code"] = "COMMAND_REJECTED"
			failed = true
			results = append(results, result)
			continue
		}
		manifest, scanErr := importfiles.Scan(context.Background(), n, []string{root})
		if scanErr != nil {
			result["code"] = "INPUT_REJECTED"
			failed = true
			results = append(results, result)
			continue
		}
		plan, mappingErr := importfiles.InspectMapping(context.Background(), n, manifest, []string{root})
		if mappingErr != nil || !plan.Complete || n.HasDDL() && !plan.DDLComplete || len(plan.Tables) != 1 || manifest.FileCount != 1 {
			result["code"] = "MAPPING_UNAVAILABLE"
			failed = true
		} else {
			columns := 3
			if item.ID == "NI-PARAM-include" || item.ID == "NI-PARAM-exclude_col" {
				columns = 2
			}
			if n.HasData() && plan.Tables[0].Columns != columns {
				result["code"] = "COLUMN_COUNT_MISMATCH"
				failed = true
			} else {
				result["status"] = "PASS"
			}
		}
		results = append(results, result)
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"scope": "LOCAL_SYNTHETIC_FILES_ONLY", "records": results, "passed": !failed}); err != nil {
		return err
	}
	if failed {
		return errCasesFailed
	}
	return nil
}
