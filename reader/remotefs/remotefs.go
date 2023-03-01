package remotefs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anz-bank/golden-retriever/pinner"
	"github.com/anz-bank/golden-retriever/reader"
	"github.com/anz-bank/golden-retriever/reader/filesystem"
	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/anz-bank/golden-retriever/retriever/git"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/afero"
)

// ensures RemoteFs implements afero and Reader
var _ afero.Fs = &RemoteFs{}
var _ reader.Reader = &RemoteFs{}

// RemoteFs
type RemoteFs struct {
	*filesystem.Fs
	retriever retriever.Retriever
	vendorDir string
}

// New initializes and returns an instance of RemoteFs.
func New(fs *filesystem.Fs, retriever retriever.Retriever) *RemoteFs {
	return &RemoteFs{
		Fs:        fs,
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
func (r *RemoteFs) Read(ctx context.Context, path string) ([]byte, error) {
	b, _, _, err := r.ReadHashBranch(ctx, path)
	return b, err
}

// Read returns the content of the file (either local or remote). If it is a remote file, returns the commit hash as well.
func (r *RemoteFs) ReadHash(ctx context.Context, path string) ([]byte, retriever.Hash, error) {
	b, h, _, err := r.ReadHashBranch(ctx, path)
	return b, h, err
}

func (r *RemoteFs) ReadHashBranch(ctx context.Context, path string) ([]byte, retriever.Hash, string, error) {
	if r.IsRemote(path) {
		resource, err := r.ParseResource(path)
		if err != nil {
			return nil, retriever.ZeroHash, "", err
		}

		if r.vendorDir != "" {
			if _, err := os.Stat(filepath.Join(r.vendorDir, path)); err == nil {
				body, err := ioutil.ReadFile(filepath.Join(r.vendorDir, path))
				if err == nil {
					return body, resource.Ref.Hash(), resource.Ref.Name(), nil
				}
			}
		}

		b, err := r.retriever.Retrieve(ctx, resource)
		if err != nil {
			return nil, retriever.ZeroHash, "", err
		}

		if r.vendorDir != "" {
			p := filepath.Join(r.vendorDir, resource.String())
			err = os.MkdirAll(filepath.Dir(p), os.ModePerm)
			if err != nil {
				return nil, resource.Ref.Hash(), resource.Ref.Name(), err
			}
			err = ioutil.WriteFile(p, b, 0644)
			if err != nil {
				return nil, resource.Ref.Hash(), resource.Ref.Name(), err
			}
		}

		return b, resource.Ref.Hash(), resource.Ref.Name(), nil
	}

	return r.Fs.ReadHashBranch(ctx, path)
}

func (r *RemoteFs) Vendor(dir string) {
	r.vendorDir = filepath.Clean(dir)
	log.Info("vendor files are stored under", r.vendorDir)
}

const remoteImportPrefix = "//"

// IsRemote reports whether the path is a remote file.
// e.g. valid remote file paths:
// - github.com/foo/bar/path/to/file@v0.0.1
// - //github.com/foo/bar/path/to/file@v0.0.1
func (*RemoteFs) IsRemote(path string) bool {
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
var resourceRegexp = `^((\w+\.)+(\w)+(/[\w-]+){2})((/[\w.-]+)+)(@([\w./-]+))?$`

// ParseResource takes a string in certain format and returns the corresponding resource.
func (*RemoteFs) ParseResource(str string) (*retriever.Resource, error) {
	return retriever.ParseResource(strings.TrimPrefix(str, remoteImportPrefix), resourceRegexp, 1, 5, 8)
}
