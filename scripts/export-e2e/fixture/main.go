// 本辅助进程只用于本轮已授权的非生产 test-mysql 专用夹具，不进入控制面或 Agent 包。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
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
}

func main() {
	if err := run(); err != nil {
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
	if !regexp.MustCompile(`^odo_e2e_[0-9]{8}_[a-f0-9]{12}$`).MatchString(*databaseName) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(*runID) || !strings.HasSuffix(*databaseName, (*runID)[:12]) || !oneOf(*action, "create", "augment", "exercise-sequence", "inspect", "cleanup", "audit") || *action != "audit" && *python == "" {
		return errors.New("夹具范围不匹配")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	payload, err := connection(ctx, *dataRoot)
	if err != nil {
		return err
	}
	defer credential.Zero(payload.Password)
	if *action == "audit" {
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
	content, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	defer credential.Zero(content)
	// 秘密只写入本次子进程的匿名标准输入；不使用 argv、环境变量或凭据文件。
	command := exec.CommandContext(ctx, *python, filepath.Join("scripts", "export-e2e", "fixture.py"))
	command.Stdin = bytes.NewReader(content)
	command.Stdout = os.Stdout
	command.Stderr = nil
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

func connection(ctx context.Context, dataRoot string) (input, error) {
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
	err = db.QueryRowContext(ctx, `SELECT d.data_source_id,d.environment,d.connection_kind,d.compatibility_mode,d.state,d.host,d.port,d.cluster_name,d.tenant_name,d.username,d.credential_id,d.current_credential_revision,c.key_id,c.nonce,c.ciphertext FROM data_sources d JOIN credential_revisions c ON c.credential_id=d.credential_id AND c.revision=d.current_credential_revision AND c.data_source_id=d.data_source_id AND c.secret_type='DATABASE_PASSWORD' AND c.status='ACTIVE' WHERE d.display_name='test-mysql'`).Scan(&id, &environment, &kind, &mode, &state, &value.Host, &value.Port, &cluster, &tenant, &username, &credentialID, &revision, &keyID, &nonce, &ciphertext)
	if err != nil || environment != "TEST" || kind != "ODP" || mode != "MYSQL" || state != "ENABLED" || strings.ContainsAny(username+tenant+cluster.String, "@#\r\n\x00") {
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
