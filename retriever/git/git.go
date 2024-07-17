package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"

	"github.com/anz-bank/golden-retriever/once"
	"github.com/anz-bank/golden-retriever/retriever"
)

func init() {
	log.SetLevel(log.WarnLevel)

	proxy.RegisterDialerType("http", httpProxy)
}

func isReferenceNotFoundErr(err error) bool {
	return nomatchspecErr.Is(err) || errors.Is(err, plumbing.ErrReferenceNotFound)
}

var nomatchspecErr = git.NoMatchingRefSpecError{}

// Clone a repository into the given cache directory.
func (a Git) Clone(ctx context.Context, resource *retriever.Resource) (r *git.Repository, err error) {
	return a.CloneWithOpts(ctx, resource, CloneOpts{Depth: 1})
}

type CloneOpts struct {
	Depth        int
	SingleBranch bool // warning do not set this to true if the reference could be a tag
	NoCheckout   bool
	Tags         OptTags
}

func (o CloneOpts) String() string {
	return fmt.Sprintf("{Depth:%v, SingleBranch:%v, NoCheckout:%v, Tags:%v}",
		o.Depth, o.SingleBranch, o.NoCheckout, o.Tags)
}

// CloneWithOpts clones a repository into the given cache directory using the given options.
func (a Git) CloneWithOpts(ctx context.Context, resource *retriever.Resource, opts CloneOpts) (r *git.Repository, err error) {
	log.Debugf("cloning repository to resource: %v with opts: %v", resource, opts)
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

	tags := opts.Tags.TagMode(git.AllTags)

	for _, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options := &git.CloneOptions{
			URL:           url,
			Depth:         opts.Depth,
			Auth:          auth,
			SingleBranch:  opts.SingleBranch,
			ReferenceName: plumbing.ReferenceName(resource.Ref.Name()),
			NoCheckout:    opts.NoCheckout,
			Tags:          tags,
		}

		if isPlain {
			r, err = git.PlainCloneContext(ctx, c.RepoDir(repo), false, options)
		} else {
			r, err = git.CloneContext(ctx, a.cacher.NewStorer(repo), memfs.New(), options)
		}
		if err == nil {
			return r, nil
		}

		errmsg := err.Error()
		if isReferenceNotFoundErr(err) {
			errmsg = fmt.Sprintf("reference %s not found", resource.Ref.Name())
		}
		if errors.Is(err, transport.ErrRepositoryNotFound) {
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
	var refSpec string
	if ref == "HEAD" {
		refSpec = fmt.Sprintf("+%s:refs/remotes/origin/%[1]s", "HEAD")
	} else {
		refSpec = fmt.Sprintf("+%s:%[1]s", ref)
	}
	err = a.FetchRefSpec(ctx, r, repo, config.RefSpec(refSpec), FetchOpts{Depth: 1})
	if err == nil {
		return nil
	}

	return fmt.Errorf("Unable to find reference, tried - %s: %s", refSpec, err.Error())
}

type FetchOpts struct {
	Depth int
	Force bool
	Tags  OptTags
}

type OptTags int

const (
	FetchOptTagsDefault   OptTags = iota // Fetch the default tags for the operation.
	FetchOptTagsAll                      // Fetch all tags.
	FetchOptTagsFollowing                // Fetch any tag that points into the histories being fetched.
	FetchOptTagsNone                     // Don't fetch tags.
)

func (t OptTags) String() string {
	switch t {
	case FetchOptTagsDefault:
		return "default"
	case FetchOptTagsAll:
		return "all"
	case FetchOptTagsFollowing:
		return "following"
	case FetchOptTagsNone:
		return "none"
	default:
		return "-"
	}
}

func (t OptTags) TagMode(def git.TagMode) git.TagMode {
	switch t {
	case FetchOptTagsDefault:
		return def
	case FetchOptTagsAll:
		return git.AllTags
	case FetchOptTagsFollowing:
		return git.TagFollowing
	case FetchOptTagsNone:
		return git.NoTags
	default:
		panic(fmt.Errorf("invalid tag: %v", t))
	}
}

func (o FetchOpts) String() string {
	return fmt.Sprintf("{Depth:%v, Force:%v, Tags:%v}",
		o.Depth, o.Force, o.Tags)
}

// FetchRefSpec fetches a specific reference specification
func (a Git) FetchRefSpec(ctx context.Context, r *git.Repository, repo string, spec config.RefSpec, opts FetchOpts) (err error) {
	log.Debugf("fetching ref spec: %v with opts: %v", spec, opts)
	var tried []string

	logWriter := log.StandardLogger().Writer()
	defer func() { _ = logWriter.Close() }()

	tags := opts.Tags.TagMode(git.AllTags)

	for _, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options := &git.FetchOptions{
			Depth:     opts.Depth,
			Force:     opts.Force,
			Progress:  logWriter,
			Auth:      auth,
			RemoteURL: url,
			RefSpecs:  []config.RefSpec{spec},
			Tags:      tags,
		}
		log.Debugf("fetching ref spec context with auth method: %v", meth.Name())
		err = r.FetchContext(ctx, options)
		if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
			log.Debugf("ref spec: %v fetched with auth method: %v", spec, meth.Name())
			return nil
		}

		errmsg := err.Error()
		if isReferenceNotFoundErr(err) {
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
	base_options := git.FetchOptions{
		Depth:    opts.Depth,
		RefSpecs: []config.RefSpec{config.RefSpec(refSpec)},
	}

	tried := []string{}
	for i, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		// Note that some default values are set based on auth during the fetch, start again from a clean base
		options := base_options
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

		err = r.FetchContext(ctx, &options)
		if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
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
		if errors.Is(err, plumbing.ErrObjectNotFound) {
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

func (o checkoutOpts) String() string {
	return fmt.Sprintf("{force:%v}", o.force)
}

// Checkout the repository at the reference of the given retriever.
func (a Git) checkout(r *git.Repository, resource *retriever.Resource, opts checkoutOpts) error {
	log.Debugf("checking out repository to resource: %v with opts: %v", resource, opts)
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
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				return fmt.Errorf("reference %s not found", rev)
			}
			return
		}
	}

	hash, err := retriever.NewHash(h.String())
	if err != nil {
		return
	}
	err = resource.Ref.SetHash(hash)
	return
}

