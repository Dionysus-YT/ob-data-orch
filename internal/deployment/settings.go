// Package deployment 提供统一安装配置、浏览器登录、静态页面和 Agent 包分发。
package deployment

import (
	"bufio"
	"bytes"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ob-data-orch/internal/credential"
)

// SubjectID 保留既有管理员对象归属，登录机制改变不迁移业务对象。
const SubjectID = "local-mvp-owner"

// Settings 是安装后持久保留的部署配置，密码只保存带盐的派生校验值。
type Settings struct {
	PublicURL            string `json:"publicUrl"`
	ListenAddress        string `json:"listenAddress"`
	PasswordSalt         []byte `json:"passwordSalt"`
	PasswordHash         []byte `json:"passwordHash"`
	RealExecutionEnabled bool   `json:"realExecutionEnabled"`
}

// OpenSettings 仅在首次交互启动时初始化；配置损坏或服务无输入时拒绝重置已有身份。
func OpenSettings(directory string, input io.Reader, output io.Writer) (Settings, error) {
	path := filepath.Join(directory, "settings.json")
	if err := credential.PreparePrivateDirectory(directory); err != nil {
		return Settings{}, err
	}
	content, err := os.ReadFile(path)
	if err == nil {
		var settings Settings
		if json.Unmarshal(content, &settings) != nil || validateSettings(settings) != nil {
			return Settings{}, errors.New("安装配置无效，请恢复 settings.json，不要删除身份目录")
		}
		return settings, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, err
	}
	// 有业务库但无密钥时禁止自动补根，避免旧密文被新根静默隔离。
	reader := bufio.NewReader(io.LimitReader(input, 8192))
	fmt.Fprintln(output, "首次配置：请输入访问地址（例如 https://orch.example.internal:8080）：")
	address, err := reader.ReadString('\n')
	if err != nil {
		return Settings{}, errors.New("请先运行启动入口完成首次配置，再安装后台服务")
	}
	address = strings.TrimSpace(address)
	parsed, err := validatePublicURL(address)
	if err != nil {
		return Settings{}, err
	}
	fmt.Fprintln(output, "请设置管理员密码（至少 8 个字符；账户名为 admin）：")
	password, err := reader.ReadBytes('\n')
	if err != nil {
		credential.Zero(password)
		return Settings{}, errors.New("未取得管理员密码")
	}
	defer credential.Zero(password)
	password = bytes.TrimRight(password, "\r\n")
	if len(password) < 8 || len(password) > 256 {
		return Settings{}, errors.New("管理员密码长度须为 8 至 256 字节")
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return Settings{}, err
	}
	hash, err := passwordKey(password, salt)
	if err != nil {
		return Settings{}, err
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	settings := Settings{PublicURL: address, ListenAddress: net.JoinHostPort("", port), PasswordSalt: salt, PasswordHash: hash}
	content, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Settings{}, err
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil {
		return Settings{}, writeErr
	}
	if closeErr != nil {
		return Settings{}, closeErr
	}
	return settings, nil
}

func validatePublicURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("访问地址须为不带路径的 HTTPS 地址")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsUnspecified() {
		return nil, errors.New("访问地址须使用 Agent 可连接的固定地址")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("访问端口无效")
		}
	}
	return parsed, nil
}

func validateSettings(settings Settings) error {
	if _, err := validatePublicURL(settings.PublicURL); err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(settings.ListenAddress); err != nil {
		return err
	}
	if len(settings.PasswordSalt) != 32 || len(settings.PasswordHash) != 32 {
		return errors.New("登录配置无效")
	}
	return nil
}

func passwordKey(password, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(password), salt, 600000, 32)
}
