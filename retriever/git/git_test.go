package git

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

// Verify setting the repository using the various supported reference types (branch, tag, hash) sets the content.
func TestGitSession_SetReferenceTypes(t *testing.T) {
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
func TestGitSession_SetHash(t *testing.T) {
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
func TestGitSession_SetReset(t *testing.T) {
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
func TestGitSession_SetReset_First(t *testing.T) {
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
func TestGitSession_SetReset_OnCheckout(t *testing.T) {
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
