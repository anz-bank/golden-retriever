package git

import (
	"bytes"
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Verify setting the repository using the various supported reference types (branch, tag, hash) sets the content.
func TestGitSession_Set_ReferenceTypes(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)

	session := NewSession(g)

	// Retrieve the repository.
	err := session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve a specific hash
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve a tag
	err = session.Set(context.Background(), pubRepo, "tags/v0.0.2", SessionSetOpts{Verbose: true})
	require.NoError(t, err)

	// Verify the content of the tag
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Remove authentication methods to ensure another fetch cannot be done
	g.authMethods = []Authenticator{}

	// Set the repository.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))
}

// Verify setting the repository to a specific hash sets the content.
func TestGitSession_Set_Hash(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve a specific hash
	err := session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))
}

// Verify attempting to check out a repository without fetching results in an error.
func TestGitSession_Set_Fetch_Checkout(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	session := NewSession(g)

	// Retrieve the repository without fetching (which should fail because we have nothing).
	err := session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve the repository with explicit fetching (which should succeed).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Fetch: SessionSetOptFetchTrue, Verbose: true})
	require.NoError(t, err)
}

// Verify setting the repository to a hash reference with various fetch options.
func TestGitSession_Set_Fetch_Hash(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)

	session := NewSession(g)

	// Retrieve the repository at a hash.
	err := session.Set(context.Background(), pubRepo, pubRepoInitSHA, SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoInitContent, string(file))

	// Retrieve a different hash without fetching (which should fail because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different hash, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve the original hash without fetching (which should succeed because we know the hash).
	err = session.Set(context.Background(), pubRepo, pubRepoInitSHA, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoInitContent, string(file))
}

// Verify setting the repository to a short hash reference with various fetch options.
func TestGitSession_Set_Fetch_Hash_Short(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository at a hash.
	err := session.Set(context.Background(), pubRepo, pubRepoV2SHA[0:8], SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve a different hash without fetching (which should fail because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different hash, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve the original hash without fetching (which should succeed because we know the hash).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))
}

// Verify setting the repository to a short hash reference with various fetch options.
func TestGitSession_Set_Fetch_Hash_Short_Arbitrary(t *testing.T) {

	// Skip the failing test.
	// Note: The current go-git-version (v5.12.0) cannot fetch or clone a repository using an unknown short hash because
	// the hash fails to be resolved as a remote reference when attempted to be fetched.
	//
	// However, the following workarounds are available:
	// 1. Use the full hash: resolve it using the hash, just not an abbreviated one.
	// 2. Use a tagged hash: if the hash is associated with a tag or branch then it can be fetched with its short hash.
	// 3. Clone the entire repository: if a depth of zero is requested, then the hash is known and no fetch is required.
	t.Skip()

	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository at a hash.
	err := session.Set(context.Background(), pubRepo, pubRepoInitSHA[0:8], SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoInitContent, string(file))

	// Retrieve a different hash without fetching (which should fail because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different hash, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve the original hash without fetching (which should succeed because we know the hash).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))
}

// Verify setting the repository to a branch reference with various fetch options.
func TestGitSession_Set_Fetch_Branch(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository at a reference.
	err := session.Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// Retrieve a different reference without fetching (which should fail because we don't know the reference yet).
	err = session.Set(context.Background(), pubRepo, pubRepoDevelopBranch, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different reference, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoDevelopBranch, SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoDevelopContent, string(file))

	// Retrieve the original hash without fetching (which should false because we don't know if the remote reference has changed).
	err = session.Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)
}

