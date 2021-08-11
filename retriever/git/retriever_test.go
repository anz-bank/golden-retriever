package git

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anz-bank/golden-retriever/retriever"

	"bou.ke/monkey"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	pubRepo            = "github.com/SyslBot/a-public-repo"
	pubRepoREADME      = pubRepo + "/README.md"
	pubRepoInitSHA     = "1e7c4cecaaa8f76e3c668cebc411f1b03171501f"
	pubRepoV1SHA       = "f948d44b0d97dbbe019949c8b574b5f246b25dc2"
	pubRepoV2SHA       = "6a27bac5e5c379649c5b4574845744957cd6c749"
	pubRepoDevSHA      = "7e4ece290cbbb72a77660f91a2e12e191560f7e6"
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

func TestGitRetrieveCloneWrongResource(t *testing.T) {
	monkey.Patch(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})
	defer monkey.UnpatchAll()

	tests := []struct {
		resourceStr string
		errmsg      string
	}{
		{pubRepo + "/README", "git show: file not found"},
		{pubRepo + "/wrong.md", "git show: file not found"},
		{pubRepo + "/README.md@nosuchbranch", "git clone: Unable to authenticate, tried: \n    - None: reference nosuchbranch not found"},
		{pubRepo + "/README.md@v100.0.0", "git clone: Unable to authenticate, tried: \n    - None: reference v100.0.0 not found"},
		{pubRepo + "/README.md@commitshanotfoundc668cebc411f1b03171501f", "git clone: Unable to authenticate, tried: \n    - None: reference commitshanotfoundc668cebc411f1b03171501f not found"},
	}

	for _, tr := range tests {
		t.Run(tr.resourceStr, func(t *testing.T) {
			r := New(nil)
			content, err := r.Retrieve(context.Background(), ParseResource(t, tr.resourceStr))
			require.EqualError(t, err, tr.errmsg)
			require.Equal(t, "", string(content))
		})
	}
}

func TestGitRetrieveClonePublicRepo(t *testing.T) {
	monkey.Patch(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})
	defer monkey.UnpatchAll()

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

func TestGitRetrieveToFileSystem(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tmpDir := "tmpdir"
	repodir := filepath.Join(tmpDir, pubRepo)
	defer func() {
		_, err := os.Stat(repodir)
		require.NoError(t, err)
		err = os.RemoveAll(tmpDir)
		require.NoError(t, err)
	}()

	resourcev1 := ParseResource(t, pubRepoREADME+"@v0.0.1")
	r := NewWithCache(nil, NewFscache(tmpDir))
	c, err := r.Retrieve(context.Background(), resourcev1)
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(c))

	resource := ParseResource(t, pubRepoREADME)
	c, err = r.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(c))
}

func TestGitRetrieveCloneThenFetchRepo(t *testing.T) {
	tmpDir := "tmpdir"
	repodir := filepath.Join(tmpDir, pubRepo)
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
		{pubRepoREADME, pubRepoV2Content},
		{pubRepoREADME + "@main", pubRepoV2Content},
		{pubRepoREADME + "@develop", pubRepoDevContent},
	}

	r := NewWithCache(nil, NewFscache(tmpDir))
	for _, tr := range tests {
		t.Run(tr.resourceStr, func(t *testing.T) {
			content, err := r.Retrieve(context.Background(), ParseResource(t, tr.resourceStr))
			require.NoError(t, err)
			require.Equal(t, tr.content, string(content))
		})
	}
}

func TestGitRetrievePrivateRepoAuth(t *testing.T) {
	username := "SyslBot"
	password := os.Getenv("TEST_PRIV_REPO_TOKEN")
	resource := ParseResource(t, privRepoREADME)

	monkey.Patch(NewSSHAgent, func() (*SSHAgent, error) {
		return nil, errors.New("Create SSH Agent failed")
	})

	noneGit := New(nil)
	c, err := noneGit.Retrieve(context.Background(), resource)
	require.EqualError(t, err, "git clone: Unable to authenticate, tried: \n    - None: authentication required")
	require.Equal(t, "", string(c))

	tokenGit := New(&AuthOptions{Tokens: map[string]string{"github.com": password}})
	c, err = tokenGit.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, privRepoContent, string(c))

	wrongTokenGit := New(&AuthOptions{Tokens: map[string]string{"github.com": "foobar"}})
	c, err = wrongTokenGit.Retrieve(context.Background(), resource)
	require.EqualError(t, err, "git clone: Unable to authenticate, tried: \n    - None: authentication required,\n    - Username and Password/Token: authentication required")
	require.Equal(t, "", string(c))

	pwGit := New(&AuthOptions{Credentials: map[string]Credential{"github.com": Credential{username, password}}})
	c, err = pwGit.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, privRepoContent, string(c))

	tmpDir := "tmpdir"
	sshKey := tmpDir + "/id_ed25519"
	repodir := filepath.Join(tmpDir, testHost+"/a-private-repo")

	err = os.Mkdir(tmpDir, os.ModePerm)
	require.NoError(t, err)
	err = ioutil.WriteFile(sshKey, []byte(os.Getenv("TEST_SSH_KEY")), os.ModePerm)
	require.NoError(t, err)

	keyGit := New(&AuthOptions{SSHPrivateKey: sshKey}, NewFscache(tmpDir))
	c, err = keyGit.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, content, string(c))

	err = exec.Command("ssh-add", "-K", sshKey).Run()
	require.NoError(t, err)

	defer func() {
		err = exec.Command("ssh-add", "-d", sshKey).Run()
		require.NoError(t, err)
	}()

	clearTmpdir := func(t *testing.T, repodir, dir string) {
		_, err = os.Stat(repodir)
		require.NoError(t, err)
		err = os.RemoveAll(dir)
		require.NoError(t, err)
	}

	clearTmpdir(t, repodir, tmpDir)

	monkey.UnpatchAll()

	sshagentGit := New(nil, NewFscache(tmpDir))
	c, err = sshagentGit.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, content, string(c))

	clearTmpdir(t, repodir, tmpDir)
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
