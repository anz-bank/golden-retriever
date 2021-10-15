package git

import (
	"path/filepath"
	"sync"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Cacher is an interface to cache git repositories.
type Cacher interface {
	// Get repository via the repo name.
	Get(string) (*git.Repository, bool)
	// Set save repository with the repo name key.
	Set(string, *git.Repository)
	// NewStorer returns a new storage.Storer with repo name like github.com/org/repo.
	NewStorer(string) storage.Storer
}

// MemCache implements the Cacher interface storing repositories in memory.
type MemCache struct {
	repos map[string]*git.Repository
	mutex sync.RWMutex
}

// NewMemcache returns a new MemCache.
func NewMemcache() MemCache {
	return MemCache{
		repos: make(map[string]*git.Repository),
		mutex: sync.RWMutex{},
	}
}

func (s MemCache) Get(repo string) (*git.Repository, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	r, ok := s.repos[repo]
	return r, ok
}

func (s MemCache) Set(repo string, v *git.Repository) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.repos[repo] = v
}

func (s MemCache) NewStorer(repo string) storage.Storer {
	return memory.NewStorage()
}

// FsCache implements the Cacher interface storing repositories in filesystem.
type FsCache struct {
	dir string
}

// NewFscache returns a new FsCache.
func NewFscache(dir string) FsCache {
	return FsCache{dir: dir}
}

func (s FsCache) Get(repo string) (*git.Repository, bool) {
	st := s.NewStorer(repo)
	r, err := git.Open(st, nil)
	if err != nil {
		return nil, false
	}

	return r, true
}

func (s FsCache) Set(repo string, v *git.Repository) {
	if _, is := v.Storer.(*filesystem.Storage); !is {
		panic("it is not a filesystem storage")
	}
}

func (s FsCache) NewStorer(repo string) storage.Storer {
	return filesystem.NewStorage(osfs.New(s.repoDir(repo)), cache.NewObjectLRUDefault())
}

func (s FsCache) repoDir(repo string) string {
	return filepath.Join(s.dir, repo)
}

// PlainFsCache implements the Cacher interface storing repositories in filesystem
// without extra storage.Storer files.
type PlainFsCache struct {
	dir string
}

// NewPlainFscache returns a new PlainFsCache.
func NewPlainFscache(dir string) PlainFsCache {
	return PlainFsCache{dir: dir}
}

func (s PlainFsCache) Get(repo string) (*git.Repository, bool) {
	r, err := git.PlainOpen(s.RepoDir(repo))
	if err != nil {
		return nil, false
	}

	return r, true
}

func (s PlainFsCache) Set(repo string, v *git.Repository) {
	if _, is := v.Storer.(*filesystem.Storage); !is {
		panic("it is not a filesystem storage")
	}
}

func (s PlainFsCache) NewStorer(repo string) storage.Storer {
	panic("storage.Storer not supported by PlainFsCache")
}

func (s PlainFsCache) RepoDir(repo string) string {
	return filepath.Join(s.dir, repo)
}
