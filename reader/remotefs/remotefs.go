package remotefs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

var CacheDir string

func init() {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	CacheDir = filepath.Join(userCacheDir, "anz-bank.golden-retriever")
}

// NewWithGitRetriever initializes and returns an instance of RemoteFs with retriever git.Git.
func NewWithGitRetriever(fs *filesystem.Fs, options *git.AuthOptions) (*RemoteFs, error) {
	log.Debugf("cached git repositories folder: %s", CacheDir)
	return New(fs, git.NewWithCache(options, git.NewPlainFscache(CacheDir))), nil
}

// NewPinnerGitRetriever initializes and returns an instance of pinner.Pinner.
func NewPinnerGitRetriever(modFile string, options *git.AuthOptions) (retriever.Retriever, error) {
	log.Debugf("cached git repositories folder: %s", CacheDir)
	return pinner.New(modFile, git.NewWithCache(options, git.NewPlainFscache(CacheDir)))
}

// NewWithRetriever initializes and returns an instance of RemoteFs with a retriever.
func NewWithRetriever(fs *filesystem.Fs, retr retriever.Retriever) *RemoteFs {
	return New(fs, retr)
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

// IsRemote reports whether the path is a remote file.
// e.g. valid remote file paths:
// - github.com/foo/bar/path/to/file@v0.0.1
// - //github.com/foo/bar/path/to/file@v0.0.1
func (RemoteFs) IsRemote(path string) bool {
	if strings.HasPrefix(path, remoteImportPrefix) {
		return true
	}

	re, err := regexp.Compile(resourceRegexp)
	if err != nil {
		panic(fmt.Sprintf("compile regular expression %s error: %s", resourceRegexp, err))
	}
	return re.MatchString(path)
}

// resourceRegexp is the regular expression of remote file path string. e.g. github.com/foo/bar/path/to/file@v0.0.1
var resourceRegexp = `^((\w+\.)+(\w)+(/[\w-]+){2})((/[\w.-]+)+)(@([\w.-]+))?$`

// ParseResource takes a string in certain format and returns the corresponding resource.
func (RemoteFs) ParseResource(str string) (*retriever.Resource, error) {
	return retriever.ParseResource(strings.TrimPrefix(str, remoteImportPrefix), resourceRegexp, 1, 5, 8)
}
