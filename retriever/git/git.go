package git

import (
	"context"
	"fmt"
	"github.com/anz-bank/golden-retriever/once"
	"os"
	"strings"

	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

func init() {
	log.SetLevel(log.WarnLevel)

	proxy.RegisterDialerType("http", httpProxy)
}

func isReferenceNotFoundErr(err error) bool {
	return nomatchspecErr.Is(err) || plumbing.ErrReferenceNotFound == err
}

var nomatchspecErr = git.NoMatchingRefSpecError{}

// Clone a repository into the given cache directory.
func (a Git) Clone(ctx context.Context, resource *retriever.Resource) (r *git.Repository, err error) {
	return a.CloneWithOpts(ctx, resource, CloneOpts{Depth: 1})
}

type CloneOpts struct {
	Depth int
}

// CloneWithOpts clones a repository into the given cache directory using the given options.
func (a Git) CloneWithOpts(ctx context.Context, resource *retriever.Resource, opts CloneOpts) (r *git.Repository, err error) {
	repo := resource.Repo
	c, isPlain := a.cacher.(PlainFsCache)

	if resource.Ref.IsHash() {
		if isPlain {
			r, err = git.PlainInit(c.RepoDir(repo), false)
		} else {
			r, err = git.Init(a.cacher.NewStorer(repo), nil)
		}
		if err != nil {
			return nil, err
		}

		err = a.FetchCommitWithOpts(ctx, r, repo, resource.Ref.Hash(), FetchOpts{Depth: opts.Depth})
		return
	}

	tried := []string{}

	for _, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options := &git.CloneOptions{
			URL:   url,
			Depth: opts.Depth,
			Auth:  auth,
		}

		ref := resource.Ref.Name()
		rules := retriever.RefRules
		if ref == "HEAD" {
			ref = "heads"
			rules = []string{"refs/%s/master", "refs/%s/main"}
		}

		for iter := retriever.NewRefIterator(rules, ref); iter.Next(); {
			options.ReferenceName = plumbing.ReferenceName(iter.Current())

			if isPlain {
				r, err = git.PlainCloneContext(ctx, c.RepoDir(repo), false, options)
			} else {
				r, err = git.CloneContext(ctx, a.cacher.NewStorer(repo), memfs.New(), options)
			}
			if err == nil {
				return r, nil
			}
		}

		errmsg := err.Error()
		if isReferenceNotFoundErr(err) {
			errmsg = fmt.Sprintf("reference %s not found", ref)
		}
		if transport.ErrRepositoryNotFound == err {
			errmsg = fmt.Sprintf("repository %s not found", repo)
		}
		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), errmsg))
	}

	return nil, fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

func (a Git) Fetch(ctx context.Context, r *git.Repository, resource *retriever.Resource) error {
	if resource.Ref.IsHash() {
		return a.FetchCommit(ctx, r, resource.Repo, resource.Ref.Hash())
	}
	return a.FetchRef(ctx, r, resource.Repo, resource.Ref.Name())
}

// FetchRef fetches specific reference
func (a Git) FetchRef(ctx context.Context, r *git.Repository, repo string, ref string) (err error) {
	tried := []string{}
	for iter := retriever.NewRefIterator(retriever.RefRules, ref); iter.Next(); {
		refSpec := iter.Current()
		if refSpec == "HEAD" {
			refSpec = fmt.Sprintf("+%s:refs/remotes/origin/%[1]s", "HEAD")
		} else {
			refSpec = fmt.Sprintf("+%s:%[1]s", refSpec)
		}
		err = a.FetchRefSpec(ctx, r, repo, config.RefSpec(refSpec), FetchOpts{Depth: 1})
		if err == nil {
			return nil
		}
		errmsg := err.Error()
		tried = append(tried, fmt.Sprintf("    - %s: %s", refSpec, errmsg))
	}

	return fmt.Errorf("Unable to find reference, tried: \n%s", strings.Join(tried, ",\n"))
}

type FetchOpts struct {
	Depth int
	Force bool
}