// Verify setting the repository to a tag reference with various fetch options.
func TestGitSession_Set_Fetch_Tag_Prefix(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository at a reference.
	err := session.Set(context.Background(), pubRepo, "tags/"+pubRepoV1Tag, SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve a different reference without fetching (which should fail because we don't know the reference yet).
	err = session.Set(context.Background(), pubRepo, "tags/"+pubRepoV2Tag, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different reference, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, "tags/"+pubRepoV2Tag, SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve the original hash without fetching (which should false because we don't know if the remote reference has changed).
	err = session.Set(context.Background(), pubRepo, "tags/"+pubRepoV1Tag, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)
}

// Verify setting the repository to a tag reference with various fetch options.
func TestGitSession_Set_Fetch_Tag(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository at a reference.
	err := session.Set(context.Background(), pubRepo, pubRepoV1Tag, SessionSetOpts{Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Retrieve a different reference without fetching (which should fail because we don't know the reference yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV2Tag, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.Error(t, err)

	// Retrieve a different reference, allowing fetching if it is unknown (which should succeed because we don't know the hash yet).
	err = session.Set(context.Background(), pubRepo, pubRepoV2Tag, SessionSetOpts{Fetch: SessionSetOptFetchUnknown, Verbose: true, Depth: 1})
	require.NoError(t, err)

	// Verify the content of the repository
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Retrieve the original hash without fetching (which should false because we don't know if the remote reference has changed).
	err = session.Set(context.Background(), pubRepo, pubRepoV1Tag, SessionSetOpts{Fetch: SessionSetOptFetchFalse, Verbose: true})
	require.NoError(t, err)
}

// Verify resetting the repository with additional and modified files reset the repository to the correct state.
func TestGitSession_Set_Reset(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Retrieve the repository.
	err := session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{})
	require.NoError(t, err)

	// Verify the content of the repository.
	modifiedFileName := "README.md"
	file, err := os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Modify an existing file.
	modifiedFileContent := "The Cat in the Hat"
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Add a new file to the repository.
	newFileName := "new-file.txt"
	newFileContent := "Knows a lot about that"
	err = os.WriteFile(filepath.Join(repoDir, newFileName), []byte(newFileContent), 0666)

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Set the repository without resetting.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetFalse, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the modified file is unchanged.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Verify the content of the new file is unchanged.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Set the repository with reset.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetTrue, Verbose: true})
	require.NoError(t, err)

	// Verify the modified content has returned to the remote branch content.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Verify the new file no longer exists.
	_, err = os.Stat(filepath.Join(repoDir, newFileName))
	require.Error(t, err)
}

// Verify resetting the repository on the first time during the session.
func TestGitSession_Set_Reset_First(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)

	// Retrieve the repository (using a throwaway session).
	err := NewSession(g).Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{})
	require.NoError(t, err)

	// Verify the content of the repository.
	modifiedFileName := "README.md"
	file, err := os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Modify an existing file.
	modifiedFileContent := "The Cat in the Hat"
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Add a new file to the repository.
	newFileName := "new-file.txt"
	newFileContent := "Knows a lot about that"
	err = os.WriteFile(filepath.Join(repoDir, newFileName), []byte(newFileContent), 0666)

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Create a session.
	session := NewSession(g)

	// Set the repository to the same reference, resetting the first time (which should reset because it's the first time).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetFirst, Verbose: true})
	require.NoError(t, err)

	// Verify the modified content has returned to the remote branch content.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Verify the new file no longer exists.
	_, err = os.Stat(filepath.Join(repoDir, newFileName))
	require.Error(t, err)

	// Modify an existing file (again).
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Add a new file to the repository (again).
	err = os.WriteFile(filepath.Join(repoDir, newFileName), []byte(newFileContent), 0666)

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Set the repository to the same reference, resetting the first time (which should result in no action because it's not the first time).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetFirst, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))
}

// Verify resetting the repository on checkout during the session.
func TestGitSession_Set_Reset_OnCheckout(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)

	// Retrieve the repository (using a throwaway session).
	err := NewSession(g).Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{})
	require.NoError(t, err)

	// Verify the content of the repository.
	modifiedFileName := "README.md"
	file, err := os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Modify an existing file.
	modifiedFileContent := "The Cat in the Hat"
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Add a new file to the repository.
	newFileName := "new-file.txt"
	newFileContent := "Knows a lot about that"
	err = os.WriteFile(filepath.Join(repoDir, newFileName), []byte(newFileContent), 0666)

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Create a session.
	session := NewSession(g)

	// Set the repository to the same reference, resetting on checkout (which should do nothing because we're already at the correct reference).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetOnCheckout, Verbose: true})
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Verify the content of the new file.
	file, err = os.ReadFile(filepath.Join(repoDir, newFileName))
	require.NoError(t, err)
	require.Equal(t, newFileContent, string(file))

	// Set the repository to a different reference, resetting on checkout (which should reset because it's a different reference).
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Reset: SessionSetOptResetOnCheckout, Verbose: true})
	require.NoError(t, err)

	// Verify the modified content is now at the requested reference.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Verify the new file no longer exists.
	_, err = os.Stat(filepath.Join(repoDir, newFileName))
	require.Error(t, err)

	// Modify an existing file.
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Set the repository to a different reference, without resetting (which should result in an error because there are unstaged changes).
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Reset: SessionSetOptResetFalse, Verbose: true})
	require.Error(t, err)
}

