package gitfs

import (
	"os"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

type gitMemFs struct {
	c *object.Commit
}

// NewGitMemFs returns a read-only afero filesystem based on a commit.
func NewGitMemFs(c *object.Commit) afero.Fs {
	return afero.NewReadOnlyFs(&gitMemFs{c})
}

func (g *gitMemFs) Open(name string) (afero.File, error) {
	f, err := g.c.File(name)
	if err != nil {
		return nil, err
	}
	return NewGitFile(f)
}

func (g *gitMemFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// flag and perm shouldn't matter, this is a read only filesystem
	return g.Open(name)
}

func (g *gitMemFs) Stat(name string) (os.FileInfo, error) {
	f, err := g.Open(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

func (g *gitMemFs) Name() string {
	return "GitMemFs"
}

// These functions will not be called as gitMemFs is wrapped with ReadOnlyFs.

func (g *gitMemFs) Create(name string) (afero.File, error) {
	panic("unimplemented")
}

func (g *gitMemFs) Mkdir(name string, perm os.FileMode) error {
	panic("unimplemented")
}

func (g *gitMemFs) MkdirAll(path string, perm os.FileMode) error {
	panic("unimplemented")
}

func (g *gitMemFs) Remove(name string) error {
	panic("unimplemented")
}

func (g *gitMemFs) RemoveAll(path string) error {
	panic("unimplemented")
}

func (g *gitMemFs) Rename(oldname, newname string) error {
	panic("unimplemented")
}

func (g *gitMemFs) Chmod(name string, mode os.FileMode) error {
	panic("unimplemented")
}

func (g *gitMemFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	panic("unimplemented")
}
