// Package domain holds the pure model of nexo: assets, installations,
// targets and health states. It has no dependencies beyond the standard
// library and performs no I/O — every rule here is testable in isolation.
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Type identifies the kind of asset nexo manages.
type Type string

const (
	TypeSkill  Type = "skill"
	TypePlugin Type = "plugin"
	// TypeMCP is modeled from day one (plan D12) so adding MCP support
	// later is additive, not structural. It is not implemented in v1.
	TypeMCP Type = "mcp"
)

// Valid reports whether t is a known asset type.
func (t Type) Valid() bool {
	switch t {
	case TypeSkill, TypePlugin, TypeMCP:
		return true
	}
	return false
}

// DefaultSource is the namespace assumed when an asset ID is written
// without one, e.g. "wordpress-review" → "local/wordpress-review".
const DefaultSource = "local"

// ID is the namespaced identity of an asset: source/name (plan D10).
// Namespacing exists from day one so registry assets never collide with
// local ones and no rename migration is needed later.
type ID struct {
	Source string
	Name   string
}

// ParseID parses "source/name" or a bare "name" (which gets
// DefaultSource). It rejects empty segments and whitespace.
func ParseID(s string) (ID, error) {
	parts := strings.Split(s, "/")
	var id ID
	switch len(parts) {
	case 1:
		id = ID{Source: DefaultSource, Name: parts[0]}
	case 2:
		id = ID{Source: parts[0], Name: parts[1]}
	default:
		return ID{}, fmt.Errorf("invalid asset id %q: expected [source/]name", s)
	}
	if err := id.Validate(); err != nil {
		return ID{}, err
	}
	return id, nil
}

// String renders the full namespaced form.
func (id ID) String() string {
	return id.Source + "/" + id.Name
}

// MarshalJSON persists the ID in its canonical "source/name" form.
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON parses the canonical form back, validating it.
func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Validate checks both segments are present and free of separators and
// whitespace.
func (id ID) Validate() error {
	for _, seg := range []string{id.Source, id.Name} {
		if seg == "" {
			return fmt.Errorf("invalid asset id %q: empty segment", id.Source+"/"+id.Name)
		}
		if strings.ContainsAny(seg, "/\\ \t\n") {
			return fmt.Errorf("invalid asset id segment %q: contains separator or whitespace", seg)
		}
	}
	return nil
}

// Asset is the generic object nexo manages. Its real identity is the
// content hash (plan D2): version is optional metadata because skill
// frontmatter has no version field in the wild, and "unversioned" is a
// first-class state — never an error.
type Asset struct {
	ID          ID
	Type        Type
	Version     string // optional; empty means unversioned
	Description string
	Hash        string // sha256 tree hash, hex-encoded
}

// Unversioned reports whether the asset carries no version metadata.
func (a Asset) Unversioned() bool { return a.Version == "" }

// Validate checks the asset is structurally sound. It does NOT require
// a version (see Unversioned) but does require a hash, because hashless
// assets cannot be drift-checked or safely uninstalled (plan D6).
func (a Asset) Validate() error {
	if err := a.ID.Validate(); err != nil {
		return err
	}
	if !a.Type.Valid() {
		return fmt.Errorf("asset %s: unknown type %q", a.ID, a.Type)
	}
	if a.Hash == "" {
		return fmt.Errorf("asset %s: missing content hash", a.ID)
	}
	return nil
}