// FetchRefSpec fetches a specific reference specification
func (a Git) FetchRefSpec(ctx context.Context, r *git.Repository, repo string, spec config.RefSpec, opts FetchOpts) (err error) {
	options := &git.FetchOptions{
		Depth:    opts.Depth,
		Force:    opts.Force,
		Progress: os.Stdout,
	}

	var tried []string
	for _, meth := range a.authMethods {
		auth, _ := meth.AuthMethod(repo)
		options.Auth = auth
		options.RefSpecs = []config.RefSpec{spec}
		err = r.FetchContext(ctx, options)
		if err == nil || err == git.NoErrAlreadyUpToDate {
			return nil
		}

		errmsg := err.Error()
		if nomatchspecErr.Is(err) {
			errmsg = fmt.Sprintf("reference %s not found", spec)
		}
		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), errmsg))
	}

	return fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

// FetchCommit the latest history of a repository in the cache directory.
func (a Git) FetchCommit(ctx context.Context, r *git.Repository, repo string, hash retriever.Hash) error {
	return a.FetchCommitWithOpts(ctx, r, repo, hash, FetchOpts{Depth: 1})
}

func (a Git) FetchCommitWithOpts(ctx context.Context, r *git.Repository, repo string, hash retriever.Hash, opts FetchOpts) error {
	_, err := r.CommitObject(plumbing.NewHash(hash.String()))
	if err == nil {
		return nil
	}

	isEmpty := false
	remotes, err := r.Remotes()
	if err != nil {
		return err
	} else if len(remotes) == 0 {
		isEmpty = true
	}

	refSpec := fmt.Sprintf("%s:%[1]s", hash)
	options := &git.FetchOptions{
		Depth:    opts.Depth,
		RefSpecs: []config.RefSpec{config.RefSpec(refSpec)},
	}

	tried := []string{}
	for i, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options.Auth = auth

		if isEmpty {
			switch {
			case i > 0:
				remote, err := r.Remote(git.DefaultRemoteName)
				if err != nil && err != git.ErrRemoteNotFound {
					return err
				}

				if remote.Config().URLs[0] == url {
					break
				}

				err = r.DeleteRemote(git.DefaultRemoteName)
				if err != nil {
					return err
				}
				fallthrough
			case i == 0:
				if _, err = r.CreateRemote(&config.RemoteConfig{
					Name: git.DefaultRemoteName,
					URLs: []string{url},
				}); err != nil && err != git.ErrRemoteExists {
					return err
				}
			}
		}

		err = r.FetchContext(ctx, options)
		if err == nil || err == git.NoErrAlreadyUpToDate {
			return nil
		}

		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), err.Error()))
	}

	return fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

// Show the content of a file with given file path and git reference in the cache directory.
func (a Git) Show(r *git.Repository, resource *retriever.Resource) ([]byte, error) {
	if !resource.Ref.IsHash() {
		err := a.ResolveReference(r, resource)
		if err != nil {
			return nil, err
		}
	}

	commit, err := r.CommitObject(plumbing.NewHash(resource.Ref.Hash().String()))
	if err != nil {
		if err == plumbing.ErrObjectNotFound {
			return nil, fmt.Errorf("object of commit %s not found", resource.Ref.Hash())
		}
		return nil, err
	}

	f, err := commit.File(resource.Filepath)
	if err != nil {
		return nil, err
	}
	contents, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(contents), nil
}

type checkoutOpts struct {
	force bool
}

// Checkout the repository at the reference of the given retriever.
func (a Git) checkout(r *git.Repository, resource *retriever.Resource, opts checkoutOpts) error {
	err := a.ResolveReference(r, resource)
	if err != nil {
		return err
	}

	worktree, err := r.Worktree()
	if err != nil {
		return err
	}

	return worktree.Checkout(&git.CheckoutOptions{
		Hash:  plumbing.NewHash(resource.Ref.Hash().String()),
		Force: opts.force,
	})
}