// Verify the current state of the repository within the session.
func TestGitSession_Set_Verify(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)
	session := NewSession(g)

	// Verify the state of a non-existent repository (using a throwaway session).
	err := NewSession(g).Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verify: true})
	require.ErrorContains(t, err, "was asked to be verified at reference")
	require.ErrorContains(t, err, "but doesn't exist")

	// Retrieve the repository.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{})
	require.NoError(t, err)

	// Verify the content of the repository.
	modifiedFileName := "README.md"
	file, err := os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, pubRepoV2Content, string(file))

	// Verify the repository is set to the requested reference.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verify: true})
	require.NoError(t, err)

	// Verify the repository is not set to a different reference.
	err = session.Set(context.Background(), pubRepo, pubRepoV1SHA, SessionSetOpts{Verify: true})
	require.ErrorContains(t, err, "was asked to be verified")
	require.ErrorContains(t, err, "but was at")

	// Modify an existing file.
	modifiedFileContent := "The Cat in the Hat"
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)

	// Verify the content of the modified file.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Verify the repository is still set to the requested reference.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verify: true})
	require.NoError(t, err)

	// Verify the modified file is unchanged.
	file, err = os.ReadFile(filepath.Join(repoDir, modifiedFileName))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))

	// Verify the repository is set to the reference, but resetting would modify.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verify: true, Reset: SessionSetOptResetTrue})
	require.ErrorContains(t, err, "verified to be at reference")
	require.ErrorContains(t, err, "but requested reset would modify contents")

	// Return the modified file to its original state.
	err = os.WriteFile(filepath.Join(repoDir, modifiedFileName), []byte(pubRepoV2Content), 0666)
	require.NoError(t, err)

	// Verify the repository is set to the reference, and no reset is required.
	err = session.Set(context.Background(), pubRepo, pubRepoV2SHA, SessionSetOpts{Verify: true, Reset: SessionSetOptResetTrue})
	require.NoError(t, err)
}

// Verify requesting different fetch behaviours within the session.
func TestGitSession_Set_Fetch(t *testing.T) {
	tmp := t.TempDir()
	cacher := NewPlainFscache(tmp)
	g := NewWithCache(nil, cacher)
	repoDir := cacher.RepoDir(pubRepo)

	// Create a buffer to store the logs for the test.
	// For simplicity of testing, we inspect the logs to infer test behaviour.
	buffer := bytes.NewBuffer(make([]byte, 0))
	log.SetLevel(log.DebugLevel)
	log.SetOutput(buffer)

	// Retrieve the repository at a named branch.
	err := NewSession(g).Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Depth: 0})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, move the head to a different commit.
	err = execute(repoDir, "git", "reset", "--hard", pubRepoV1SHA)
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the named branch again without fetching. Use a new session to avoid the cached hash.
	// Note: The repository should not be fetched because it was requested not to be.
	err = NewSession(g).Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchFalse})
	require.NoError(t, err)

	// Verify that no fetch was performed.
	assert.NotContains(t, buffer.String(), "fetching")

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, move the head to a different commit.
	err = execute(repoDir, "git", "reset", "--hard", pubRepoV1SHA)
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the named branch again with fetching. Use a new session to avoid the cached hash.
	// Note: The repository should be fetched because it was requested to be, and the reference is not a hash.
	err = NewSession(g).Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchTrue})
	require.NoError(t, err)

	// Verify that a fetch was performed.
	assert.Contains(t, buffer.String(), fmt.Sprintf("fetching ref: %v", pubRepoMainBranch))

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, move the head to a different commit.
	err = execute(repoDir, "git", "reset", "--hard", pubRepoV1SHA)
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the named branch again with fetching if unknown. Use a new session to avoid the cached hash.
	// Note: The repository should not be fetched because the branch reference (main) is already known.
	err = NewSession(g).Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchUnknown})
	require.NoError(t, err)

	// Verify that no fetch was performed.
	assert.NotContains(t, buffer.String(), "fetching")

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, move the head to a different commit.
	err = execute(repoDir, "git", "reset", "--hard", pubRepoV1SHA)
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the hash of the named branch with fetching true. Use a new session to avoid the cached hash.
	// Note: The repository should not be fetched because the hash reference is already known.
	err = NewSession(g).Set(context.Background(), pubRepo, pubRepoMainSHA, SessionSetOpts{Fetch: SessionSetOptFetchTrue})
	require.NoError(t, err)

	// Verify that no fetch was performed.
	assert.NotContains(t, buffer.String(), "fetching")

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, move the head to a different commit.
	err = execute(repoDir, "git", "reset", "--hard", pubRepoV1SHA)
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoV1Content, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the short hash of the named branch with fetching true. Use a new session to avoid the cached hash.
	// Note: The repository should not be fetched because the (short) hash reference is already known.
	err = NewSession(g).Set(context.Background(), pubRepo, pubRepoMainSHA[0:8], SessionSetOpts{Fetch: SessionSetOptFetchTrue})
	require.NoError(t, err)

	// Verify that no fetch was performed.
	assert.NotContains(t, buffer.String(), "fetching")

	// Verify the content of the repository.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))
}

