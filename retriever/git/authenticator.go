package git

import (
	"fmt"
	"net/url"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Authenticator is a generic authentication method to access git repositories.
type Authenticator interface {
	Name() string
	AuthMethod(string) (transport.AuthMethod, string)
}

// None implements the Authenticator interface. It is typically used to fetch public repositories.
type None struct{}

// Name returns the name of the auth method.
func (None) Name() string { return "None" }

// AuthMethod returns the AuthMethod and corresponding git repository URL.
func (None) AuthMethod(repo string) (transport.AuthMethod, string) {
	return nil, HTTPSURL(repo)
}

// SSHAgent implements the Authenticator interface.
type SSHAgent struct {
	authMethod transport.AuthMethod
}

// NewSSHAgent returns a new SSHAgent.
// It opens a pipe with the SSH agent and uses the pipe as the implementer of the public key callback function.
func NewSSHAgent() (*SSHAgent, error) {
	auth, err := ssh.NewSSHAgentAuth("git")
	if err != nil {
		return nil, fmt.Errorf("set up ssh-agent auth failed %s\n", err.Error())
	}
	return &SSHAgent{auth}, nil
}

// Name returns the name of the auth method.
func (SSHAgent) Name() string { return "ssh-agent" }

// AuthMethod returns the AuthMethod and corresponding git repository URL.
func (a SSHAgent) AuthMethod(repo string) (transport.AuthMethod, string) {
	return a.authMethod, SSHURL(repo)
}

// SSHKeyAuth implements the Authenticator interface.
type SSHKeyAuth struct {
	authMethods map[string]transport.AuthMethod
}

// NewSSHKeyAuth returns a new SSHKeyAuth with given SSH keys and passwords.
func NewSSHKeyAuth(sshkeys map[string]SSHKey) (*SSHKeyAuth, error) {
	methods := make(map[string]transport.AuthMethod, len(sshkeys))

	for host, key := range sshkeys {
		publicKeys, err := ssh.NewPublicKeysFromFile("git", key.PrivateKey, key.PrivateKeyPassword)
		if err != nil {
			return nil, err
		}

		methods[host] = publicKeys
	}

	return &SSHKeyAuth{methods}, nil
}

// Name returns the name of the auth method.
func (SSHKeyAuth) Name() string { return "Personal SSH key" }

// AuthMethod returns the AuthMethod and corresponding git repository URL.
func (a SSHKeyAuth) AuthMethod(repo string) (transport.AuthMethod, string) {
	u, err := url.Parse("https://" + repo)
	if err != nil {
		return nil, ""
	}
	return a.authMethods[u.Host], SSHURL(repo)
}

// BasicAuth implements the Authenticator interface.
// It stores pairs of usernames and passwords(tokens) for accessing different hosts.
type BasicAuth struct {
	authMethods map[string]transport.AuthMethod
}

// NewBasicAuth returns a new BasicAuth with given usernames and passwords.
func NewBasicAuth(credentials map[string]Credential) *BasicAuth {
	methods := make(map[string]transport.AuthMethod, len(credentials))
	for host, credential := range credentials {
		methods[host] = &http.BasicAuth{
			Username: credential.Username,
			Password: credential.Password,
		}
	}
	return &BasicAuth{methods}
}

// Name returns the name of the auth method.
func (BasicAuth) Name() string { return "Username and Password/Token" }

// AuthMethod returns the AuthMethod and corresponding git repository URL.
func (a BasicAuth) AuthMethod(repo string) (transport.AuthMethod, string) {
	u, err := url.Parse("https://" + repo)
	if err != nil {
		return nil, ""
	}
	return a.authMethods[u.Host], HTTPSURL(repo)
}

// Credential represents a pair of username and password(token).
type Credential struct {
	Username string
	Password string
}

// SSHKey represents a pair of SSH private key and key password.
type SSHKey struct {
	PrivateKey         string
	PrivateKeyPassword string
}

// HTTPSURL returns the HTTPS URL of given repository path.
func HTTPSURL(repo string) string {
	return "https://" + repo + ".git"
}

// SSHURL returns the SSH URL of given repository path.
func SSHURL(repo string) string {
	return "ssh://" + repo + ".git"
}
