//go:build windows

package agentwire

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// securePrivatePath 使用当前服务账户、SYSTEM 和管理员的受保护 DACL 保存 Agent 身份状态。
// Go 文件模式不能移除 Windows 继承 ACL，因此必须在写入前显式建立并复验 DACL。
func securePrivatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private Agent path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return errors.New("private Agent path type is invalid")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read Agent service account identity: %w", err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("read SYSTEM SID: %w", err)
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("read Administrators SID: %w", err)
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		privateAllowAll(user.User.Sid, inheritance),
		privateAllowAll(system, inheritance),
		privateAllowAll(administrators, inheritance),
	}, nil)
	if err != nil {
		return fmt.Errorf("build private Agent ACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("restrict private Agent ACL: %w", err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return errors.New("private Agent ACL cannot be verified")
	}
	dacl, defaulted, err := sd.DACL()
	if err != nil || dacl == nil || defaulted {
		return errors.New("private Agent ACL cannot be verified")
	}
	return nil
}

func privateAllowAll(sid *windows.SID, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
