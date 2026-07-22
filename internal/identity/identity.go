// Package identity defines the fail-closed authentication and authorization
// seam used by the control plane. It deliberately provides no HTTP header,
// cookie, token, or test-user implementation.
package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("identity is unauthenticated")
	ErrUnavailable     = errors.New("identity provider is unavailable")
	ErrDenied          = errors.New("access is not authorized")
)

type PrincipalType string

const (
	BrowserPrincipal PrincipalType = "BROWSER"
	AgentPrincipal   PrincipalType = "AGENT"
)

type Principal struct {
	Type PrincipalType
	ID   string
}

// Provider receives the whole request so a future deployment integration can
// validate its own secure session or machine credential. The platform does not
// infer identity from a user-controlled header.
type Provider interface {
	AuthenticateBrowser(*http.Request) (Principal, error)
	AuthenticateAgent(*http.Request) (Principal, error)
}

type Scope string

const (
	ScopeDataSourceRead  Scope = "DATA_SOURCE_READ"
	ScopeDataSourceWrite Scope = "DATA_SOURCE_WRITE"
	ScopeNodeUse         Scope = "NODE_USE"
	ScopeTaskRead        Scope = "TASK_READ"
)

// Authorizer has no implicit administrator or all-object wildcard. A provider
// must explicitly decide both a fixed capability and the requested object.
type Authorizer interface {
	Authorize(context.Context, Principal, Scope, string) error
}

// Role is a fixed product role from the confirmed authorization baseline. It
// is distinct from object scopes because creation has no existing object ID.
type Role string

const RoleDataSourceAdmin Role = "ROLE_DATA_SOURCE_ADMIN"

// RoleAuthorizer verifies a fixed role without granting any object scope.
// The concrete implementation must load roles from the trusted identity/
// authorization projection rather than a client-provided request value.
type RoleAuthorizer interface {
	AuthorizeRole(context.Context, Principal, Role) error
}

func Validate(principal Principal, expected PrincipalType) error {
	if principal.Type != expected || strings.TrimSpace(principal.ID) == "" {
		return ErrUnauthenticated
	}
	return nil
}

func Can(ctx context.Context, authorizer Authorizer, principal Principal, scope Scope, objectID string) error {
	if authorizer == nil {
		return ErrUnavailable
	}
	if strings.TrimSpace(objectID) == "" || scope == "" {
		return ErrDenied
	}
	if err := authorizer.Authorize(ctx, principal, scope, objectID); err != nil {
		return ErrDenied
	}
	return nil
}

// HasRole is used only where the product permits an action before an object
// exists, such as creating a data source. It fails closed if the role service
// is absent, returns an error, or receives an invalid principal/role.
func HasRole(ctx context.Context, authorizer RoleAuthorizer, principal Principal, role Role) error {
	if authorizer == nil || Validate(principal, BrowserPrincipal) != nil || role == "" {
		return ErrDenied
	}
	if err := authorizer.AuthorizeRole(ctx, principal, role); err != nil {
		return ErrDenied
	}
	return nil
}
