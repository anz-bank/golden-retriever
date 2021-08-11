package mock

import (
	"context"
	"errors"

	"github.com/anz-bank/golden-retriever/retriever"
)

// Retriever is a mock implementation of the Retriever interface.
type Retriever struct{}

func (r Retriever) Retrieve(ctx context.Context, resource *retriever.Resource) (content []byte, err error) {
	switch {
	case resource.Ref == nil:
		resource.Ref = retriever.HEADReference()
		fallthrough
	case resource.Ref.Name() == retriever.HEAD:
		if err = resource.Ref.SetHash(r.HEADHash()); err != nil {
			return nil, err
		}
		return r.HEADContent(), nil
	case resource.Ref.IsHash():
		return r.HashContent(), nil
	case resource.Ref.Name() == "master":
		if err = resource.Ref.SetHash(r.BranchHash()); err != nil {
			return nil, err
		}
		return r.BranchContent(), nil
	case resource.Ref.Name() == "v1":
		if err = resource.Ref.SetHash(r.TagHash()); err != nil {
			return nil, err
		}
		return r.TagContent(), nil
	}
	return nil, errors.New("Unknown case")
}

func (Retriever) HashContent() []byte {
	return []byte("content of a commit")
}

func (Retriever) HEADContent() []byte {
	return []byte("content in HEAD")
}

func (Retriever) HEADHash() retriever.Hash {
	h, _ := retriever.NewHash("133416d690dbffc8fe321e12bdd4f21d79e2a479")
	return h
}

func (Retriever) BranchContent() []byte {
	return []byte("content of a branch")
}

func (Retriever) BranchHash() retriever.Hash {
	h, _ := retriever.NewHash("233416d690dbffc8fe321e12bdd4f21d79e2a479")
	return h
}

func (Retriever) TagContent() []byte {
	return []byte("content of v1")
}

func (Retriever) TagHash() retriever.Hash {
	h, _ := retriever.NewHash("333416d690dbffc8fe321e12bdd4f21d79e2a479")
	return h
}
