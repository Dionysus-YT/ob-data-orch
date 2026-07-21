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