// ResolveReference resolves a SymbolicReference to a HashReference.
func (a Git) ResolveReference(r *git.Repository, resource *retriever.Resource) (err error) {
	if resource.Ref == nil {
		resource.Ref = retriever.HEADReference()
	}

	if resource.Ref.IsHash() {
		return nil
	}

	var h *plumbing.Hash
	rev := resource.Ref.Name()
	if rev == "HEAD" {
		ref, e := r.Reference("HEAD", false)
		if e == nil {
			resource.Ref.SetName(strings.TrimPrefix(ref.Target().String(), "refs/heads/"))
		}
		h, err = r.ResolveRevision(plumbing.Revision("refs/remotes/origin/HEAD"))
	}

	if err != nil || rev != "HEAD" {
		h, err = r.ResolveRevision(plumbing.Revision(rev))
		if err != nil {
			if err == plumbing.ErrReferenceNotFound {
				return fmt.Errorf("reference %s not found", rev)
			}
			return
		}
	}

	hash, err := retriever.NewHash(h.String())
	if err != nil {
		return
	}
	resource.Ref.SetHash(hash)
	return
}

// Session provides a mechanism to ensure that repeat requests to set the content of a repository to a given
// reference are guaranteed to be stable. For example, if an initial request is made to set a repository to contents of
// the remote 'main' branch, then subsequent requests within the same session to set the repository to the remote 'main'
// branch are guaranteed to set the repository to the same state, even if the 'main' branch on the remote repository has
// changed since it was first retrieved.
type Session interface {
	// Set the content of the repository to the given origin reference.
	// The reference (ref) can be one of the following formats:
	// 1. Branch name: e.g. main
	// 2. Tag name: e.g. tags/t
	// 3. Hash: e.g. 865e3e5c6fca0120285c3aa846fdb049f8f074e6
	Set(ctx context.Context, repo string, ref string, opts SessionSetOpts) error
}

// SessionSetOpts provide configuration to the Session/Set
type SessionSetOpts struct {

	// Whether known symbolic references should be fetched and updated from the remote repository the first time it is
	// accessed, even if the reference is already known in the local repository.
	Fetch bool

	// Whether changes to the repository should be forced.
	Force bool

	// FIXME: A couple of issues exist within go-git that prevents the sensible use of this value in all scenarios.
	// The recommendation is therefore to fetch values at depth zero (fetch all values) until these issues are resolved.
	// https://github.com/go-git/go-git/issues/305
	// https://github.com/go-git/go-git/issues/328
	Depth int // The depth at which content should be fetched.
}

type sessionImpl struct {
	once   once.Once
	hashes map[string]retriever.Hash
	g      *Git
}

func NewSession(g *Git) Session {
	return sessionImpl{
		once:   once.NewOnce(),
		hashes: make(map[string]retriever.Hash),
		g:      g}
}

func (s sessionImpl) Set(ctx context.Context, repo string, ref string, opts SessionSetOpts) error {
	key := repo + "@" + ref
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		ch := s.once.Register(key)
		defer s.once.Unregister(key)
		if ch != nil {
			<-ch
		}

		hash, ok := s.hashes[key]
		if ok {
			reference, err := retriever.NewHashReference(hash)
			if err != nil {
				return err
			}
			resource := retriever.Resource{Repo: repo, Ref: reference}
			return s.g.Set(ctx, &resource, SetOpts{Fetch: false, Force: opts.Force, Depth: opts.Depth})
		} else {
			reference, err := resolveReference(ref)
			if err != nil {
				return err
			}
			resource := retriever.Resource{Repo: repo, Ref: reference}
			err = s.g.Set(ctx, &resource, SetOpts{Fetch: opts.Fetch, Force: opts.Force, Depth: opts.Depth})
			if err != nil {
				return err
			}
			s.hashes[key] = resource.Ref.Hash()
			return nil
		}
	}
}

func resolveReference(ref string) (*retriever.Reference, error) {
	hash, err := retriever.NewHash(ref)
	if err == nil {
		return retriever.NewHashReference(hash)
	}
	return retriever.NewSymbolicReference(ref), nil
}
