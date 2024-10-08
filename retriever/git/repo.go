package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
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

// InitWithRemote initialises a plain repository at the directory for the given repository, adding the appropriate remote.
func (a Git) InitWithRemote(_ context.Context, repo string) (*Repo, error) {
	log.Debugf("initialising repo: %v", repo)
	c, plain := a.cacher.(PlainFsCache)
	if !plain {
		return nil, fmt.Errorf("repository must be a plain repository")
	}
	rr, err := git.PlainInit(c.RepoDir(repo), false)
	if err != nil {
		return nil, fmt.Errorf("error initialising repository: %w", err)
	}
	return withAuth1(&a, repo, func(_ transport.AuthMethod, url string) (*Repo, error) {

		// Add the remote repository (using the authentication url).
		if _, err := rr.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{url},
		}); err != nil {
			return nil, fmt.Errorf("error creating repository remote: %v: %w", url, err)
		}
		return &Repo{&a, rr, repo}, nil
	})
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
// using the gh cli, if that fails it fetches the entire repo.
func (r *Repo) FetchRefOrAll(ctx context.Context, ref string, opts FetchOpts) error {
	err := r.FetchRef(ctx, ref, opts)
	if err == nil {
		return nil
	}

	// If hitting github.com then try to expand the ref to a hash using the github API (via the gh command-line)
	var err2, resolveErr error
	if strings.HasPrefix(r.repo, "github.com") {
		cmd := exec.Command("gh", "api", "/repos/"+r.repo[11:]+"/commits/"+ref, "--jq", ".sha")
		cmd.Env = append(cmd.Environ(), "GH_NO_UPDATE_NOTIFIER=TRUE")
		// Check if there is a gihub token in authmethods
		for _, meth := range r.g.authMethods {
			githubAuth, _ := meth.AuthMethod("github.com")
			if basicAuth, ok := githubAuth.(*http.BasicAuth); ok {
				cmd.Env = append(cmd.Env, "GH_TOKEN="+basicAuth.Password)
				break
			}
		}
		var out []byte
		out, resolveErr = cmd.Output()
		if resolveErr == nil {
			err2 = r.FetchRef(ctx, strings.TrimSpace(string(out)), opts)
			if err2 == nil {
				return nil
			}
		}
	}

	// the current version of go-git (v5.12.0) does not support --unshallow yet so set depth to infinite instead (see https://git-scm.com/docs/shallow)
	opts.Depth = 2147483647
	opts.Tags = FetchOptTagsAll
	err3 := r.Fetch(ctx, opts)
	if err3 != nil {
		return fmt.Errorf("error fetching ref or all: %w", errors.Join(err, err2, err3))
	}
	exists, err3 := r.Exists(ref)
	if err3 != nil {
		return fmt.Errorf("error fetching ref or all: %w", errors.Join(err, err2, err3))
	}
	if !exists {
		return fmt.Errorf("error fetching ref or all, ref: %v doesn't exist after fetch: %w", ref, err)
	}
	if resolveErr != nil {
		log.Infof("reference resolved after full fetch, to enable resolving the reference via API and not requiring a full fetch, install gh and either set GH_TOKEN or call 'gh auth login': %v", resolveErr)
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
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
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
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
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
func (r *Repo) ListRemoteRefs(ctx context.Context, remoteName string, opts ListOpts) (*[]*plumbing.Reference, error) {
	log.Debugf("listing all references from repository: %v remote with opts: %v", r, opts)
	return withAuth1(r.g, r.repo, func(auth transport.AuthMethod, url string) (*[]*plumbing.Reference, error) {
		if remoteName == "" {
			remoteName = "origin"
		}
		remote, err := r.r.Remote(remoteName)
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
