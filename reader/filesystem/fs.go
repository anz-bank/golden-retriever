package filesystem

import (
	"context"
	"io/ioutil"

	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/spf13/afero"
)

// Fs is an implementation of interface Reader. Basically a wrap of afero.Fs.
type Fs struct {
	afero.Fs
}

// New initializes and returns an instance of Fs.
func New(fs afero.Fs) *Fs {
	return &Fs{fs}
}

// Read returns the contents of the given local file.
func (fs Fs) Read(_ context.Context, path string) ([]byte, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return ioutil.ReadAll(file)
}

// ReadHash returns the contents of the given local file and an empty hash.
func (fs Fs) ReadHash(ctx context.Context, path string) ([]byte, retriever.Hash, error) {
	b, err := fs.Read(ctx, path)

	return b, retriever.ZeroHash, err
}

// ReadHashBranch returns the contents of the given local file and empty hash and branch.
func (fs Fs) ReadHashBranch(ctx context.Context, path string) ([]byte, retriever.Hash, string, error) {
	b, err := fs.Read(ctx, path)

	return b, retriever.ZeroHash, "", err
}
