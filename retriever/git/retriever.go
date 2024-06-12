package git

import (
	"context"
	"fmt"
	"sync"
	"time"

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
}

type SetOpts struct {
	Fetch SetOptFetch // How to fetch (or not) content from remote repositories.
	Reset SetOptReset // How to reset (or not) the state of repositories.
	Depth int
}

// SetOptFetch describes how the Session.Set fetches content from remote repositories.
type SetOptFetch int

const (
	SetOptFetchTrue    SetOptFetch = iota // Fetch remote content.
	SetOptFetchUnknown                    // Fetch remote content if the reference is unknown to the local repository.
	SetOptFetchFalse                      // Don't fetch remote content.
)

// SetOptReset describes how Git.Set resets the state of repositories.
type SetOptReset int

const (
	SetOptResetTrue       SetOptReset = iota // Reset the repository (even if it's already at the requested resource).
	SetOptResetOnCheckout                    // Reset the repository if it is being checked out to a different resource.
	SetOptResetFalse                         // Don't reset the repository (but still attempt a checkout without resetting if required).
)

func (o SetOpts) String() string {
	return fmt.Sprintf("{Fetch:%v, Reset:%v, Depth:%v}",
		o.Fetch, o.Reset, o.Depth)
}

type SetResult struct {
	Hash string // The hash that the repository was set to.
}

// Set the repository to the given resource reference, resetting as necessary.
//
// This method behaves in the following manner:
// 1. If the repository doesn't exist, it is cloned and set to the requested reference.
// 2. If fetching is requested, the remote repository is fetched (at the requested depth) before the reference is resolved.
// 3. If resetting is not requested, and the repository is already at the requested reference, no action is performed.
// 4. Otherwise, the repository is cleaned and reset to the requested reference.
//
// The following reference types are supported:
// 1. Branches:         e.g. main
// 2. Hashes:      		e.g. 1e7c4cecaaa8f76e3c668cebc411f1b03171501f
// 3. Tags:         	e.g. v0.0.1
// 4. Prefixed tags:    e.g. tags/v0.0.1 [legacy behaviour]
//
// The following reference types are not supported:
// 1. Short hashes:     e.g. 1e7c4cec
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

		// Clone and checkout the repository if it doesn't exist.
		if !ok {
			if opts.Fetch == SetOptFetchFalse {
				return nil, fmt.Errorf("repository: %v doesn't exist and fetch was explicitly false", repo)
			}
			r, err := a.CloneRepo(ctx, repo, CloneOpts{
				Depth: opts.Depth,
				Tags:  FetchOptTagsNone,
			})
			if err != nil {
				return nil, fmt.Errorf("error cloning repository to reference: %v: %w", ref, err)
			}
			exists, err := r.Exists(ref)
			if err != nil {
				return nil, fmt.Errorf("error checking existence of reference: %v: %w", ref, err)
			}
			if !exists {
				err := r.FetchRefOrAll(ctx, ref, FetchOpts{
					Depth: opts.Depth,
					Force: true,
					Tags:  FetchOptTagsNone,
				})
				if err != nil {
					return nil, fmt.Errorf("error fetching reference: %v: %w", ref, err)
				}
			}
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
			return &SetResult{Hash: headHash}, nil
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
		case SetOptFetchFalse:
		case SetOptFetchTrue:
			if headHash == refHash {
				log.Debugf("ignoring fetch, repository at requested hash: %v", refHash)
			} else {
				fetch = true
			}
		case SetOptFetchUnknown:
			fetch = !exists
		}

		if fetch {
			err := r.FetchRefOrAll(ctx, ref, FetchOpts{
				Depth: opts.Depth,
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

		// Return if we're already at the requested reference, and resetting isn't requested.
		if headHash == refHash && (opts.Reset != SetOptResetTrue) {
			log.Debugf("taking no action, repo: %v already set to requested reference: %v and reset not requested", r, ref)
			return &SetResult{Hash: headHash}, nil
		}

		// Checkout the repository to the requested reference, resetting as necessary.
		err = r.Checkout(ref, CheckoutOpts{
			Force: opts.Reset != SetOptResetFalse,
		})
		if err != nil {
			return nil, fmt.Errorf("error checking out reference: %v: %w", ref, err)
		}
		return &SetResult{Hash: refHash}, nil
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
