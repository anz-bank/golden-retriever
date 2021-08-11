package reader

import (
	"context"

	"github.com/anz-bank/golden-retriever/retriever"
)

type Reader interface {
	Read(context.Context, string) ([]byte, error)
	ReadHash(context.Context, string) ([]byte, retriever.Hash, error)
}
