package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/deployment"
	"ob-data-orch/internal/localmvp"
	"ob-data-orch/internal/servicehost"
	"ob-data-orch/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("控制面停止", "error", err)
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base := filepath.Dir(executable)
	version := flag.Bool("version", false, "显示版本")
	data := flag.String("data-dir", filepath.Join(base, "data"), "固定数据目录，升级时保留")
	initialize := flag.Bool("initialize", false, "仅完成首次安装配置")
	healthCheck := flag.Bool("health-check", false, "检查本机控制面 HTTPS 健康状态")
	install := flag.Bool("install-service", false, "安装开机自启服务")
	devWeb := flag.String("dev-web-url", "", "仅供本机开发启动器代理 Vite")
	flag.Parse()
	if *version {
		info := buildinfo.Current()
		fmt.Printf("OB Data Orch %s (%s)\n", info.Version, info.Commit)
		return nil
	}
	if flag.NArg() != 0 {
		return errors.New("启动参数无效")
	}
	if *healthCheck {
		return deployment.CheckHealth(*data)
	}
	if *install {
		password, err := bufio.NewReader(io.LimitReader(os.Stdin, 1024)).ReadBytes('\n')
		if err != nil {
			return err
		}
		defer credential.Zero(password)
		return servicehost.Install("OBDataOrch", password, "-data-dir", *data)
	}
	if *initialize {
		_, err := deployment.OpenSettings(*data, os.Stdin, os.Stdout)
		return err
	}
	return servicehost.Run("OBDataOrch", func(ctx context.Context) error { return serveWithDevelopment(ctx, base, *data, *devWeb) })
}

func serve(ctx context.Context, base, data string) (result error) {
	return serveWithDevelopment(ctx, base, data, "")
}

func serveWithDevelopment(ctx context.Context, base, data, devWeb string) (result error) {
	_, closeLog, err := servicehost.LogOutput(data)
	if err != nil {
		return err
	}
	defer closeLog()
	settings, err := deployment.OpenSettings(data, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	if devWeb != "" {
		address, parseErr := url.Parse(settings.PublicURL)
		if parseErr != nil || net.ParseIP(address.Hostname()) == nil || !net.ParseIP(address.Hostname()).IsLoopback() {
			return errors.New("开发服务须使用回环地址")
		}
		settings.ListenAddress = address.Host
		// 仅开发启动器写入固定停止标记，供源码重建前正常关闭数据库与日志。
		var cancel context.CancelFunc
		ctx, cancel = servicehost.DevelopmentContext(ctx, base)
		defer cancel()
	}
	defer func() {
		if result != nil {
			slog.Error("控制面停止", "error", result)
		}
	}()
	tlsConfig, err := deployment.TLSConfig(data, settings.PublicURL)
	if err != nil {
		return err
	}
	browser, err := deployment.NewBrowser(settings)
	if err != nil {
		return err
	}
	databasePath := filepath.Join(data, "local-mvp.db")
	keyPath := filepath.Join(data, "local-mvp-root-key.json")
	if _, err := os.Stat(databasePath); err == nil {
		if _, err := os.Stat(keyPath); err != nil {
			return errors.New("数据库已存在但根密钥缺失，请恢复原密钥")
		}
	}
	key, err := credential.LoadOrCreateRootKey(keyPath, "local-mvp-root-v1")
	if err != nil {
		return err
	}
	defer credential.Zero(key)
	keyring, err := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": key})
	if err != nil {
		return err
	}
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer database.Close()
	now := time.Now().UTC()
	if err := database.EnsureAuthSubject(ctx, store.AuthSubject{SubjectID: deployment.SubjectID, ExternalSubject: deployment.SubjectID, DisplayName: "管理员", AccountStatus: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		return err
	}
	dependencies, err := localmvp.DependenciesWithLogs(database, keyring, settings.RealExecutionEnabled, settings.RealExecutionEnabled, filepath.Join(data, "local-mvp-logs"))
	if err != nil {
		return err
	}
	dependencies.Identity = browser
	dependencies.Authorizer = browser
	dependencies.Roles = browser
	dependencies.CSRF = browser
	dependencies.EnrollmentTTL = 24 * time.Hour
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := dependencies.PersistentLogs.Shutdown(shutdown); err != nil {
			slog.Error("日志封存失败", "error", err)
		}
	}()
	api := controlplane.NewHandlerWithDependencies(buildinfo.Current(), dependencies)
	var handler http.Handler
	if devWeb == "" {
		handler, err = browser.Handler(api, filepath.Join(base, "web"), filepath.Join(base, "agents"), data)
	} else {
		handler, err = browser.DevelopmentHandler(api, filepath.Join(base, "web"), filepath.Join(base, "agents"), data, devWeb)
	}
	if err != nil {
		return err
	}
	server := &http.Server{Addr: settings.ListenAddress, Handler: handler, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	failures := make(chan error, 1)
	go func() { failures <- server.ListenAndServeTLS("", "") }()
	slog.Info("控制面启动", "address", settings.PublicURL)
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	case err := <-failures:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
