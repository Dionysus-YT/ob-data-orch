package identity

import (
	"context"
	"errors"
	"testing"
)

func TestValidateRejectsWrongOrMissingPrincipal(t *testing.T) {
	if err := Validate(Principal{Type: BrowserPrincipal, ID: "synthetic-subject"}, AgentPrincipal); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("wrong domain validation error = %v", err)
	}
	if err := Validate(Principal{Type: BrowserPrincipal}, BrowserPrincipal); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing principal validation error = %v", err)
	}
}

func TestCanFailsClosedWithoutExplicitGrant(t *testing.T) {
	principal := Principal{Type: BrowserPrincipal, ID: "synthetic-subject"}
	if err := Can(context.Background(), nil, principal, ScopeDataSourceRead, "source-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing authorizer error = %v", err)
	}
	if err := Can(context.Background(), denyAuthorizer{}, principal, ScopeDataSourceRead, "source-1"); !errors.Is(err, ErrDenied) {
		t.Fatalf("denied authorizer error = %v", err)
	}
}

type denyAuthorizer struct{}

func (denyAuthorizer) Authorize(context.Context, Principal, Scope, string) error {
	return errors.New("synthetic deny")
}
