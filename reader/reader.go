package reader

import (
	"context"

	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/spf13/afero"
)

// Reader is the interface that reads local (and remote) file content.
type Reader interface {
	afero.Fs
	Read(context.Context, string) ([]byte, error)
	ReadHash(context.Context, string) ([]byte, retriever.Hash, error)
	ReadHashBranch(context.Context, string) ([]byte, retriever.Hash, string, error)
}
