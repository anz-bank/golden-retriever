package gitfs

import (
	"os"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func TestParseResource(t *testing.T) {
	// clone to in-memory storage
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:          "../../",
		SingleBranch: false,
	})
	require.NoError(t, err)

	// get long git ref
	ref, err := repo.ResolveRevision(plumbing.Revision("7e489e7c42dbfa96472426382183114090437ff3"))
	require.NoError(t, err)

	// get the commit
	commit, err := repo.CommitObject(*ref)
	require.NoError(t, err)

	fs := NewGitMemFs(commit)
	file, err := fs.Stat("once" + string(os.PathSeparator) + "once.go")
	require.NoError(t, err)
	require.NotNil(t, file)
}
