package retriever

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Retriever is the interface that wraps the Retrieve method.
// Retrieve fetches remote resource and return the bytes content.
type Retriever interface {
	// Retrieve resource and return resource content
	Retrieve(ctx context.Context, resource *Resource) (content []byte, err error)
}

// Resource represents git file resource.
type Resource struct {
	Repo     string
	Filepath string
	Ref      *Reference
}

// ParseResource takes a string in given format and returns the corresponding resource.
func ParseResource(str string, regexpStr string, repoidx, pathidx, refidx uint) (*Resource, error) {
	re, err := regexp.Compile(regexpStr)
	if err != nil {
		return nil, err
	}

	if !re.MatchString(str) {
		return nil, fmt.Errorf("%s doesn't match resource regexp %s", str, regexpStr)
	}

	m := re.FindStringSubmatch(str)

	ref := HEADReference()
	if isHash(m[refidx]) {
		h, err := NewHash(m[refidx])
		if err != nil {
			return nil, err
		}
		ref, err = NewHashReference(h)
		if err != nil {
			return nil, err
		}
	} else if m[refidx] != "" {
		ref = NewSymbolicReference(m[refidx])
	}

	return &Resource{
		Repo:     m[repoidx],
		Filepath: strings.TrimPrefix(m[pathidx], "/"),
		Ref:      ref,
	}, nil
}

// String returns resource representing in string.
func (r *Resource) String() string {
	return fmt.Sprintf("%s/%s@%s", r.Repo, r.Filepath, r.Ref.String())
}
