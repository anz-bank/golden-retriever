package git

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/stretchr/testify/require"
	"github.com/undefinedlabs/go-mpatch"
)

const (
	pubRepo            = "github.com/SyslBot/a-public-repo"
	pubRepoREADME      = pubRepo + "/README.md"
	pubRepoInitSHA     = "1e7c4cecaaa8f76e3c668cebc411f1b03171501f"
	pubRepoV1SHA       = "f948d44b0d97dbbe019949c8b574b5f246b25dc2"
	pubRepoV2SHA       = "6a27bac5e5c379649c5b4574845744957cd6c749"
	pubRepoDevSHA      = "865e3e5c6fca0120285c3aa846fdb049f8f074e6"
	pubRepoInitContent = "# a-public-repo\nA public repo for modules testing\n"
	pubRepoV1Content   = "# a-public-repo v0.0.1\nA public repo for modules testing\n"
	pubRepoV2Content   = "# a-public-repo v0.0.2\nA public repo for modules testing\n"
	pubRepoDevContent  = "# a-public-repo-dev\nA public repo for modules testing\n"
)

const (
	privRepo        = "github.com/SyslBot/a-private-repo"
	privRepoREADME  = privRepo + "/README.md"
	privRepoContent = "# a-private-repo\nA private repo for modules testing\n"
)

var (
	username = "SyslBot"
	password = os.Getenv("TEST_PRIV_REPO_TOKEN")
)

func TestGitRetrieveCloneWrongResource(t *testing.T) {
	patch, _ := mpatch.PatchMethod(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})

	defer patch.Unpatch()

	tests := []struct {
		resourceStr    string
		errMsgContains []string
	}{
		{pubRepo + "/README", []string{"git show: file not found"}},
		{pubRepo + "/wrong.md", []string{"git show: file not found"}},
		{pubRepo + "/README.md@nosuchbranch", []string{"git clone: Unable to authenticate, tried:", "- None: reference nosuchbranch not found"}},
		{pubRepo + "/README.md@v100.0.0", []string{"git clone: Unable to authenticate, tried:", "- None: reference v100.0.0 not found"}},
		{pubRepo + "/README.md@commitshanotfoundc668cebc411f1b03171501f", []string{"git clone: Unable to authenticate, tried:", "- None: reference commitshanotfoundc668cebc411f1b03171501f not found"}},
	}

	for _, tr := range tests {
		t.Run(tr.resourceStr, func(t *testing.T) {
			r := New(nil)
			content, err := r.Retrieve(context.Background(), ParseResource(t, tr.resourceStr))
			for _, msg := range tr.errMsgContains {
				require.Contains(t, err.Error(), msg)
			}
			require.Equal(t, "", string(content))
		})
	}
}

func TestGitRetrieveClonePublicRepo(t *testing.T) {
	patch, _ := mpatch.PatchMethod(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})
	defer patch.Unpatch()

	tests := []struct {
		resourceStr string
		content     string
	}{
		{pubRepoREADME, pubRepoV2Content},
		{pubRepoREADME + "@main", pubRepoV2Content},
		{pubRepoREADME + "@develop", pubRepoDevContent},
		{pubRepoREADME + "@v0.0.1", pubRepoV1Content},
		{pubRepoREADME + "@" + pubRepoInitSHA, pubRepoInitContent},
	}

	for _, tr := range tests {
		t.Run(tr.resourceStr, func(t *testing.T) {
			r := New(nil)
			content, err := r.Retrieve(context.Background(), ParseResource(t, tr.resourceStr))
			require.NoError(t, err)
			require.Equal(t, tr.content, string(content))
		})
	}
}

