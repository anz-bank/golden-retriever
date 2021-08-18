package reader

import (
	"context"

	"github.com/anz-bank/golden-retriever/retriever"
)

// Reader is the interface that reads local (and remote) file content.
type Reader interface {
	Read(context.Context, string) ([]byte, error)
	ReadHash(context.Context, string) ([]byte, retriever.Hash, error)
}
