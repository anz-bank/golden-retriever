package git

import (
	"context"
	"fmt"

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