func TestGitRetrieveToFilesystem(t *testing.T) {
	tmpDir := "tmpdir"
	repodir := filepath.Join(tmpDir, pubRepo)
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(repodir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(repodir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()

	r := NewWithCache(nil, NewPlainFscache(tmpDir))

	resource := ParseResource(t, pubRepoREADME)
	c, err := r.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(c))
}

func TestGitRetrieveHEADThenTag(t *testing.T) {
	tmpDir := "tmpdir"
	repodir := filepath.Join(tmpDir, pubRepo)
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(repodir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(repodir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()

	r := NewWithCache(nil, NewPlainFscache(tmpDir))

	resource := ParseResource(t, pubRepoREADME+"@main")
	c, err := r.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(c))

	resourcev1 := ParseResource(t, pubRepoREADME+"@v0.0.1")
	c, err = r.Retrieve(context.Background(), resourcev1)
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(c))
}

func TestGitRetrieveCloneThenFetchRepo(t *testing.T) {
	tmpDir := "tmpdir"
	repodir := filepath.Join(tmpDir, pubRepo)
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(repodir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(repodir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()

	tests := []struct {
		resourceStr string
		content     string
	}{
		{pubRepoREADME + "@" + pubRepoInitSHA, pubRepoInitContent},
		{pubRepoREADME + "@v0.0.1", pubRepoV1Content},
		{pubRepoREADME + "@main", pubRepoV2Content},
		{pubRepoREADME + "@develop", pubRepoDevContent},
	}

	r := NewWithCache(nil, NewPlainFscache(tmpDir))
	for _, tr := range tests {
		t.Run(tr.resourceStr, func(t *testing.T) {
			content, err := r.Retrieve(context.Background(), ParseResource(t, tr.resourceStr))
			require.NoError(t, err)
			require.Equal(t, tr.content, string(content))
		})
	}
}

func TestGitRetrievePrivateRepoAuthNone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	patch, _ := mpatch.PatchMethod(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})

	defer patch.Unpatch()

	noneGit := New(nil)
	c, err := noneGit.Retrieve(context.Background(), ParseResource(t, privRepoREADME))
	errMsg := err.Error()
	require.Contains(t, errMsg, "git clone: Unable to authenticate, tried:")
	require.Contains(t, errMsg, "- None: authentication required")
	require.Equal(t, "", string(c))
}

func TestGitRetrievePrivateRepoAuthToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	patch, _ := mpatch.PatchMethod(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})

	defer patch.Unpatch()

	tokenGit := New(&AuthOptions{Tokens: map[string]string{"github.com": password}})
	c, err := tokenGit.Retrieve(context.Background(), ParseResource(t, privRepoREADME))
	require.NoError(t, err)
	require.Equal(t, privRepoContent, string(c))

	wrongTokenGit := New(&AuthOptions{Tokens: map[string]string{"github.com": "foobar"}})
	c, err = wrongTokenGit.Retrieve(context.Background(), ParseResource(t, privRepoREADME))
	errMsg := err.Error()
	require.Contains(t, errMsg, "git clone: Unable to authenticate, tried:")
	require.Contains(t, errMsg, "- None: authentication required")
	require.Contains(t, errMsg, "- Username and Password/Token: authentication required")
	require.Equal(t, "", string(c))
}

func TestGitRetrievePrivateRepoAuthPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	patch, _ := mpatch.PatchMethod(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})

	defer patch.Unpatch()

	pwGit := New(&AuthOptions{Credentials: map[string]Credential{"github.com": Credential{username, password}}})
	c, err := pwGit.Retrieve(context.Background(), ParseResource(t, privRepoREADME))
	require.NoError(t, err)
	require.Equal(t, privRepoContent, string(c))
}

func BenchmarkGitRetrieveHash(b *testing.B) {
	public := pubRepoREADME + "@" + pubRepoInitSHA
	resource := ParseResource(b, public)

	r := New(nil)
	for n := 0; n < b.N; n++ {
		r.Retrieve(context.Background(), resource)
	}
}

func BenchmarkGitRetrieveTag(b *testing.B) {
	public := pubRepoREADME + "@v0.0.1"
	resource := ParseResource(b, public)

	r := New(nil)
	for n := 0; n < b.N; n++ {
		r.Retrieve(context.Background(), resource)
	}
}

