package git

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v5/config"
	"strings"

	"github.com/anz-bank/golden-retriever/once"
	"github.com/anz-bank/golden-retriever/retriever"

	log "github.com/sirupsen/logrus"
)

// Git implements the Retriever interface.
type Git struct {
	authMethods []Authenticator
	cacher      Cacher
	once        once.Once
}

// New returns new Git with given authencitation parameters. Cache repositories in memory by default.
func New(options *AuthOptions) *Git {
	return NewWithCache(options, NewMemcache())
}

// NewWithCache returns new Git with given authencitation parameters and git cacher.
func NewWithCache(options *AuthOptions, cacher Cacher) *Git {
	methods := make([]Authenticator, 1)
	methods[0] = None{}

	if sshagent, err := NewSSHAgent(); err == nil {
		methods = append(methods, sshagent)
	} else {
		log.Debugf("New SSH Agent error: %s", err.Error())
	}

	if options != nil {
		if len(options.SSHKeys) > 0 {
			if m, err := NewSSHKeyAuth(options.SSHKeys); err != nil {
				log.Debugf("Set up SSH key error: %s", err.Error())
			} else {
				methods = append(methods, m)
			}
		}

		if len(options.Credentials) > 0 {
			methods = append(methods, NewBasicAuth(options.Credentials))
		}

		if len(options.Tokens) > 0 {
			creds := make(map[string]Credential, len(options.Tokens))
			for host, token := range options.Tokens {
				creds[host] = Credential{
					Username: "modv2",
					Password: token,
				}
			}
			methods = append(methods, NewBasicAuth(creds))
		}
	}

	return &Git{
		authMethods: methods,
		cacher:      cacher,
		once:        once.NewOnce(),
	}
}

// AuthOptions describes which authenication methods are available.
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

// Set the repository to the given resource reference.
func (a Git) Set(ctx context.Context, resource *retriever.Resource, opts SetOpts) (err error) {
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

		// Cache the options with which to fetch from the remote repository
		fetchOpts := FetchOpts{Depth: opts.Depth, Force: true}

		// Clone and checkout the repository if it doesn't exist
		if !ok {
			// An issue exists whereby attempting to clone a hash reference results in
			// failure. To work around this issue, we clone the repository at its head,
			// then fetch the reference specifically.
			if resource.Ref.IsHash() {
				cloneResource := resource
				cloneResource = &retriever.Resource{
					Repo: resource.Repo,
					Ref:  retriever.NewSymbolicReference("HEAD"),
				}
				r, err = a.CloneWithOpts(ctx, cloneResource, CloneOpts{Depth: opts.Depth})
				if err != nil {
					return fmt.Errorf("git clone: %s", err.Error())
				}
				err := a.FetchRefSpec(ctx, r, resource.Repo, spec, fetchOpts)
				if err != nil {
					return fmt.Errorf("git fetch: %s", err.Error())
				}
			} else {
				r, err = a.CloneWithOpts(ctx, resource, CloneOpts{Depth: opts.Depth})
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
		// go-git does not attempt to fetch a named remote branch if that branch is already known locally.
		// For example:
		// refs/heads/main:refs/remotes/origin/main
		// To work around this we fetch everything.
		str = fmt.Sprintf("refs/*:refs/*")
	}
	if update {
		str = "+" + str
	}
	return config.RefSpec(str)
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
		}

		r, ok := a.cacher.Get(resource.Repo)
		if !ok {
			r, err = a.Clone(ctx, resource)
			if err != nil {
				return nil, fmt.Errorf("git clone: %s", err.Error())
			}
		} else {
			c, err = a.Show(r, resource)
			if err == nil {
				return c, nil
			}

			err = a.Fetch(ctx, r, resource)
			if err != nil {
				return nil, fmt.Errorf("git fetch: %s", err.Error())
			}
		}

		c, err = a.Show(r, resource)
		if err != nil {
			return nil, fmt.Errorf("git show: %s", err.Error())
		}
		return c, nil
	}
}
