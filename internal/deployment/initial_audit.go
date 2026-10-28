package deployment

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/store"
)

const initialAuditConfirmation = "GRANT_AUDIT_LOG admin"

// BootstrapInitialAuditAccess 是原安装入口的显式离线操作，不由服务启动、网页或 Agent 调用。
// 它不初始化配置或身份；只在控制面端口空闲、原口令及固定确认短语均通过后打开现有库。
func BootstrapInitialAuditAccess(ctx context.Context, directory string, input io.Reader, output io.Writer) error {
	content, err := os.ReadFile(filepath.Join(directory, "settings.json"))
	if err != nil {
		return errors.New("请先在原安装目录完成首次配置和控制面启动")
	}
	var settings Settings
	if json.Unmarshal(content, &settings) != nil || validateSettings(settings) != nil {
		return errors.New("安装配置无效，首次授权不会重置配置或密码")
	}
	databasePath := filepath.Join(directory, "local-mvp.db")
	if info, err := os.Stat(databasePath); err != nil || !info.Mode().IsRegular() {
		return errors.New("现有业务库不可用，请先在原安装目录启动并正常停止控制面")
	}
	// 持有监听端口直到操作结束，拒绝与现役控制面并行，也阻止其在授权中途启动。
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", settings.ListenAddress)
	if err != nil {
		return errors.New("控制面监听端口不可独占，请先停止控制面后再首次授权")
	}
	defer listener.Close()
	if _, err = fmt.Fprintln(output, "将仅为固定 admin 显式授予 CAP_AUDIT_LOG（审计读取），不增加业务对象权限。此入口仅可使用一次，撤权后不能重授。请输入现有 admin 密码："); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(io.LimitReader(input, 1024), 1024)
	password, readErr := reader.ReadSlice('\n')
	if readErr != nil {
		credential.Zero(password)
		return errors.New("未取得完整管理员密码，首次授权未执行")
	}
	secret := bytes.TrimSuffix(bytes.TrimSuffix(password, []byte{'\n'}), []byte{'\r'})
	if len(secret) == 0 || len(secret) > 256 {
		credential.Zero(password)
		return errors.New("管理员认证失败，首次授权未执行")
	}
	hash, err := passwordKey(secret, settings.PasswordSalt)
	credential.Zero(password)
	valid := err == nil && subtle.ConstantTimeCompare(hash, settings.PasswordHash) == 1
	credential.Zero(hash)
	if !valid {
		return errors.New("管理员认证失败，首次授权未执行")
	}
	if _, err = fmt.Fprintln(output, "确认上述主体、独立能力和一次性边界后，输入 GRANT_AUDIT_LOG admin；其他输入或中断均取消："); err != nil {
		return err
	}
	confirmation, err := reader.ReadSlice('\n')
	confirmed := err == nil && bytes.Equal(bytes.TrimSpace(confirmation), []byte(initialAuditConfirmation))
	credential.Zero(confirmation)
	if !confirmed {
		return errors.New("未取得明确确认，首次授权未执行")
	}
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		return errors.New("现有数据库校验失败，首次授权未确认完成")
	}
	defer database.Close()
	before, err := database.ReadInitialAuditAccess(ctx, SubjectID)
	if err != nil {
		return errors.New("初始管理员当前不满足首次授权条件")
	}
	if before.Granted || before.Consumed {
		return store.ErrInitialAuditAccessConsumed
	}
	revision, err := database.GrantInitialAuditAccess(ctx, accesscontrol.InitialAuditGrant{SubjectID: SubjectID, ExpectedRevision: before.Revision, Confirmed: true, At: time.Now().UTC()})
	if err != nil {
		return errors.New("首次授权事务未确认完成；请保留原库和身份，核对状态后再处理")
	}
	after, err := database.ReadInitialAuditAccess(ctx, SubjectID)
	if err != nil || !after.Granted || !after.Consumed || after.Revision != revision {
		return errors.New("首次授权已提交，当前状态待核对；请保留原库和身份，不要重复补授")
	}
	_, err = fmt.Fprintf(output, "已显式授予 admin 审计读取，授权版本 %d；安全审计与一次性标记已提交。请从原启动入口恢复控制面并登录查看。\n", revision)
	return err
}
