package pinner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/anz-bank/golden-retriever/retriever/git"
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

// NewWithGit initializes and returns an instance of Pinner with retriever git.Git.
func NewWithGit(modFile string, options *git.AuthOptions) (*Pinner, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	cacheDir := filepath.Join(userCacheDir, "ANZ.GoldenRetriever")
	return New(modFile, git.NewWithCache(options, git.NewFscache(cacheDir)))
}

// Retrieve returns the bytes of the given resource.
// If no reference specified and the repository has been retrieved and pinned before, the pinned one will be returned.
func (m *Pinner) Retrieve(ctx context.Context, resource *retriever.Resource) (content []byte, err error) {
	if i, ok := m.mod.GetImport(resource.Repo); ok {
		if resource.Ref == nil || (resource.Ref.Name() == "" && resource.Ref.Hash().IsZero()) {
			h, err := retriever.NewHash(i.Pinned)
			if err != nil {
				return nil, err
			}
			r, err := retriever.NewReference(i.Ref, h)
			if err != nil {
				return nil, fmt.Errorf("Module ref %s and pinned %s error: %s", i.Ref, i.Pinned, err.Error())
			}
			resource.Ref = r
		}
	}

	content, err = m.retriever.Retrieve(ctx, resource)
	if err != nil {
		return nil, err
	}

	im := &Import{Pinned: resource.Ref.Hash().String()}
	if resource.Ref.Name() != "" && resource.Ref.Name() != retriever.HEAD {
		im.Ref = resource.Ref.Name()
	}
	m.mod.SetImport(resource.Repo, im)
	err = m.mod.Save()

	return
}