// TryResolveAsTag tries to resolve a SymbolicReference as a Tag Reference.
func (a Git) TryResolveAsTag(r *git.Repository, resource *retriever.Resource) bool {
	if resource.Ref == nil {
		return false
	}

	if resource.Ref.IsHash() {
		return false
	}

	var h *plumbing.Hash
	if resource.Ref.IsHEAD() {
		return false
	}

	rev := resource.Ref.Name()
	if strings.HasPrefix(rev, "refs/") {
		if !strings.HasPrefix(rev, "refs/tags/") {
			return false
		}

		rev = rev[10:]
	}

	rev = "refs/tags/" + rev
	h, err := r.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return false
	}

	hash, err := retriever.NewHash(h.String())
	if err != nil {
		return false
	}
	err = resource.Ref.SetHash(hash)

	return true
}

// Session provides a mechanism to ensure that repeat requests to set the content of a repository to a given
// reference are guaranteed to be stable. For example, if an initial request is made to set a repository to contents of
// the remote 'main' branch, then subsequent requests within the same session to set the repository to the remote 'main'
// branch are guaranteed to set the repository to the same state, even if the 'main' branch on the remote repository has
// changed since it was first retrieved.
//
// The following reference types are supported:
// 1. Branches:         e.g. main
// 2. Hashes:      		e.g. 1e7c4cecaaa8f76e3c668cebc411f1b03171501f
// 3. Short hashes:     e.g. 1e7c4cec
// 4. Tags:         	e.g. v0.0.1
// 5. Prefixed tags:    e.g. tags/v0.0.1 [legacy behaviour]
type Session interface {
	// Set the repository to the given reference, resetting as necessary.
	Set(ctx context.Context, repo string, ref string, opts SessionSetOpts) error

	// Resolve the commit of the given reference within the repository.
	Resolve(ctx context.Context, repo string, ref string, opts SessionResolveOpts) (*object.Commit, error)
}

// SessionSetOpts provide configuration to the Session.Set method.
type SessionSetOpts struct {

	// How to fetch (or not) content from remote repositories.
	Fetch SessionOptFetch

	// How to reset (or not) the state of repositories.
	Reset SessionOptReset

	// The depth at which content should be fetched.
	Depth int

	// True to verify the repository is already at the requested reference (returning an error if it's not).
	Verify bool

	// Whether verbose (i.e. debug level) logs should be written when interacting with the session.
	Verbose bool
}

// SessionResolveOpts provide configuration to the Session.Resolve method.
type SessionResolveOpts struct {

	// How to fetch (or not) content from remote repositories.
	Fetch SessionOptFetch

	// The depth at which content should be fetched.
	Depth int

	// Whether verbose (i.e. debug level) logs should be written when interacting with the session.
	Verbose bool
}

// SessionOptFetch describes how to fetch from remote repositories.
type SessionOptFetch int

const (
	SessionOptFetchFirst   SessionOptFetch = iota // Fetch remote content for a reference if it is the first time the reference is set during the session, otherwise don't fetch.
	SessionOptFetchUnknown                        // Fetch remote reference if the reference is unknown to the local repository.
	SessionOptFetchTrue                           // Fetch remote content.
	SessionOptFetchFalse                          // Don't fetch remote content.
)

