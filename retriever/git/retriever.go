package git

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anz-bank/golden-retriever/once"
	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
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
	Fetch bool
	Force bool
	Depth int
}

func (o SetOpts) String() string {
	return fmt.Sprintf("{Fetch:%v, Force:%v, Depth:%v}",
		o.Fetch, o.Force, o.Depth)
}

// Set the repository to the given resource reference.
func (a Git) Set(ctx context.Context, resource *retriever.Resource, opts SetOpts) (err error) {
	log.Debugf("setting repository to resource: %v with opts: %v", resource, opts)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		ch := a.once.Register(resource.Repo)
		defer a.once.Unregister(resource.Repo)
		if ch != nil {
			<-ch
		}

		r, ok := a.cacher.Get(resource.Repo)

		// Cache the reference spec to retrieve if necessary
		spec := originRefSpec(resource, true)

		// Cache whether the 'single branch' option should be set when fetching.
		singleBranch := resource.Ref.Type() == retriever.ReferenceTypeBranch

		// Cache whether the 'no tags' option should be set when fetching.
		noTags := resource.Ref.Type() != retriever.ReferenceTypeTag &&
			resource.Ref.Type() != retriever.ReferenceTypeSymbolic

		// Cache the options with which to fetch from the remote repository
		fetchOpts := FetchOpts{
			Depth:  opts.Depth,
			Force:  true,
			NoTags: noTags,
		}

		// Clone and checkout the repository if it doesn't exist
		if !ok {
			// An issue exists whereby attempting to clone a hash reference results in
			// failure. To work around this issue, we clone the repository at its head,
			// then fetch the reference specifically.
			if resource.Ref.IsHash() {
				cloneResource := resource
				cloneResource = &retriever.Resource{
					Repo: resource.Repo,
					Ref:  retriever.HEADReference(),
				}
				r, err = a.CloneWithOpts(ctx, cloneResource, CloneOpts{
					Depth:        opts.Depth,
					SingleBranch: true,
					NoTags:       true})
				if err != nil {
					return fmt.Errorf("git clone: %s", err.Error())
				}
				err := a.FetchRefSpec(ctx, r, resource.Repo, spec, fetchOpts)
				if err != nil {
					return fmt.Errorf("git fetch: %s", err.Error())
				}
			} else {
				r, err = a.CloneWithOpts(ctx, resource, CloneOpts{
					Depth:        opts.Depth,
					SingleBranch: singleBranch,
					NoTags:       noTags,
				})
				if err != nil {
					return fmt.Errorf("git clone: %s", err.Error())
				}
			}
			return a.checkout(r, resource, checkoutOpts{force: opts.Force})
		}

		// Either fetch the latest content for the resource if requested, or
		// fetch the latest content if the resource cannot be resolved
		if opts.Fetch {
			err := a.FetchRefSpec(ctx, r, resource.Repo, spec, fetchOpts)
			if err != nil {
				return fmt.Errorf("git fetch: %s", err.Error())
			}
		} else {
			err := a.ResolveReference(r, resource)
			if err != nil {
				err = a.FetchRefSpec(ctx, r, resource.Repo, spec, fetchOpts)
				if err != nil {
					return fmt.Errorf("git fetch: %s", err.Error())
				}
			}
		}

		// Checkout the repository at the given reference
		return a.checkout(r, resource, checkoutOpts{force: opts.Force})
	}
}

// The RefSpec to use to update the given resource.
// The following resources are supported:
// 1. Branch name: e.g. main
// 2. Tag name: e.g. tags/t
// 3. Hash: e.g. 865e3e5c6fca0120285c3aa846fdb049f8f074e6
// The update flag indicates that git should update the reference even if it isn’t a fast-forward.
func originRefSpec(resource *retriever.Resource, update bool) config.RefSpec {
	var str = ""
	if resource.Ref.IsHash() {
		str = fmt.Sprintf("%s:refs/remotes/origin/%[1]s", resource.Ref.String())
	} else if strings.HasPrefix(resource.Ref.Name(), "tags/") {
		str = fmt.Sprintf("refs/%s:refs/remotes/origin/%[1]s", resource.Ref.String())
	} else {
		str = fmt.Sprintf("%v:%[1]s", resource.Ref.String())
	}
	if update {
		str = "+" + str
	}
	return config.RefSpec(str)
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
