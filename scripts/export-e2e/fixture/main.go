// 本辅助进程只用于已授权的 TEST MySQL/Oracle 固定夹具，不进入控制面或 Agent 包。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/parammeta"

	_ "modernc.org/sqlite"
)

type input struct {
	Action   string `json:"action"`
	Database string `json:"database"`
	RunID    string `json:"runId"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password []byte `json:"password"`
	Schema   string `json:"schema,omitempty"`
}

func main() {
	if err := run(); err != nil {
		if regexp.MustCompile(`^DIRECT_COMPATIBILITY_[A-Z_]+_UNAVAILABLE$`).MatchString(err.Error()) {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "BLOCKED", "code": err.Error()})
			os.Exit(1)
		}
		fmt.Println(`{"status":"BLOCKED","code":"FIXTURE_OPERATION_UNAVAILABLE"}`)
		os.Exit(1)
	}
}

func run() error {
	action := flag.String("action", "catalog", "catalog/create/augment/exercise-sequence/inspect/cleanup")
	dataRoot := flag.String("data-root", "var/dev/control-plane/data", "控制面现有数据目录，只读")
	databaseName := flag.String("database", "", "本次独立夹具库")
	runID := flag.String("run-id", "", "本次夹具所有权标识")
	python := flag.String("python", "", "隔离测试 Python")
	flag.Parse()
	if *action == "catalog" {
		catalog, err := parammeta.LoadExpertParameters()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Version     string                 `json:"version"`
			Definitions []parammeta.Definition `json:"definitions"`
		}{catalog.MetadataVersion(), catalog.Definitions()})
	}
	capabilityAction := oneOf(*action, "import-capabilities-create", "import-capabilities-inspect", "import-capabilities-repair", "import-capabilities-cleanup", "import-oracle-capabilities-create", "import-oracle-capabilities-inspect", "import-oracle-capabilities-cleanup")
	oracleDirectAction := oneOf(*action, "direct-oracle-probe", "direct-oracle-create", "direct-oracle-inspect", "direct-oracle-cleanup", "direct-oracle-source-request", "direct-oracle-audit", "direct-oracle-compatibility", "direct-oracle-jdbc", "direct-oracle-execution-create", "direct-oracle-execution-inspect", "direct-oracle-execution-cleanup")
	if !regexp.MustCompile(`^odo_e2e_[0-9]{8}_[a-f0-9]{12}$`).MatchString(*databaseName) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(*runID) || !strings.HasSuffix(*databaseName, (*runID)[:12]) || !capabilityAction && !oracleDirectAction && !oneOf(*action, "create", "augment", "exercise-sequence", "inspect", "cleanup", "audit", "import-create", "import-extend", "import-inspect", "import-cleanup", "import-parameters-create", "import-parameters-inspect", "import-parameters-repair", "import-oracle-probe", "import-oracle-audit", "import-oracle-remaining-probe", "import-oracle-remaining-create", "import-oracle-remaining-inspect", "import-oracle-remaining-cleanup", "direct-probe", "direct-create", "direct-inspect", "direct-cleanup", "direct-jdbc", "direct-compatibility", "direct-source-request") || !oneOf(*action, "audit", "import-oracle-audit", "direct-oracle-audit") && *python == "" {
		return errors.New("夹具范围不匹配")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	payload, err := connection(ctx, *dataRoot, strings.HasPrefix(*action, "import-oracle-") || oracleDirectAction)
	if err != nil {
		return err
	}
	defer credential.Zero(payload.Password)
	if oneOf(*action, "audit", "import-oracle-audit", "direct-oracle-audit") {
		// 测试只在内存中比较已授权源密码及其可恢复编码，不输出密码或原始证据。
		evidence, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<20))
		if err != nil || len(evidence) >= 16<<20 {
			return errors.New("秘密核对证据不完整")
		}
		defer credential.Zero(evidence)
		variants := [][]byte{payload.Password, base64.StdEncoding.AppendEncode(nil, payload.Password), escapedSecret(payload.Password)}
		for _, value := range variants[1:] {
			defer credential.Zero(value)
		}
		present := false
		for _, value := range variants {
			if len(value) > 0 && bytes.Contains(evidence, value) {
				present = true
			}
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "AUDITED", "plaintextOrRecoverableEncodingPresent": present})
	}
	payload.Action, payload.Database, payload.RunID = *action, *databaseName, *runID
	if oneOf(*action, "direct-source-request", "direct-oracle-source-request") {
		return directSourceRequest(payload)
	}
	if *action == "direct-oracle-compatibility" {
		return directCompatibility(ctx, payload, *python)
	}
	if *action == "direct-oracle-jdbc" {
		return directJDBC(ctx, payload)
	}
	if oracleDirectAction {
		return directOracleFixture(ctx, payload)
	}
	if *action == "direct-compatibility" {
		return directCompatibility(ctx, payload, *python)
	}
	if *action == "direct-jdbc" {
		return directJDBC(ctx, payload)
	}
	if strings.HasPrefix(*action, "import-oracle-remaining-") || strings.HasPrefix(*action, "import-oracle-capabilities-") {
		return oracleRemaining(ctx, payload)
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	defer credential.Zero(content)
	// 秘密只写入本次子进程的匿名标准输入；不使用 argv、环境变量或凭据文件。
	fixturePath := filepath.Join("scripts", "export-e2e", "fixture.py")
	if strings.HasPrefix(*action, "import-") {
		// 普通导入使用固定的小型夹具，不执行导出性能数据准备或外部 SQL。
		fixturePath = filepath.Join("scripts", "normal-import-e2e", "fixture.py")
	}
	if strings.HasPrefix(*action, "import-oracle-") {
		fixturePath = filepath.Join("scripts", "normal-import-e2e", "oracle_fixture.py")
	}
	if strings.HasPrefix(*action, "direct-") {
		// 旁路仅使用本轮自有库和固定测试动作，不借用普通导入的文件语义。
		fixturePath = filepath.Join("scripts", "direct-load-e2e", "fixture.py")
	}
	command := exec.CommandContext(ctx, *python, fixturePath)
	command.Stdin = bytes.NewReader(content)
	command.Stdout = os.Stdout
	command.Stderr = nil
	return command.Run()
}

// directJDBC 只对本轮独立库的固定目标运行现役只读探针，不创建平台检查或替代执行证明。
func directJDBC(ctx context.Context, payload input) (result error) {
	if payload.Action == "direct-oracle-jdbc" {
		owned := payload
		owned.Action = "direct-oracle-inspect"
		var inspection bytes.Buffer
		if err := directOracleFixtureTo(ctx, owned, &inspection); err != nil {
			return errors.New("Oracle 固定探针夹具所有权不可用")
		}
		var owner struct {
			OwnerVerified bool `json:"ownerVerified"`
		}
		if json.Unmarshal(inspection.Bytes(), &owner) != nil || !owner.OwnerVerified {
			return errors.New("Oracle 固定探针夹具所有权不匹配")
		}
	}
	workspace, err := credential.CreateWorkspace("artifacts/direct-load-e2e-tools/probes", payload.RunID)
	if err != nil {
		return err
	}
	defer func() {
		if err := workspace.Cleanup(); err != nil {
			result = err
		}
	}()
	probePath, probeDigest, err := jdbcprobe.Install(workspace)
	if err != nil {
		return err
	}
	javaPath := `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`
	java, err := os.ReadFile(javaPath)
	if err != nil {
		return err
	}
	javaDigest := sha256.Sum256(java)
	username := []byte(payload.Username)
	defer credential.Zero(username)
	runtime := jdbcprobe.Runtime{JavaPath: javaPath, JavaSHA256: hex.EncodeToString(javaDigest[:]), ProbePath: probePath, ProbeSHA256: probeDigest, ConnectorPath: `E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`, ConnectorSHA256: "3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4", Environment: []string{`SystemRoot=C:\Windows`, `WINDIR=C:\Windows`, `PATH=C:\Windows\system32`}}
	mode, database, table, scenario := jdbcprobe.CompatibilityModeMySQL, payload.Database, "dlvt_basic", "PRIVATE_ODP"
	if payload.Action == "direct-oracle-jdbc" {
		mode, database, table, scenario = jdbcprobe.CompatibilityModeOracle, payload.Schema, "DLVO_"+strings.ToUpper(payload.RunID[:12])+"_BASIC", "OBSERVER_DIRECT"
	}
	observed, err := jdbcprobe.TestDirectLoad(ctx, runtime, jdbcprobe.DirectLoadRequest{Connection: jdbcprobe.Request{Host: payload.Host, Port: payload.Port, Username: username, Password: payload.Password}, CompatibilityMode: mode, Database: database, Table: table, Scenario: scenario})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "PROBED", "scope": "REAL_FIXED_PROBE_ONLY", "databaseAccess": observed.DatabaseAccess, "writeAccess": observed.WriteAccess, "observerVersion": observed.ObserverVersion, "odpVersion": observed.ODPVersion, "tableFacts": observed.TableFacts})
}

// oracleRemaining 只运行测试目录的固定主类；连接秘密经有界匿名帧传递，不提供 SQL/URL/驱动选择入口。
func oracleRemaining(ctx context.Context, payload input) error {
	// 测试只使用本节点已经核验过的固定连接器版本；文件被替换时拒绝继续。
	connector, err := os.ReadFile(`E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`)
	if err != nil {
		return errors.New("固定连接器不可用")
	}
	digest := sha256.Sum256(connector)
	if hex.EncodeToString(digest[:]) != "3168e7e3f2f14f35738d7c695e05012bf351f623757c7baada29659d2afa2ad4" {
		return errors.New("固定连接器摘要变化")
	}
	var frame bytes.Buffer
	defer func() { credential.Zero(frame.Bytes()) }()
	for _, value := range [][]byte{[]byte("NIVR1"), []byte(payload.Action), []byte(payload.RunID), []byte(payload.Schema), []byte(payload.Host), []byte(fmt.Sprint(payload.Port)), []byte(payload.Username), payload.Password} {
		if len(value) > 4096 {
			return errors.New("固定 Oracle 夹具输入越界")
		}
		_ = binary.Write(&frame, binary.BigEndian, uint32(len(value)))
		_, _ = frame.Write(value)
	}
	classes, mainClass := "artifacts/normal-import-remaining-20261020/classes", "OracleRemainingFixture"
	if strings.HasPrefix(payload.Action, "import-oracle-capabilities-") {
		classes, mainClass = "artifacts/normal-import-capabilities-20261020/classes", "OracleCapabilityFixture"
	}
	command := exec.CommandContext(ctx, `C:\Program Files\Java\jre1.8.0_491\bin\java.exe`, "-Xmx64m", "-cp", classes+`;E:\workespace\ob-loader-dumper-4.3.5-RELEASE\lib\oceanbase-client-2.4.14.jar`, mainClass)
	command.Stdin, command.Stdout, command.Stderr = bytes.NewReader(frame.Bytes()), os.Stdout, nil
	return command.Run()
}

// escapedSecret 仅在短生命周期字节缓冲区中形成 URL 编码，不把密码转换为不可清零的字符串。
func escapedSecret(value []byte) []byte {
	const hex = "0123456789ABCDEF"
	result := make([]byte, 0, len(value)*3)
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '-', char == '_', char == '.', char == '~':
			result = append(result, char)
		case char == ' ':
			result = append(result, '+')
		default:
			result = append(result, '%', hex[char>>4], hex[char&15])
		}
	}
	return result
}

func connection(ctx context.Context, dataRoot string, oracle bool) (input, error) {
	dbPath, err := filepath.Abs(filepath.Join(dataRoot, "local-mvp.db"))
	if err != nil {
		return input{}, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return input{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var value input
	var id, environment, kind, mode, state, tenant, username, credentialID, keyID string
	var cluster sql.NullString
	var revision int64
	var nonce, ciphertext []byte
	selector, expectedMode := "d.display_name='test-mysql'", "MYSQL"
	if oracle {
		// Oracle 仅限本次明确授权的既有 TEST 源，不能通过命令行选择任意连接。
		selector, expectedMode = "d.data_source_id='55208ffb-2171-4e69-bb97-5af53c2f0d81'", "ORACLE"
	}
	var schema sql.NullString
	err = db.QueryRowContext(ctx, `SELECT d.data_source_id,d.environment,d.connection_kind,d.compatibility_mode,d.state,d.host,d.port,d.cluster_name,d.tenant_name,d.username,d.credential_id,d.current_credential_revision,c.key_id,c.nonce,c.ciphertext,d.default_database FROM data_sources d JOIN credential_revisions c ON c.credential_id=d.credential_id AND c.revision=d.current_credential_revision AND c.data_source_id=d.data_source_id AND c.secret_type='DATABASE_PASSWORD' AND c.status='ACTIVE' WHERE `+selector).Scan(&id, &environment, &kind, &mode, &state, &value.Host, &value.Port, &cluster, &tenant, &username, &credentialID, &revision, &keyID, &nonce, &ciphertext, &schema)
	if err != nil || environment != "TEST" || kind != "ODP" && kind != "OBSERVER_DIRECT" || kind == "OBSERVER_DIRECT" && cluster.String != "" || mode != expectedMode || state != "ENABLED" || strings.ContainsAny(username+tenant+cluster.String, "@#\r\n\x00") {
		return input{}, errors.New("测试数据源不满足授权范围")
	}
	rootPath := filepath.Join(dataRoot, "local-mvp-root-key.json")
	if _, err := os.Stat(rootPath); err != nil {
		return input{}, err
	}
	// 原根密钥只读复用；缺失时在上方结束，禁止为真实数据库生成替代密钥。
	key, err := credential.LoadOrCreateRootKey(rootPath, "local-mvp-root-v1")
	if err != nil {
		return input{}, err
	}
	defer credential.Zero(key)
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": key})
	if err != nil {
		return input{}, err
	}
	password, err := keyring.Decrypt(credential.Envelope{FormatVersion: credential.FormatVersion, KeyID: keyID, Reference: credential.Reference{CredentialID: credentialID, Revision: revision, SecretType: credential.DatabasePassword, DataSourceID: id}, Nonce: nonce, Ciphertext: ciphertext})
	if err != nil {
		return input{}, err
	}
	value.Username = username + "@" + tenant
	if cluster.Valid && cluster.String != "" {
		value.Username += "#" + cluster.String
	}
	value.Password = password
	if oracle {
		value.Schema = schema.String
		if value.Schema == "" {
			value.Schema = strings.ToUpper(username)
		}
	}
	return value, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
