package gitfs

import (
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

// gitFile is an afero.File wrapper on *object.File. It behaves just like a
// read-only file. It does not allow any modifications on the file.
type gitFile struct {
	r *strings.Reader
	f *object.File
}

// NewGitFile returns a read-only afero.File based on a git file.
func NewGitFile(f *object.File) (afero.File, error) {
	contents, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return &gitFile{f: f, r: strings.NewReader(contents)}, nil
}

func (g *gitFile) Close() error {
	return nil
}

func (g *gitFile) Read(p []byte) (n int, err error) {
	return g.r.Read(p)
}

func (g *gitFile) ReadAt(p []byte, off int64) (n int, err error) {
	return g.r.ReadAt(p, off)
}

func (g *gitFile) Seek(offset int64, whence int) (int64, error) {
	return g.r.Seek(offset, whence)
}

// Writes are not allowed
func (g *gitFile) Write(p []byte) (n int, err error) {
	return g.WriteAt(p, 0)
}

func (g *gitFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, os.ErrPermission
}

func (g *gitFile) Name() string {
	return g.f.Name
}

func (g *gitFile) Readdir(count int) ([]os.FileInfo, error) {
	panic("unimplemented")
}

func (g *gitFile) Readdirnames(n int) ([]string, error) {
	panic("unimplemented")
}

type GitFileInfo struct {
	f *object.File
}

func (g *GitFileInfo) Name() string {
	return g.f.Name
}

func (g *GitFileInfo) Size() int64 {
	return g.f.Size
}

func (g *GitFileInfo) Mode() os.FileMode {
	return os.FileMode(g.f.Mode)
}

func (g *GitFileInfo) ModTime() time.Time {
	// FIXME: not really sure where I can get time, maybe from commit?
	panic("unimplemented")
}

func (g *GitFileInfo) IsDir() bool {
	return !g.f.Mode.IsFile()
}

func (g *GitFileInfo) Sys() interface{} {
	return g.f
}

func (g *gitFile) Stat() (os.FileInfo, error) {
	return &GitFileInfo{g.f}, nil
}

func (g *gitFile) Sync() error {
	// should always be updated based on the commit
	return nil
}

func (g *gitFile) Truncate(size int64) error {
	return os.ErrPermission
}

func (g *gitFile) WriteString(s string) (ret int, err error) {
	return -1, os.ErrPermission
}
