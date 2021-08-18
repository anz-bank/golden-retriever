package pinner

import (
	"context"
	"errors"
	"fmt"

	"github.com/anz-bank/golden-retriever/retriever"
)

// Pinner is an implementation of Retriever interface with ability to pin git repository version
type Pinner struct {
	mod       *Mod
	retriever retriever.Retriever
}

// New intializes and returns new Pinner instance
func New(modFile string, retriever retriever.Retriever) (*Pinner, error) {
	if retriever == nil {
		return nil, errors.New("args cannot be nil")
	}

	mod, err := NewMod(modFile)
	if err != nil {
		return nil, err
	}

	return &Pinner{
		mod:       mod,
		retriever: retriever,
	}, nil
}

// Retrieve returns the bytes of the given resource.
// If no reference specified and the repository has been retrieved and pinned before, the pinned one will be returned.
func (m *Pinner) Retrieve(ctx context.Context, resource *retriever.Resource) (content []byte, err error) {
	onlyHash := (resource.Ref != nil && resource.Ref.IsHash() && resource.Ref.Name() == "")
	i, ok := m.mod.GetImport(resource.Repo)
	if ok && !onlyHash {
		switch {
		case resource.Ref == nil || resource.Ref.IsHEAD() || resource.Ref.IsEmpty() || resource.Ref.Name() == i.Ref:
			h, err := retriever.NewHash(i.Pinned)
			if err != nil {
				return nil, err
			}
			r, err := retriever.NewReference(i.Ref, h)
			if err != nil {
				return nil, fmt.Errorf("Module ref %s and pinned %s error: %s", i.Ref, i.Pinned, err.Error())
			}
			resource.Ref = r
		case resource.Ref.Name() != "" && i.Ref != "" && resource.Ref.Name() != i.Ref:
			return nil, fmt.Errorf("cannot import multiple versions (%s, %s) of a single repo %s", resource.Ref.Name(), i.Ref, resource.Repo)
		case resource.Ref.Name() != "" && resource.Ref.Name() == i.Ref && resource.Ref.Hash().String() != i.Pinned:
			return nil, fmt.Errorf("reference name %s and commit SHA %s not match", resource.Ref.Name(), resource.Ref.Hash().String())
		}
	}

	content, err = m.retriever.Retrieve(ctx, resource)
	if err != nil {
		return nil, err
	}

	if !ok && !onlyHash {
		im := &Import{Pinned: resource.Ref.Hash().String()}
		if resource.Ref.Name() != "" && resource.Ref.Name() != retriever.HEAD {
			im.Ref = resource.Ref.Name()
		}
		m.mod.SetImport(resource.Repo, im)
		err = m.mod.Save()
	}

	return
}
