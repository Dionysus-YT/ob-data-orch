// Package localmvp 仅组装显式启动的本机演示控制面。
// 它只能绑定回环地址，不能作为生产身份、授权或 Agent 实现使用。
package localmvp

import (
	"context"
	"errors"
	"net"
	"net/http"

	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

const CSRFToken = "local-mvp-csrf-v1"

// Dependencies 为本机 MVP 返回受限身份和 SQLite 数据源依赖。
func Dependencies(database *store.Store, keyring *credential.Keyring) controlplane.Dependencies {
	return controlplane.Dependencies{Identity: localIdentity{}, Authorizer: localAuthorizer{}, Roles: localAuthorizer{}, DataSources: database, Creator: database, StateChanger: database, Updater: database, CredentialRefs: database, Encryptor: keyring, CSRF: localCSRF{}, CredentialKeyID: "local-mvp-root-v1"}
}

type localIdentity struct{}

func (localIdentity) AuthenticateBrowser(request *http.Request) (identity.Principal, error) {
	if !loopback(request.RemoteAddr) {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	return identity.Principal{Type: identity.BrowserPrincipal, ID: "local-mvp-owner"}, nil
}
func (localIdentity) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

type localAuthorizer struct{}

func (localAuthorizer) Authorize(_ context.Context, principal identity.Principal, scope identity.Scope, objectID string) error {
	if principal.Type != identity.BrowserPrincipal || principal.ID != "local-mvp-owner" || objectID == "" || (scope != identity.ScopeDataSourceRead && scope != identity.ScopeDataSourceWrite) {
		return identity.ErrDenied
	}
	return nil
}
func (localAuthorizer) AuthorizeRole(_ context.Context, principal identity.Principal, role identity.Role) error {
	if principal.Type != identity.BrowserPrincipal || principal.ID != "local-mvp-owner" || role != identity.RoleDataSourceAdmin {
		return identity.ErrDenied
	}
	return nil
}

type localCSRF struct{}

func (localCSRF) ValidateCSRF(request *http.Request) error {
	if request.Header.Get("X-CSRF-Token") != CSRFToken {
		return errors.New("local CSRF token rejected")
	}
	return nil
}

func loopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	return net.ParseIP(host).IsLoopback()
}