func (f SessionOptFetch) String() string {
	switch f {
	case SessionOptFetchFirst:
		return "first"
	case SessionOptFetchUnknown:
		return "unknown"
	case SessionOptFetchTrue:
		return "true"
	case SessionOptFetchFalse:
		return "false"
	default:
		return "-"
	}
}

// SessionOptReset describes how to reset the state of repositories.
type SessionOptReset int

const (
	SessionOptResetFirst      SessionOptReset = iota // Reset the repository if it is the first time it is set during the session, otherwise reset on checkout.
	SessionOptResetOnCheckout                        // Reset the repository if it is being checked out to a different resource.
	SessionOptResetTrue                              // Reset the repository.
	SessionOptResetFalse                             // Don't reset the repository.
)

func (f SessionOptReset) String() string {
	switch f {
	case SessionOptResetFirst:
		return "first"
	case SessionOptResetOnCheckout:
		return "on-checkout"
	case SessionOptResetTrue:
		return "true"
	case SessionOptResetFalse:
		return "false"
	default:
		return "-"
	}
}

type sessionImpl struct {
	once   once.Once
	hashes map[string]string // The mapping of repo@ref to known hashes
	g      *Git
}

func NewSession(g *Git) Session {
	return sessionImpl{
		once:   once.NewOnce(),
		hashes: make(map[string]string),
		g:      g}
}

func (s sessionImpl) Set(ctx context.Context, repo string, ref string, opts SessionSetOpts) error {
	_, err := s.set(ctx, repo, ref, opts.Fetch, opts.Reset, OptCheckoutTrue, opts.Depth, opts.Verify, opts.Verbose)
	return err
}

func (s sessionImpl) Resolve(ctx context.Context, repo string, ref string, opts SessionResolveOpts) (*object.Commit, error) {
	result, err := s.set(ctx, repo, ref, opts.Fetch, SessionOptResetFalse, OptCheckoutFalse, opts.Depth, false, opts.Verbose)
	if err != nil {
		return nil, err
	}
	return result.Commit, nil
}

func (s sessionImpl) set(ctx context.Context, repo string, ref string,
	optFetch SessionOptFetch, optReset SessionOptReset,
	optCheckout OptCheckout, optDepth int, optVerify bool, optVerbose bool) (*SetResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		key := repo + "@" + ref
		ch := s.once.Register(key)
		defer s.once.Unregister(key)
		if ch != nil {
			<-ch
		}

		// Maintain legacy behaviour.
		ref = strings.TrimPrefix(ref, "tags/")

		if optVerbose {
			level := log.GetLevel()
			log.SetLevel(log.DebugLevel)
			defer func() { log.SetLevel(level) }()
		}

		// Cache whether this is the first request for the session.
		first := len(s.hashes) == 0

		// Cache the known session reference hash.
		sessionRefHash, hasSessionRefHash := s.hashes[key]

		// Use the session hash if known
		if hasSessionRefHash && ref != sessionRefHash {
			log.Debugf("conforming reference: %v to hash: %v within current session", ref, sessionRefHash)
			ref = sessionRefHash
		}

		// Cache the fetch behaviour.
		var fetch OptFetch
		switch optFetch {
		case SessionOptFetchFirst:
			fetch = OptFetchTrue
			if hasSessionRefHash { // Don't fetch, it's not the first time the reference has been set within the session.
				fetch = OptFetchFalse
			}
		case SessionOptFetchUnknown:
			fetch = OptFetchUnknown
		case SessionOptFetchFalse:
			fetch = OptFetchFalse
		case SessionOptFetchTrue:
			fetch = OptFetchTrue
		default:
			return nil, fmt.Errorf("invalid fetch option: %v", optFetch)
		}

		// Cache the reset behaviour.
		var reset OptReset
		switch optReset {
		case SessionOptResetFirst:
			reset = OptResetTrue
			if !first { // Reset on checkout, it's not the first time a reference has been set within the session.
				reset = OptResetOnCheckout
			}
		case SessionOptResetOnCheckout:
			reset = OptResetOnCheckout
		case SessionOptResetFalse:
			reset = OptResetFalse
		case SessionOptResetTrue:
			reset = OptResetTrue
		default:
			return nil, fmt.Errorf("invalid reset option: %v", optReset)
		}

		// Set the reference.
		result, err := s.g.Set(ctx, repo, ref, SetOpts{
			Fetch:    fetch,
			Reset:    reset,
			Depth:    optDepth,
			Verify:   optVerify,
			Checkout: optCheckout})
		if err != nil {
			return nil, err
		}
		s.hashes[key] = result.Commit.Hash.String()
		return result, nil
	}
}
