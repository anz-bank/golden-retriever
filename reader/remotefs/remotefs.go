package remotefs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/anz-bank/golden-retriever/pinner"
	"github.com/anz-bank/golden-retriever/reader/filesystem"
	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/anz-bank/golden-retriever/retriever/git"
	log "github.com/sirupsen/logrus"
)

// RemoteFs
type RemoteFs struct {
	fs        *filesystem.Fs
	retriever retriever.Retriever
}

// New initializes and returns an instance of RemoteFs.
func New(fs *filesystem.Fs, retriever retriever.Retriever) *RemoteFs {
	return &RemoteFs{
		fs:        fs,
		retriever: retriever,
	}
}

// NewWithGitRetriever initializes and returns an instance of RemoteFs with retriever git.Git.
func NewWithGitRetriever(fs *filesystem.Fs, modFile string, options *git.AuthOptions) (*RemoteFs, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	cacheDir := filepath.Join(userCacheDir, "ANZ.GoldenRetriever")

	retr, err := pinner.New(modFile, git.NewWithCache(options, git.NewFscache(cacheDir)))
	if err != nil {
		return nil, err
	}
	log.Debugf("cached git repositories folder: %s", cacheDir)

	return New(fs, retr), nil
}

// Read returns the content of the file (either local or remote).
func (r RemoteFs) Read(ctx context.Context, path string) ([]byte, error) {
	b, _, err := r.ReadHash(ctx, path)
	return b, err
}

// Read returns the content of the file (either local or remote). If it is a remote file, returns the commit hash as well.
func (r RemoteFs) ReadHash(ctx context.Context, path string) ([]byte, retriever.Hash, error) {
	if r.IsRemote(path) {
		resource, err := r.ParseResource(path)
		if err != nil {
			return nil, retriever.ZeroHash, err
		}
		b, err := r.retriever.Retrieve(ctx, resource)
		if err != nil {
			return nil, retriever.ZeroHash, err
		}
		return b, resource.Ref.Hash(), nil
	}

	return r.fs.ReadHash(ctx, path)
}

const remoteImportPrefix = "//"

// IsRemoteImport reports whether the path is a remote file.
func IsRemoteImport(path string) bool {
	return strings.HasPrefix(path, remoteImportPrefix)
}

// IsRemote reports whether the path is a remote file.
func (RemoteFs) IsRemote(path string) bool {
	return IsRemoteImport(path)
}

// ResourceRegexp is the regular expression of remote file path string. e.g. //github.com/foo/bar/path/to/file@v0.0.1
var ResourceRegexp = `^//([\w.]+(/[\w-]+){2})((/[\w.-]+)+)(@([\w.-]+))?$`

// ParseResource takes a string in certain format and returns the corresponding resource.
func (RemoteFs) ParseResource(str string) (*retriever.Resource, error) {
	return retriever.ParseResource(str, ResourceRegexp, 1, 3, 6)
}
