package git

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/stretchr/testify/require"
	"github.com/undefinedlabs/go-mpatch"
)

const (
	pubRepo       = "github.com/SyslBot/a-public-repo"
	pubRepoREADME = pubRepo + "/README.md"

	pubRepoInitSHA       = "1e7c4cecaaa8f76e3c668cebc411f1b03171501f"
	pubRepoV1SHA         = "f948d44b0d97dbbe019949c8b574b5f246b25dc2"
	pubRepoV2SHA         = "6a27bac5e5c379649c5b4574845744957cd6c749"
	pubRepoMainSHA       = pubRepoV2SHA
	pubRepoV1Tag         = "v0.0.1"
	pubRepoV2Tag         = "v0.0.2"
	pubRepoDevelopBranch = "develop"
	pubRepoMainBranch    = "main"

	pubRepoInitContent    = "# a-public-repo\nA public repo for modules testing\n"
	pubRepoV1Content      = "# a-public-repo v0.0.1\nA public repo for modules testing\n"
	pubRepoV2Content      = "# a-public-repo v0.0.2\nA public repo for modules testing\n"
	pubRepoDevelopContent = "# a-public-repo-dev\nA public repo for modules testing\n"
	pubRepoMainContent    = pubRepoV2Content
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
		{pubRepoREADME + "@" + pubRepoMainBranch, pubRepoV2Content},
		{pubRepoREADME + "@" + pubRepoDevelopBranch, pubRepoDevelopContent},
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
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	r := NewWithCache(nil, cacher)

	resource := ParseResource(t, pubRepoREADME)
	c, err := r.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(c))
}

func TestGitRetrieveHEADThenTag(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	r := NewWithCache(nil, cacher)

	resource := ParseResource(t, pubRepoREADME+"@"+pubRepoMainBranch)
	c, err := r.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(c))

	resourcev1 := ParseResource(t, pubRepoREADME+"@v0.0.1")
	c, err = r.Retrieve(context.Background(), resourcev1)
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(c))
}

func TestGitRetrieveCloneThenFetchRepo(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	r := NewWithCache(nil, cacher)

	tests := []struct {
		resourceStr string
		content     string
	}{
		{pubRepoREADME + "@" + pubRepoInitSHA, pubRepoInitContent},
		{pubRepoREADME + "@v0.0.1", pubRepoV1Content},
		{pubRepoREADME + "@" + pubRepoMainBranch, pubRepoMainContent},
		{pubRepoREADME + "@" + pubRepoDevelopBranch, pubRepoDevelopContent},
	}

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
