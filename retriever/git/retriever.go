package git

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/anz-bank/golden-retriever/once"
	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/go-git/go-git/v5"
	log "github.com/sirupsen/logrus"
)

// Git implements the Retriever interface.
type Git struct {
	authMethods []Authenticator
	cacher      Cacher
	once        once.Once

	noForcedFetch bool
	fetchedRefs   *sync.Map
}

// New returns new Git with given authentication parameters. Cache repositories in memory by default.
func New(options *AuthOptions) *Git {
	return NewWithOptions(&NewGitOptions{options, NewMemcache(), false})
}

// NewWithCache returns new Git with given authentication parameters and git cacher.
func NewWithCache(options *AuthOptions, cacher Cacher) *Git {
	return NewWithOptions(&NewGitOptions{options, cacher, false})
}

type NewGitOptions struct {
	AuthOptions   *AuthOptions
	Cacher        Cacher
	NoForcedFetch bool
}

// NewWithOptions returns new Git with given options.
func NewWithOptions(options *NewGitOptions) *Git {
	methods := make([]Authenticator, 0, 2)

	if sshagent, err := NewSSHAgent(); err == nil {
		methods = append(methods, sshagent)
	} else {
		log.Debugf("New SSH Agent error: %s", err.Error())
	}

	if options.AuthOptions != nil {
		if len(options.AuthOptions.SSHKeys) > 0 {
			if m, err := NewSSHKeyAuth(options.AuthOptions.SSHKeys); err != nil {
				log.Debugf("Set up SSH key error: %s", err.Error())
			} else {
				methods = append(methods, m)
			}
		}

		if len(options.AuthOptions.Credentials) > 0 {
			methods = append(methods, NewBasicAuth(options.AuthOptions.Credentials))
		}

		if len(options.AuthOptions.Tokens) > 0 {
			creds := make(map[string]Credential, len(options.AuthOptions.Tokens))
			for host, token := range options.AuthOptions.Tokens {
				creds[host] = Credential{
					Username: "modv2",
					Password: token,
				}
			}
			methods = append(methods, NewBasicAuth(creds))
		}
	}

	methods = append(methods, None{})

	if options.AuthOptions != nil && options.AuthOptions.Local {
		methods = append(methods, Local{})
	}

	return &Git{
		authMethods: methods,
		cacher:      options.Cacher,
		once:        once.NewOnce(),

		noForcedFetch: options.NoForcedFetch,
		fetchedRefs:   &sync.Map{},
	}
}

// AuthOptions describes which authentication methods are available.
type AuthOptions struct {
	// Credentials is a key-value pairs of <host>, <username+password>, e.g. { "github.com": {"username": "abcdef", "password": "123456"} }
	Credentials map[string]Credential
	// Tokens is a key-value pairs of <host>, <personal access token>, e.g. { "github.com": "qwerty" }
	Tokens map[string]string
	// SSHKeys is a key-value pairs of <host>, <private key + key password>, e.g. { "github.com": {"private_key": "~/.ssh/id_rsa_github", "private_key_password": ""} }
	SSHKeys map[string]SSHKey
	// True if authentication to a local repository should be included in the available methods.
	Local bool
}

type SetOpts struct {
	Fetch    OptFetch    // How to fetch (or not) content from remote repositories.
	Reset    OptReset    // How to reset (or not) the state of repositories.
	Checkout OptCheckout // How to check out (or not) the state of repositories.
	Depth    int         // The depth at which to fetch remote content (if required).
	Verify   bool        // True to verify the repository is already at the requested reference (returning an error if it's not).
}

// OptFetch describes how to fetch content from remote repositories.
type OptFetch int

const (
	OptFetchTrue    OptFetch = iota // Fetch remote content.
	OptFetchUnknown                 // Fetch remote content if the reference is unknown to the local repository.
	OptFetchFalse                   // Don't fetch remote content.
)

func (f OptFetch) String() string {
	switch f {
	case OptFetchTrue:
		return "true"
	case OptFetchUnknown:
		return "unknown"
	case OptFetchFalse:
		return "false"
	default:
		return "-"
	}
}

// OptReset describes how to reset the state of repositories.
type OptReset int

const (
	OptResetTrue       OptReset = iota // Reset the repository (even if it's already at the requested reference).
	OptResetOnCheckout                 // Reset the repository if it is being checked out to a different reference.
	OptResetFalse                      // Don't reset the repository (but still attempt a checkout without resetting if required).
)

