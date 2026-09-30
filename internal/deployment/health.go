package deployment

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// CheckHealth 通过回环连接校验当前控制面的 HTTPS 证书及健康响应，不访问外部业务端点。
func CheckHealth(directory string) error {
	content, err := os.ReadFile(filepath.Join(directory, "settings.json"))
	if err != nil {
		return err
	}
	var settings Settings
	if json.Unmarshal(content, &settings) != nil || validateSettings(settings) != nil {
		return errors.New("安装配置无效")
	}
	address, _ := validatePublicURL(settings.PublicURL)
	content, err = os.ReadFile(filepath.Join(directory, "control-plane-ca.pem"))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(content) {
		return errors.New("服务信任证书无效")
	}
	port := address.Port()
	if port == "" {
		port = "443"
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: address.Hostname()}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(settings.PublicURL + "/healthz")
	if err != nil {
		return errors.New("控制面健康检查失败，请检查服务日志与访问地址")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("控制面健康响应无效")
	}
	return nil
}
