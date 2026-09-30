package deployment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

// DevelopmentHandler 仅在回环代理本机 Vite；真实能力由持久配置控制，认证、API 与 Agent 通道沿用正式入口。
func (b *Browser) DevelopmentHandler(api http.Handler, webDirectory, packageDirectory, dataDirectory, target string) (http.Handler, error) {
	upstream, err := url.Parse(target)
	public, publicErr := url.Parse(b.settings.PublicURL)
	if err != nil || publicErr != nil || upstream.Scheme != "http" || !loopbackHost(upstream.Hostname()) || !loopbackHost(public.Hostname()) || upstream.Port() == "" || upstream.User != nil || upstream.Path != "" || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, errors.New("开发代理仅允许回环地址")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = &http.Transport{Proxy: nil}
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		// Vite 只处理源码资源，不接收浏览器会话或业务认证材料。
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
		r.Header.Del("X-CSRF-Token")
		r.Header.Set("Accept-Encoding", "identity")
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Set("Cache-Control", "no-store")
		if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
			return nil
		}
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if err != nil || len(content) >= 4<<20 || !bytes.Contains(content, []byte("local-mvp-csrf-v1")) {
			return errors.New("开发页面安全令牌占位无效")
		}
		// 会话只从代理前保存的上下文取值，不由 Vite 或客户端指定。
		csrf, _ := response.Request.Context().Value(developmentCSRFKey{}).(string)
		if csrf == "" {
			return errors.New("开发页面会话不可用")
		}
		content = bytes.ReplaceAll(content, []byte("local-mvp-csrf-v1"), []byte(csrf))
		response.Body = io.NopCloser(bytes.NewReader(content))
		response.ContentLength = int64(len(content))
		response.Header.Set("Content-Length", strconv.Itoa(len(content)))
		response.Header.Del("ETag")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "开发前端暂不可用，请检查 Vite 终端", http.StatusBadGateway)
	}
	development := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := b.session(r)
		if !ok {
			http.Error(w, "请登录", http.StatusUnauthorized)
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), developmentCSRFKey{}, session.csrf)))
	})
	return b.handler(api, webDirectory, packageDirectory, dataDirectory, development)
}

type developmentCSRFKey struct{}

func loopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
