package commandgen

import "fmt"

type Platform string

const (
	PlatformWindowsAMD64 Platform = "WINDOWS_AMD64"
	PlatformLinuxAMD64   Platform = "LINUX_AMD64"
	PlatformLinuxARM64   Platform = "LINUX_ARM64"
)

type ConnectionKind string

const (
	ConnectionObserverDirect ConnectionKind = "OBSERVER_DIRECT"
	ConnectionODP            ConnectionKind = "ODP"
	ConnectionPublicCloud    ConnectionKind = "PUBLIC_CLOUD"
	ConnectionLogicalDB      ConnectionKind = "LOGICAL_DATABASE"
)

type ValueSource string

const (
	SourceUser       ValueSource = "USER"
	SourceDataSource ValueSource = "DATA_SOURCE"
	SourceSecurity   ValueSource = "SECURITY"
	SourceFormat     ValueSource = "FORMAT"
)

type ValueKind string

const (
	ValueString          ValueKind = "STRING"
	ValueInteger         ValueKind = "INTEGER"
	ValueBoolean         ValueKind = "BOOLEAN"
	ValueSecretReference ValueKind = "SECRET_REFERENCE"
)

type FieldState string

const (
	StateUnset    FieldState = "UNSET"
	StateExplicit FieldState = "EXPLICIT"
	StateDerived  FieldState = "DERIVED"
	StateInactive FieldState = "INACTIVE"
	StateBlocked  FieldState = "BLOCKED"
)

type CredentialReference struct {
	CredentialID string `json:"credentialId"`
	Revision     int64  `json:"revision"`
}

// Value deliberately has no plaintext-secret member. Passwords can only be
// represented as a versioned CredentialReference.
type Value struct {
	Kind    ValueKind
	String  string
	Integer int64
	Boolean bool
	Secret  *CredentialReference
}

type FieldInput struct {
	Name   string
	Source ValueSource
	Value  Value
}

type Request struct {
	Tool                  string
	ToolVersion           string
	MetadataVersion       string
	CapabilityVersion     string
	ConnectionKind        ConnectionKind
	DataSourceFactVersion string
	NodeFactVersion       string
	TargetPlatform        Platform
	Fields                []FieldInput
}

type NormalizedValue struct {
	Kind    ValueKind            `json:"kind,omitempty"`
	String  string               `json:"string,omitempty"`
	Integer *int64               `json:"integer,omitempty"`
	Boolean *bool                `json:"boolean,omitempty"`
	Secret  *CredentialReference `json:"secretReference,omitempty"`
}

type NormalizedField struct {
	DefinitionID string          `json:"definitionId"`
	Name         string          `json:"name"`
	State        FieldState      `json:"state"`
	Source       ValueSource     `json:"source,omitempty"`
	Value        NormalizedValue `json:"value,omitempty"`
}

type TokenEvidence struct {
	DefinitionID string      `json:"definitionId"`
	Parameter    string      `json:"parameter"`
	Source       ValueSource `json:"source"`
	ArgvStart    int         `json:"argvStart"`
	ArgvLength   int         `json:"argvLength"`
}

type SecretSlot struct {
	SlotID              string              `json:"slotId"`
	Target              string              `json:"target"`
	SecurityProperty    string              `json:"securityProperty"`
	CredentialReference CredentialReference `json:"credentialReference"`
}

type Result struct {
	Tool                string            `json:"tool"`
	ToolVersion         string            `json:"toolVersion"`
	MetadataVersion     string            `json:"metadataVersion"`
	CapabilityVersion   string            `json:"capabilityVersion"`
	NormalizedFields    []NormalizedField `json:"normalizedFields"`
	ArgvTemplate        []string          `json:"argvTemplate"`
	RedactedCommand     string            `json:"redactedCommand"`
	SecretSlots         []SecretSlot      `json:"secretSlots"`
	SecretSourceSummary string            `json:"secretSourceSummary"`
	TokenEvidence       []TokenEvidence   `json:"tokenEvidence"`
	ConfigFingerprint   string            `json:"configFingerprint"`
}

type Issue struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
}

type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("command generation blocked by %d validation issue(s)", len(e.Issues))
}