func (f OptReset) String() string {
	switch f {
	case OptResetTrue:
		return "true"
	case OptResetOnCheckout:
		return "on-checkout"
	case OptResetFalse:
		return "false"
	default:
		return "-"
	}
}

// OptCheckout describes how to check out the state of repositories.
type OptCheckout int

const (
	OptCheckoutTrue  OptCheckout = iota // Check out the repository.
	OptCheckoutFalse                    // Don't check out the repository.
)

func (f OptCheckout) String() string {
	switch f {
	case OptCheckoutTrue:
		return "true"
	case OptCheckoutFalse:
		return "false"
	default:
		return "-"
	}
}

func (o SetOpts) String() string {
	return fmt.Sprintf("{Fetch:%v, Reset:%v, Depth:%v}",
		o.Fetch, o.Reset, o.Depth)
}

type SetResult struct {
	Commit *object.Commit // The commit that the repository was set to.
}

// Set the repository to the given reference, resetting as necessary.
//
// This method behaves in the following manner:
// 1. If the repository doesn't exist, it is cloned and set to the requested reference.
// 2. If fetching is requested, the remote repository is fetched (at the requested depth) before the reference is resolved.
// 3. If resetting is not requested, and the repository is already at the requested reference, no action is performed.
// 4. Otherwise, the repository is cleaned and reset to the requested reference.
//
// The one caveat to the above description is if checking out is not requested. In this case, fetching is still
// performed, but checking out (and hence any requests to reset the repository) are ignored.
//
// The following reference types are supported:
// 1. Branches:         e.g. main
// 2. Hashes:      		e.g. 1e7c4cecaaa8f76e3c668cebc411f1b03171501f
// 3. Short hashes:     e.g. 1e7c4cec
// 4. Tags:         	e.g. v0.0.1
// 5. Prefixed tags:    e.g. tags/v0.0.1 [legacy behaviour]
func (a Git) Set(ctx context.Context, repo, ref string, opts SetOpts) (*SetResult, error) {
	log.Debugf("setting repo: %v to reference: %v with opts: %v", repo, ref, opts)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		ch := a.once.Register(repo)
		defer a.once.Unregister(repo)
		if ch != nil {
			<-ch
		}

		// Cache the git repository object.
		rr, ok := a.cacher.Get(repo)

		// Cache a function to return the set result at the given hash.
		resultAt := func(r *Repo, hash string) (*SetResult, error) {
			commit, err := r.r.CommitObject(plumbing.NewHash(hash))
			if err != nil {
				return nil, err
			}
			return &SetResult{Commit: commit}, nil
		}

		// Handle the case where the repository has not yet been initialised.
		if !ok {
			if opts.Fetch == OptFetchFalse {
				return nil, fmt.Errorf("repository: %v doesn't exist and fetch was explicitly false", repo)
			}
			if opts.Verify {
				return nil, fmt.Errorf("repository: %v was asked to be verified at reference: %v but doesn't exist", repo, ref)
			}

			// Initialise the repository either with a clone (if requested) or an init and fetch of the head reference.
			// If we haven't been requested to check out the repository, then initialising it with the remote repository
			// is sufficient. We fetch the head reference because all future interactions with the repository assume
			// that the head reference is always known.
			init := func() (*Repo, error) {
				return a.CloneRepo(ctx, repo, CloneOpts{
					Depth: opts.Depth,
					Tags:  FetchOptTagsNone,
				})
			}
			if opts.Checkout != OptCheckoutTrue {
				init = func() (*Repo, error) {
					r, err := a.InitWithRemote(ctx, repo)
					if err != nil {
						return nil, err
					}
					err = r.FetchRef(ctx, "HEAD", FetchOpts{
						Depth: max(1, opts.Depth), // workaround: ref not updated if fetched with zero depth
						Force: true,
						Tags:  FetchOptTagsNone,
					})
					return r, err
				}
			}
			r, err := init()
			if err != nil {
				return nil, fmt.Errorf("error cloning repository to reference: %v: %w", ref, err)
			}

			// Fetch the requested reference if it's not known within the repository.
			exists, err := r.Exists(ref)
			if err != nil {
				return nil, fmt.Errorf("error checking existence of reference: %v: %w", ref, err)
			}
			if !exists {
				err := r.FetchRefOrAll(ctx, ref, FetchOpts{
					Depth: max(1, opts.Depth), // workaround: ref not updated if fetched with zero depth
					Force: true,
					Tags:  FetchOptTagsNone,
				})
				if err != nil {
					return nil, fmt.Errorf("error fetching reference: %v: %w", ref, err)
				}
			}

			// Resolve the hash of the reference if we don't need to check out the repository.
			if opts.Checkout != OptCheckoutTrue {
				refHash, err := r.ResolveHash(ref)
				if err != nil {
					return nil, fmt.Errorf("error resolving hash for reference: %v: %w", ref, err)
				}
				log.Debugf("checkout ignored, reference: %v resolved within repo: %v and no checkout requested", ref, r)
				return resultAt(r, refHash)
			}

			// Checkout the repository.
			err = r.Checkout(ref, CheckoutOpts{
				Force: true,
			})
			if err != nil {
				return nil, fmt.Errorf("error checking out reference: %v: %w", ref, err)
			}
			headHash, err := r.ResolveHash("HEAD")
			if err != nil {
				return nil, fmt.Errorf("error resolving head hash: %w", err)
			}
			return resultAt(r, headHash)
		}

		// Cache the repository object.
		r := &Repo{g: &a, r: rr, repo: repo}

		// Cache the current repository hash.
		headHash, err := r.ResolveHash("HEAD")
		if err != nil {
			return nil, fmt.Errorf("error resolving head hash: %w", err)
		}

		// Cache the hash of the target reference (if known).
		refHash := ""
		exists, err := r.Exists(ref)
		if err != nil {
			return nil, fmt.Errorf("error resolving existence of reference: %v: %w", ref, err)
		}
		if exists {
			refHash, err = r.ResolveHash(ref)
			if err != nil {
				return nil, fmt.Errorf("error resolving hash for reference: %v: %w", ref, err)
			}
		}

		// Cache whether to fetch from the remote repository.
		fetch := false
		switch opts.Fetch {
		case OptFetchFalse: // no-op: fetch = false
		case OptFetchTrue:
			if strings.HasPrefix(refHash, ref) {
				log.Debugf("ignoring fetch, requested hash reference: %v already known", ref)
			} else {
				fetch = true
			}
		case OptFetchUnknown:
			fetch = !exists
		}

		// Fetch from the remote repository if required.
		if fetch {
			err := r.FetchRefOrAll(ctx, ref, FetchOpts{
				Depth: max(1, opts.Depth), // workaround: ref not updated if fetched with zero depth
				Force: true,
				Tags:  FetchOptTagsNone,
			})
			if err != nil {
				return nil, fmt.Errorf("error fetching reference: %v in repo: %v: %w", ref, r, err)
			}
		}

		// Update the reference hash (which may have changed after the fetch).
		refHash, err = r.ResolveHash(ref)
		if err != nil {
			return nil, fmt.Errorf("error resolving hash for reference: %v: %w", ref, err)
		}

		// Handle the case where verification was requested.
		if opts.Verify {
			if headHash != refHash {
				return nil, fmt.Errorf("repository: %v was asked to be verified at reference: %v[%v] but was at: %v", repo, ref, refHash, headHash)
			}
			if opts.Reset != OptResetTrue {
				log.Debugf("taking no action, repo: %v verified to be at reference: %v and reset not requested", r, ref)
				return resultAt(r, headHash)
			}
			clean, err := r.IsClean()
			if err != nil {
				return nil, fmt.Errorf("error checking clean status: %w", err)
			}
			if clean {
				log.Debugf("taking no action, repo: %v verified to be at reference: %v and reset not required because repository is clean", r, ref)
				return resultAt(r, headHash)
			} else {
				return nil, fmt.Errorf("repository: %v verified to be at reference: %v but requested reset would modify contents", repo, ref)
			}
		}

		// Return if we're already at the requested reference, and resetting isn't requested.
		//
		// Note: An issue can arise when a repository is initially set with the no-checkout option, resulting in a
		// clone but no checkout. In this state, Git reports the current reference to be the head of the repository,
		// even though no checkout has been was performed. If the repository is then requested to be set, we can falsely
		// return early if we assume the content has been set based on its current reference. To avoid this scenario, we
		// only return early if the repository is not empty, indicating that either:
		// 1. The repository has been checked out previously (most likely), or
		// 2. The repository is actually empty, or
		// 3. The repository has been modified outside of this process.
		// In any of these scenarios, avoiding an early return does not affect the correctness of the end result.
		if headHash == refHash && (opts.Reset != OptResetTrue) {
			c, plain := a.cacher.(PlainFsCache)
			if !plain {
				return nil, fmt.Errorf("repository must be a plain repository")
			}
			dir := c.RepoDir(repo)
			files, err := os.ReadDir(dir)
			if err != nil {
				return nil, fmt.Errorf("error reading directory: %v for repository: %v", dir, repo)
			}
			if len(files) > 1 { // .git
				log.Debugf("taking no action, repo: %v already set to requested reference: %v and reset not requested", r, ref)
				return resultAt(r, headHash)
			} else {
				log.Debugf("ignoring equivalent references: %v against repo: %v, repository empty", ref, r)
			}
		}

		// Return if checking out is requested not to be done.
		if opts.Checkout != OptCheckoutTrue {
			log.Debugf("checkout ignored, reference: %v resolved within repo: %v and no checkout requested", ref, r)
			return resultAt(r, refHash)
		}

		// Checkout the repository to the requested reference, resetting as necessary.
		err = r.Checkout(ref, CheckoutOpts{
			Force: opts.Reset != OptResetFalse,
		})
		if err != nil {
			return nil, fmt.Errorf("error checking out reference: %v: %w", ref, err)
		}
		return resultAt(r, refHash)
	}
}

