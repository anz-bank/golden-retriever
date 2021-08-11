package retriever

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReference(t *testing.T) {
	h, err := NewHash("1e7c4cecaaa8f76e3c668cebc411f1b03171501f")
	require.NoError(t, err)

	refs := []struct {
		n   string
		h   Hash
		ref *Reference
		err func(require.TestingT, error, ...interface{})
	}{
		{"", ZeroHash, &Reference{HEAD, ZeroHash}, require.NoError},
		{"main", ZeroHash, &Reference{"main", ZeroHash}, require.NoError},
		{"foo", ZeroHash, &Reference{"foo", ZeroHash}, require.NoError},
		{"", h, &Reference{"", h}, require.NoError},
		{"foo", h, &Reference{"foo", h}, require.NoError},
	}

	for _, r := range refs {
		s := r.h.String()
		if r.h.IsZero() {
			s = "nohash"
		}
		t.Run(r.n+s, func(t *testing.T) {
			ref, err := NewReference(r.n, r.h)
			r.err(t, err)
			require.Equal(t, r.ref, ref)
		})
	}

}
