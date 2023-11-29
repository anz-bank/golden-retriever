package retriever

import (
	"fmt"
	"regexp"

	"github.com/go-git/go-git/v5/plumbing"
)

var RefRules = append([]string{"%s"}, plumbing.RefRevParseRules...)

// Reference represents a git reference.
type Reference struct {
	name string
	hash Hash
	typ  ReferenceType
}

type ReferenceType int

const (
	ReferenceTypeUnknown  ReferenceType = iota
	ReferenceTypeSymbolic               // branch or tag
	ReferenceTypeBranch
	ReferenceTypeTag
	ReferenceTypeHash
)

// NewBranchReference returns a new git reference to a branch.
func NewBranchReference(name string) *Reference {
	return &Reference{name: name, typ: ReferenceTypeBranch}
}

// NewTagReference returns a new git reference to a tag.
func NewTagReference(tag string) *Reference {
	return &Reference{name: tag, typ: ReferenceTypeTag}
}

// NewSymbolicReference returns a new symbolic git reference including branch and tag. e.g. v0.0.1, main, develop.
// Note: Where possible prefer NewBranchReference or NewTagReference.
func NewSymbolicReference(n string) *Reference {
	return &Reference{name: n, typ: ReferenceTypeSymbolic}
}

// NewHashReference returns a new hash git reference.
func NewHashReference(h Hash) (*Reference, error) {
	if !h.IsValid() {
		return nil, fmt.Errorf("Invalid commit SHA %s", h)
	}
	return &Reference{hash: h, typ: ReferenceTypeHash}, nil
}

// HEADReference returns a HEAD git reference.
func HEADReference() *Reference {
	return &Reference{name: HEAD, typ: ReferenceTypeBranch}
}

// NewReference returns a new Reference.
func NewReference(n string, h Hash) (*Reference, error) {
	if n != "" {
		ref := NewSymbolicReference(n)
		if h.IsValid() {
			err := ref.SetHash(h)
			if err != nil {
				return nil, err
			}
		}
		return ref, nil
	} else {
		if h.IsZero() {
			return HEADReference(), nil
		} else {
			return NewHashReference(h)
		}
	}
}

// Name returns the value of the reference name.
func (r *Reference) Name() string {
	return r.name
}

// SetName sets the value of the name.
func (r *Reference) SetName(name string) {
	r.name = name
}

// Hash returns the value of the hash.
func (r *Reference) Hash() Hash {
	return r.hash
}

// SetHash sets the value of the hash.
func (r *Reference) SetHash(hash Hash) error {
	if hash.IsZero() {
		return fmt.Errorf("Invalid commit SHA")
	}
	r.hash = hash
	return nil
}

func (r *Reference) Type() ReferenceType {
	return r.typ
}

const (
	HEAD = "HEAD"
)

// IsHEAD reports whether the reference is HEAD.
func (r *Reference) IsHEAD() bool {
	return r.name == HEAD
}

// IsEmpty reports whether the reference is empty.
func (r *Reference) IsEmpty() bool {
	return r == &Reference{}
}

// IsHash reports whether the reference is a HashReference.
func (r *Reference) IsHash() bool {
	return !r.hash.IsZero()
}

// String returns reference representing in string.
func (r *Reference) String() string {
	if r.IsHash() {
		return r.hash.String()
	}
	return r.name
}

type Hash [40]byte

var ZeroHash Hash

// NewHash returns a new Hash.
func NewHash(s string) (Hash, error) {
	if !isHash(s) {
		return ZeroHash, fmt.Errorf("Invalid commit SHA")
	}

	var h Hash
	copy(h[:], []byte(s))
	return h, nil
}

// IsZero reports whether a Hash is empty.
func (h Hash) IsZero() bool {
	return h == ZeroHash
}

func (h Hash) String() string {
	if h.IsZero() {
		return ""
	}
	return string(h[:])
}

// IsValid reports whether a Hash is valid.
func (h Hash) IsValid() bool {
	return isHash(h.String())
}

func isHash(str string) bool {
	if len(str) == 40 {
		if e, err := regexp.MatchString(`[a-fA-F0-9]{40}`, str); err == nil {
			return e
		}
	}
	return false
}

type RefIterator struct {
	rules   []string
	ref     string
	currIdx int
}

func NewRefIterator(rules []string, ref string) *RefIterator {
	iter := &RefIterator{
		rules:   rules,
		ref:     ref,
		currIdx: -1,
	}
	if ref == HEAD {
		iter.rules = []string{"%s"}
	}

	return iter
}

func (i *RefIterator) Next() bool {
	i.currIdx += 1
	return i.currIdx < len(i.rules)
}

func (i *RefIterator) Current() string {
	return fmt.Sprintf(i.rules[i.currIdx], i.ref)
}
