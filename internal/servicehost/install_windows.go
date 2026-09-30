//go:build windows

package servicehost

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
	"strings"
	"time"
	"unsafe"
)

// Install 注册当前账户运行的自动服务，避免切换账户导致 DPAPI 身份无法解密。
// 密码由安装器标准输入短时传递，不进入进程参数、服务参数或配置文件。
func Install(name string, password []byte, args ...string) error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	account, domain, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		return err
	}
	if err := grantServiceLogon(user.User.Sid); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(name)
	if err == nil {
		service.Close()
		return errors.New("服务已安装，请通过启动入口管理，不要重复安装或改变运行账户")
	}
	if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	service, err = manager.CreateService(name, executable, mgr.Config{DisplayName: name, StartType: mgr.StartAutomatic, ServiceStartName: domain + `\` + account, Password: strings.TrimRight(string(password), "\r\n")}, args...)
	if err != nil {
		return err
	}
	defer service.Close()
	if err := service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 15 * time.Second}}, 86400); err != nil {
		return err
	}
	return nil
}

// grantServiceLogon 仅授予当前安装账户作为服务登录的权限，不变更其他本地安全策略。
func grantServiceLogon(sid *windows.SID) error {
	dll := windows.NewLazySystemDLL("advapi32.dll")
	type objectAttributes struct {
		Length     uint32
		Root       uintptr
		Name       uintptr
		Attributes uint32
		Descriptor uintptr
		QoS        uintptr
	}
	type unicodeString struct {
		Length        uint16
		MaximumLength uint16
		Buffer        *uint16
	}
	attrs := objectAttributes{}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle uintptr
	status, _, _ := dll.NewProc("LsaOpenPolicy").Call(0, uintptr(unsafe.Pointer(&attrs)), 0x00000800|0x00000010, uintptr(unsafe.Pointer(&handle)))
	convert := func(status uintptr) error {
		code, _, _ := dll.NewProc("LsaNtStatusToWinError").Call(status)
		return windows.Errno(code)
	}
	if status != 0 {
		return convert(status)
	}
	defer dll.NewProc("LsaClose").Call(handle)
	text, err := windows.UTF16FromString("SeServiceLogonRight")
	if err != nil {
		return err
	}
	right := unicodeString{Length: uint16((len(text) - 1) * 2), MaximumLength: uint16(len(text) * 2), Buffer: &text[0]}
	status, _, _ = dll.NewProc("LsaAddAccountRights").Call(handle, uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(&right)), 1)
	if status != 0 {
		return convert(status)
	}
	return nil
}
