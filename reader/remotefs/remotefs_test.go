package remotefs

import (
	"testing"

	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/stretchr/testify/require"
)

func TestParseResource(t *testing.T) {
	r := &RemoteFs{}

	ref := retriever.NewSymbolicReference("ref")
	refref := retriever.NewSymbolicReference("ref.ref")
	featurerefref := retriever.NewSymbolicReference("feature/ref.ref")
	tests := []struct {
		str      string
		resource *retriever.Resource
		err      func(require.TestingT, error, ...interface{})
	}{
		{"//github.com", nil, require.Error},
		{"//github.com/foo", nil, require.Error},
		{"//github.com/foo/bar", nil, require.Error},
		{"//github.com/foo/bar/file/path", &retriever.Resource{Repo: "github.com/foo/bar", Filepath: "file/path", Ref: retriever.HEADReference()}, require.NoError},
		{"//github.com/foo/bar/file/path@ref", &retriever.Resource{Repo: "github.com/foo/bar", Filepath: "file/path", Ref: ref}, require.NoError},
		{"//github.com/foo-foo/bar_bar/file/path.et@ref.ref", &retriever.Resource{Repo: "github.com/foo-foo/bar_bar", Filepath: "file/path.et", Ref: refref}, require.NoError},
		{"//github.com/foo-foo/bar_bar/file/path.et@feature/ref.ref", &retriever.Resource{Repo: "github.com/foo-foo/bar_bar", Filepath: "file/path.et", Ref: featurerefref}, require.NoError},
	}

	for _, test := range tests {
		t.Run(test.str, func(t *testing.T) {
			resource, err := r.ParseResource(test.str)
			test.err(t, err)
			require.Equal(t, test.resource, resource)
		})
	}
}