func BenchmarkGitRetrieveHEAD(b *testing.B) {
	resource := ParseResource(b, pubRepoREADME)

	r := New(nil)
	for n := 0; n < b.N; n++ {
		r.Retrieve(context.Background(), resource)
	}
}

func BenchmarkGitRetrieveBranch(b *testing.B) {
	public := pubRepoREADME + "@dev"
	resource := ParseResource(b, public)

	r := New(nil)
	for n := 0; n < b.N; n++ {
		r.Retrieve(context.Background(), resource)
	}
}

func ParseResource(t require.TestingT, str string) *retriever.Resource {
	r, err := retriever.ParseResource(str, `^([\w\.]+(\/[\w\-\_]+){2})((\/[\w+\.]+){1,})(@([\w\.\-]+))?$`, 1, 3, 6)
	require.NoError(t, err)
	return r
}

// Verify setting the repository using the various supported reference types (branch, tag, hash) sets the content.
func TestGitSession_SetReferenceTypes(t *testing.T) {
	tmpDir := "tmpdir"
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(tmpDir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(tmpDir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()
	repo := pubRepo
	cacher := NewPlainFscache(tmpDir)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(repo)
	session := NewSession(g)

	// Retrieve the main branch
	err = session.Set(context.Background(), repo, "main", SessionSetOpts{Fetch: true, Force: true})
	require.NoError(t, err)

	// Verify the content of the main branch
	file, err := ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve a specific hash
	err = session.Set(context.Background(), repo, pubRepoV1SHA, SessionSetOpts{Fetch: true, Force: true})
	require.NoError(t, err)

	// Verify the content of the hash
	file, err = ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve a tag
	err = session.Set(context.Background(), repo, "tags/v0.0.2", SessionSetOpts{Fetch: true, Force: true})
	require.NoError(t, err)

	// Verify the content of the tag
	file, err = ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Remove authentication methods to ensure another fetch cannot be done
	g.authMethods = []Authenticator{}

	// Retrieve the main branch again
	err = session.Set(context.Background(), repo, "main", SessionSetOpts{Fetch: true, Force: true})
	require.NoError(t, err)

	// Verify the content of the main branch
	file, err = ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))
}

// Verify setting the repository to a specific hash sets the content.
func TestGitSession_SetHash(t *testing.T) {
	tmpDir := "tmpdir"
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(tmpDir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(tmpDir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()
	repo := pubRepo
	cacher := NewPlainFscache(tmpDir)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(repo)
	session := NewSession(g)

	// Retrieve a specific hash
	err = session.Set(context.Background(), repo, pubRepoV1SHA, SessionSetOpts{Fetch: true, Force: true})
	require.NoError(t, err)

	// Verify the content of the hash
	file, err := ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))
}

// Verify setting the repository within a session that doesn't opt to pre-fetch references still sets the content.
func TestGitSession_SetNoFetch(t *testing.T) {
	tmpDir := "tmpdir"
	// Ensure tmpdir is empty to start with
	_, err := os.Stat(tmpDir)
	require.Error(t, err, "tmpdir is not empty before starting the test")
	defer func() {
		_, err := os.Stat(tmpDir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()
	repo := pubRepo
	cacher := NewPlainFscache(tmpDir)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(repo)
	session := NewSession(g)

	// Retrieve the main branch (without pre-fetching)
	err = session.Set(context.Background(), repo, "main", SessionSetOpts{Fetch: false, Force: true})
	require.NoError(t, err)

	// Verify the content of the main branch
	file, err := ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve a specific hash (without pre-fetching)
	err = session.Set(context.Background(), repo, pubRepoV1SHA, SessionSetOpts{Fetch: false, Force: true})
	require.NoError(t, err)

	// Verify the content of the hash
	file, err = ioutil.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))
}
