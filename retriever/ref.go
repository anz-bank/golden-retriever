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
}

// NewSymbolicReference returns a new symbolic git reference including branch and tag. e.g. v0.0.1, main, develop.
func NewSymbolicReference(n string) *Reference {
	return &Reference{name: n}
}

// NewHashReference returns a new hash git reference.
func NewHashReference(h Hash) (*Reference, error) {
	if !h.IsValid() {
		return nil, fmt.Errorf("Invalid commit SHA %s", h)
	}
	return &Reference{hash: h}, nil
}

// HEADReference returns a HEAD git reference.
func HEADReference() *Reference {
	return &Reference{name: HEAD}
}

// NewReference returns a new Reference.
func NewReference(n string, h Hash) (*Reference, error) {
	if n == "" && h.IsZero() {
		return HEADReference(), nil
	}

	if h.IsValid() || h.IsZero() {
		return &Reference{
			name: n,
			hash: h,
		}, nil
	}

	return nil, fmt.Errorf("Invalid commit SHA %s", h)
}

// Hash returns the value of the reference name.
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

const (
	HEAD = "HEAD"
)

// IsHEAD reports whether the reference is HEAD.
func (r *Reference) IsHEAD() bool {
	return r.name == HEAD
}

// IsHEAD reports whether the reference is a HashReference.
func (r *Reference) IsHash() bool {
	return !r.hash.IsZero()
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