// Verify that fetching a reference in a remote repository that has changed is updated correctly.
func TestGitSession_Set_Fetch_Remote_Updated(t *testing.T) {

	// Construct a "remote" repository in the local file system, initialised with the contents of an actual remote repository.
	remoteCacher := NewPlainFscache(t.TempDir())
	remoteGit := NewWithCache(nil, remoteCacher)
	remoteRepoDir := remoteCacher.RepoDir(pubRepo)
	remoteRepo := remoteRepoDir
	err := NewSession(remoteGit).Set(context.Background(), pubRepo, pubRepoMainBranch, SessionSetOpts{Depth: 0})
	assert.NoError(t, err)

	// Clone the repository under test from the "remote" repository.
	// Note: Setting the shared value during the initial clone is sufficient to continue interacting with a local remote repository.
	cacher := NewPlainFscache(t.TempDir())
	g := NewWithCache(&AuthOptions{Local: true}, cacher)
	repoDir := cacher.RepoDir(remoteRepoDir)
	//_, err = g.CloneRepo(context.Background(), remoteRepo, CloneOpts{Shared: true})
	//assert.NoError(t, err)

	// Create a buffer to store the logs for the test.
	// For simplicity of testing, we inspect the logs to infer test behaviour.
	buffer := bytes.NewBuffer(make([]byte, 0))
	log.SetLevel(log.DebugLevel)
	log.SetOutput(buffer)

	// Retrieve the repository at a named branch.
	err = NewSession(g).Set(context.Background(), remoteRepo, pubRepoMainBranch, SessionSetOpts{Depth: 0})
	require.NoError(t, err)

	// Verify the content of the repository.
	file, err := os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// In an out-of-band process, add a new commit to the remote repository.
	modifiedFileName := "README.md"
	modifiedFileContent := "The Cat in the Hat"
	err = os.WriteFile(filepath.Join(remoteRepoDir, modifiedFileName), []byte(modifiedFileContent), 0666)
	require.NoError(t, err)
	err = execute(remoteRepoDir, "git", "checkout", pubRepoMainBranch)
	require.NoError(t, err)
	err = execute(remoteRepoDir, "git", "add", ".")
	require.NoError(t, err)
	err = execute(remoteRepoDir, "git", "commit", "-m", "Change One")
	require.NoError(t, err)

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the named branch again without fetching. Use a new session to avoid the cached hash.
	// Note: The repository should not be fetched because it was requested not to be.
	err = NewSession(g).Set(context.Background(), remoteRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchFalse})
	require.NoError(t, err)

	// Verify that no fetch was performed.
	assert.NotContains(t, buffer.String(), "fetching")

	// Verify the content of the repository is unchanged.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, pubRepoMainContent, string(file))

	// Clear the log buffer.
	buffer.Reset()

	// Set the repository to the named branch again with fetching. Use a new session to avoid the cached hash.
	// Note: The repository should be fetched because it was requested to be, and the reference is not a hash.
	err = NewSession(g).Set(context.Background(), remoteRepo, pubRepoMainBranch, SessionSetOpts{Fetch: SessionSetOptFetchTrue})
	require.NoError(t, err)

	// Verify that a fetch was performed.
	assert.Contains(t, buffer.String(), fmt.Sprintf("fetching ref: %v", pubRepoMainBranch))

	// Verify the content of the repository has been updated.
	file, err = os.ReadFile(filepath.Join(repoDir, "README.md"))
	require.NoError(t, err)
	require.Equal(t, modifiedFileContent, string(file))
}

// execute the given command.
func execute(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if len(out) != 0 {
		fmt.Print(string(out))
	}
	switch err.(type) {
	case *exec.ExitError:
		return fmt.Errorf("error executing command %v %v %v",
			args, string(err.(*exec.ExitError).Stderr), err)
	}
	return err
}
