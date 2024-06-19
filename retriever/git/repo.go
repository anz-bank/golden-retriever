package git

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	log "github.com/sirupsen/logrus"
)

type Repo struct {
	g    *Git
	r    *git.Repository
	repo string
}

func (r *Repo) String() string {
	return r.repo
}

// CloneRepo clones the given repository.
//
// Note: This function only supports plain (i.e. file system) caches.
func (a Git) CloneRepo(ctx context.Context, repo string, opts CloneOpts) (*Repo, error) {
	log.Debugf("cloning repo: %v with opts: %v", repo, opts)
	c, plain := a.cacher.(PlainFsCache)
	if !plain {
		return nil, fmt.Errorf("repository must be a plain repository")
	}
	tags := opts.Tags.TagMode(git.AllTags)
	r, err := withAuth1(&a, repo, func(auth transport.AuthMethod, url string) (*git.Repository, error) {
		return git.PlainCloneContext(ctx, c.RepoDir(repo), false, &git.CloneOptions{
			URL:          url,
			Depth:        opts.Depth,
			Auth:         auth,
			SingleBranch: opts.SingleBranch,
			NoCheckout:   opts.NoCheckout,
			Tags:         tags})
	})
	if err != nil {
		return nil, err
	}
	return &Repo{&a, r, repo}, nil
}

// FetchRefOrAll fetches the reference from the remote repository, falling back to attempting to resolve the reference
// by fetching all references from the remote repository.
func (r *Repo) FetchRefOrAll(ctx context.Context, ref string, opts FetchOpts) error {
	err := r.FetchRef(ctx, ref, opts)
	if err == nil {
		return nil
	}
	opts.Tags = FetchOptTagsAll
	err2 := r.Fetch(ctx, opts)
	if err2 != nil {
		return fmt.Errorf("error fetching ref or all: %w", errors.Join(err, err2))
	}
	exists, err2 := r.Exists(ref)
	if err2 != nil {
		return fmt.Errorf("error fetching ref or all: %w", errors.Join(err, err2))
	}
	if !exists {
		return fmt.Errorf("error fetching ref or all, ref: %v doesn't exist after fetch: %w", ref, err)
	}
	return nil
}

// FetchRef fetches the reference from the remote repository.
//
// Note: This function does not support short hashes (e.g. 1e7c4cec) due to the following failure:
// couldn't find remote ref
// Full hash values must be used in their place.
func (r *Repo) FetchRef(ctx context.Context, ref string, opts FetchOpts) error {
	spec := config.RefSpec(fmt.Sprintf("+%s:%[1]s", ref))
	log.Debugf("fetching ref: %v from repo: %v with spec: %v and opts: %v", ref, r, spec, opts)
	tags := opts.Tags.TagMode(git.TagFollowing)
	return withAuth0(r.g, r.repo, func(auth transport.AuthMethod, url string) error {
		err := r.r.FetchContext(ctx, &git.FetchOptions{
			Depth:     opts.Depth,
			Force:     opts.Force,
			Auth:      auth,
			RemoteURL: url,
			RefSpecs:  []config.RefSpec{spec},
			Tags:      tags,
		})
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	})
}

// Fetch fetches all references within the remote repository.
func (r *Repo) Fetch(ctx context.Context, opts FetchOpts) (err error) {
	spec := config.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/origin/*"))
	log.Debugf("fetching all references from repo: %v with spec: %v and opts: %v", r, spec, opts)
	tags := opts.Tags.TagMode(git.TagFollowing)
	return withAuth0(r.g, r.repo, func(auth transport.AuthMethod, url string) error {
		err = r.r.FetchContext(ctx, &git.FetchOptions{
			Depth:     opts.Depth,
			Force:     opts.Force,
			Auth:      auth,
			RemoteURL: url,
			RefSpecs:  []config.RefSpec{spec},
			Tags:      tags,
		})
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	})
}

type ListOpts struct{}

func (o ListOpts) String() string {
	return "{}"
}

// ListRemoteRefs lists all references in the remote repository.
func (r *Repo) ListRemoteRefs(ctx context.Context, remote string, opts ListOpts) (*[]*plumbing.Reference, error) {
	log.Debugf("listing all references from repository: %v remote with opts: %v", r, opts)
	return withAuth1(r.g, r.repo, func(auth transport.AuthMethod, url string) (*[]*plumbing.Reference, error) {
		remote, err := r.r.Remote("origin")
		if err != nil {
			return nil, fmt.Errorf("error fetching remote: %w", err)
		}
		result, err := remote.ListContext(ctx, &git.ListOptions{
			Auth: auth,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing context: %w", err)
		}
		return &result, nil
	})
}

type CheckoutOpts struct {
	Force bool
}

func (o CheckoutOpts) String() string {
	return fmt.Sprintf("{Force:%v}", o.Force)
}

// Checkout checks out the repository at the given reference.
func (r *Repo) Checkout(ref string, opts CheckoutOpts) error {
	log.Debugf("checking out repo: %v to reference: %v with opts: %v", r, ref, opts)
	hash, err := r.r.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("error resolving revision in repo: %v for reference: %v: %w", r, ref, err)
	}

	worktree, err := r.r.Worktree()
	if err != nil {
		return err
	}

	return worktree.Checkout(&git.CheckoutOptions{
		Hash:  *hash,
		Force: opts.Force,
	})
}

// IsClean returns whether all files in the repository are unmodified.
func (r *Repo) IsClean() (bool, error) {
	log.Debugf("checking clean status of repo: %v", r)
	worktree, err := r.r.Worktree()
	if err != nil {
		return false, err
	}
	status, err := worktree.Status()
	if err != nil {
		return false, err
	}
	return status.IsClean(), nil
}

// ResolveHash returns the string representation of the hash value for the given reference.
func (r *Repo) ResolveHash(ref string) (string, error) {
	log.Debugf("resolving hash in repo: %v for reference: %v", r, ref)
	hash, err := r.r.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", fmt.Errorf("error resolving revision for reference: %v: %w", ref, err)
	}
	return hash.String(), nil
}

// Exists returns whether the reference exists within the repository.
func (r *Repo) Exists(ref string) (bool, error) {
	_, err := r.ResolveHash(ref)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func withAuth0(g *Git, repo string, f func(auth transport.AuthMethod, url string) error) error {
	var errs []error
	for _, meth := range g.authMethods {
		auth, url := meth.AuthMethod(repo)
		err := f(auth, url)
		if err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Errorf("error executing operation with auth: %v: %w", meth.Name(), err))
		}
	}
	return errors.Join(errs...)
}

func withAuth1[T any](g *Git, repo string, f func(auth transport.AuthMethod, url string) (*T, error)) (*T, error) {
	var errs []error
	for _, meth := range g.authMethods {
		auth, url := meth.AuthMethod(repo)
		t, err := f(auth, url)
		if err == nil {
			return t, nil
		} else {
			errs = append(errs, fmt.Errorf("error executing operation with auth: %v: %w", meth.Name(), err))
		}
	}
	return nil, errors.Join(errs...)
}