func keyFromResource(resource *retriever.Resource) string {
	return resource.Repo + ":" + resource.Ref.Name()
}

func (a Git) setFetched(r *git.Repository, resource *retriever.Resource) {
	a.fetchedRefs.Store(keyFromResource(resource), true)
	if resource.Ref.IsHEAD() {
		_ = a.ResolveReference(r, resource)
		a.fetchedRefs.Store(keyFromResource(resource), true)
	}
}

func (a Git) isFetched(resource *retriever.Resource) bool {
	_, found := a.fetchedRefs.Load(keyFromResource(resource))

	return found
}

// Retrieve remote file in format of <repo>/<filepath>@<ref>, e.g. github.com/org/foo/bar.json@v0.1.0
// Return the latest content of the file in default branch if no ref specified
func (a Git) Retrieve(ctx context.Context, resource *retriever.Resource) (c []byte, err error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		ch := a.once.Register(resource.Repo)
		defer a.once.Unregister(resource.Repo)
		if ch != nil {
			<-ch

			// Don't just continue otherwise you could get multiple threads continuing at the same time
			// Try again, checking for ctx.Done() as well
			return a.Retrieve(ctx, resource)
		}

		r, ok := a.cacher.Get(resource.Repo)
		if !ok {
			start := time.Now()
			log.Debugf(" ===> clone: %s@%s\n", resource.Repo, resource.Ref.Name())
			// Can't pass {SingleBranch: !resource.Ref.IsHEAD()} because the ref could be a tag
			r, err = a.CloneWithOpts(ctx, resource, CloneOpts{Depth: 1, NoCheckout: true})
			log.Debugf(" <=== clone (%s) complete in %s\n", resource.Repo, time.Since(start))
			if err != nil {
				return nil, fmt.Errorf("git clone: %s", err.Error())
			}
			a.setFetched(r, resource)
		} else {
			if a.noForcedFetch {
				c, err = a.Show(r, resource)
				if err == nil {
					return c, nil
				}
			}

			if resource.Ref.IsHEAD() {
				// Resolve HEAD branch but don't keep the current hash
				_ = a.ResolveReference(r, resource)
				resource.Ref = retriever.NewBranchReference(resource.Ref.Name())
			}

			// Check if it's a tag, we assume tags don't change so don't need to refetch
			if a.TryResolveAsTag(r, resource) {
				a.setFetched(r, resource)
			} else if !a.isFetched(resource) {
				start := time.Now()
				log.Debugf(" ===> fetching: %s@%s\n", resource.Repo, resource.Ref.Name())
				err = a.Fetch(ctx, r, resource)
				log.Debugf(" <=== fetching (%s) complete in %s\n", resource.Repo, time.Since(start))
				if err != nil {
					return nil, fmt.Errorf("git fetch: %s", err.Error())
				}

				a.setFetched(r, resource)
			}
		}

		c, err = a.Show(r, resource)
		if err != nil {
			return nil, fmt.Errorf("git show: %s", err.Error())
		}
		return c, nil
	}
}
