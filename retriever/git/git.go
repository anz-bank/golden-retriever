package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.WarnLevel)
}

var nomatchspecErr = git.NoMatchingRefSpecError{}

// Clone a repository into the given cache directory.
func (a Git) Clone(ctx context.Context, resource *retriever.Resource) (r *git.Repository, err error) {
	repo := resource.Repo

	if resource.Ref.IsHash() {
		s := a.cacher.NewStorer(repo)

		r, err = git.Init(s, nil)
		a.cacher.Set(repo, &git.Repository{Storer: s})
		if err != nil {
			return
		}

		err = a.FetchCommit(ctx, r, repo, resource.Ref.Hash())
		if err != nil {
			return nil, err
		}
		return r, nil
	}

	tried := []string{}

	for _, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options := &git.CloneOptions{
			URL:          url,
			Depth:        1,
			NoCheckout:   true,
			Auth:         auth,
			SingleBranch: true,
		}

		ref := resource.Ref.Name()
		rules := retriever.RefRules
		if ref == "HEAD" {
			ref = "heads"
			rules = []string{"refs/%s/master", "refs/%s/main"}
		}

		for iter := retriever.NewRefIterator(rules, ref); iter.Next(); {
			options.ReferenceName = plumbing.ReferenceName(iter.Current())
			mems := memory.NewStorage()
			r, err = git.CloneContext(ctx, mems, nil, options)
			if err == nil {
				r, err = git.CloneContext(ctx, a.cacher.NewStorer(repo), nil, options)
				a.cacher.Set(repo, r)
				resource.Ref.SetName(iter.Current())
				return
			}
		}

		errmsg := err.Error()
		if nomatchspecErr.Is(err) {
			errmsg = fmt.Sprintf("reference %s not found", ref)
		}
		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), errmsg))
	}

	err = a.runCloneCmd(ctx, repo)
	if err == nil {
		s := a.cacher.NewStorer(repo)
		a.cacher.Set(repo, &git.Repository{Storer: s})
		return &git.Repository{Storer: s}, nil
	}

	return nil, fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

func (a Git) Fetch(ctx context.Context, r *git.Repository, resource *retriever.Resource) error {
	if resource.Ref.IsHash() {
		return a.FetchCommit(ctx, r, resource.Repo, resource.Ref.Hash())
	}
	return a.FetchRef(ctx, r, resource.Repo, resource.Ref.Name())
}

// FetchRef
func (a Git) FetchRef(ctx context.Context, r *git.Repository, repo string, ref string) (err error) {
	options := &git.FetchOptions{
		Depth: 1,
	}

	tried := []string{}
	for _, meth := range a.authMethods {
		auth, _ := meth.AuthMethod(repo)
		options.Auth = auth

		for iter := retriever.NewRefIterator(retriever.RefRules, ref); iter.Next(); {
			refSpec := iter.Current()
			if refSpec == "HEAD" {
				refSpec = "+HEAD:refs/remotes/origin/HEAD"
			} else {
				refSpec = "+" + refSpec + ":" + refSpec
			}
			options.RefSpecs = []config.RefSpec{config.RefSpec(refSpec)}

			err = r.FetchContext(ctx, options)
			if err == nil || err == git.NoErrAlreadyUpToDate {
				return nil
			}

			fmt.Println(err, refSpec)
			e := a.runFetchCmd(ctx, repo, refSpec)
			if e == nil {
				return nil
			}
		}

		errmsg := err.Error()
		if nomatchspecErr.Is(err) {
			errmsg = fmt.Sprintf("reference %s not found", ref)
		}
		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), errmsg))
	}

	return fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

// FetchCommit the latest history of a repository in the cache directory.
func (a Git) FetchCommit(ctx context.Context, r *git.Repository, repo string, hash retriever.Hash) error {
	_, err := r.CommitObject(plumbing.NewHash(hash.String()))
	if err == nil {
		return nil
	}

	isEmpty := false
	remotes, err := r.Remotes()
	if err != nil {
		return err
	} else if len(remotes) == 0 {
		isEmpty = true
	}

	refSpec := fmt.Sprintf("+%s:%s", hash, hash)
	options := &git.FetchOptions{
		Depth:    1,
		RefSpecs: []config.RefSpec{config.RefSpec(refSpec)},
	}

	tried := []string{}
	for i, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options.Auth = auth

		if isEmpty {
			switch {
			case i > 0:
				remote, err := r.Remote(git.DefaultRemoteName)
				if err != nil || err != git.ErrRemoteNotFound {
					return err
				}

				if remote.Config().URLs[0] == url {
					break
				}

				err = r.DeleteRemote(git.DefaultRemoteName)
				if err != nil {
					return err
				}
				fallthrough
			case i == 0:
				if _, err = r.CreateRemote(&config.RemoteConfig{
					Name: git.DefaultRemoteName,
					URLs: []string{url},
				}); err != nil {
					return err
				}
			}
		}

		err = r.FetchContext(ctx, options)
		if err == nil || err == git.NoErrAlreadyUpToDate {
			return nil
		}

		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), err.Error()))

		err = a.runFetchCmd(ctx, repo, refSpec)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

func (a Git) runCloneCmd(ctx context.Context, repo string) error {
	// Try plain command as long as it works in shell
	if v, is := a.cacher.(FsCache); is {
		dir := filepath.Join(v.dir, repo)
		err := exec.CommandContext(ctx, "git", "clone", "--bare", "--depth=1", HTTPSURL(repo), dir).Run()
		if err == nil {
			return nil
		}
		err = exec.CommandContext(ctx, "git", "clone", "--bare", "--depth=1", SSHURL(repo), dir).Run()
		if err == nil {
			return nil
		}
	}

	return errors.New("Run plain command doesn't support for memory storage")
}

func (a Git) runFetchCmd(ctx context.Context, repo string, refSpec string) error {
	// Try plain command as long as it works in shell
	if v, is := a.cacher.(FsCache); is {
		cmd := exec.CommandContext(ctx, "git", "fetch", "origin", refSpec, "--prune")
		cmd.Dir = filepath.Join(v.dir, repo)
		return cmd.Run()
	}
	return errors.New("Run plain command doesn't support for memory storage")
}

// Show the content of a file with given file path and git reference in the cache directory.
func (a Git) Show(r *git.Repository, resource *retriever.Resource) ([]byte, error) {
	if !resource.Ref.IsHash() {
		err := a.ResolveReference(r, resource)
		if err != nil {
			return nil, err
		}
	}

	commit, err := r.CommitObject(plumbing.NewHash(resource.Ref.Hash().String()))
	if err != nil {
		return nil, err
	}

	f, err := commit.File(resource.Filepath)
	if err != nil {
		return nil, err
	}
	contents, err := f.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(contents), nil
}

// ResolveReference resolves a SymbolicReference to a HashReference.
func (a Git) ResolveReference(r *git.Repository, resource *retriever.Resource) (err error) {
	if resource.Ref == nil {
		resource.Ref = retriever.HEADReference()
	}

	if resource.Ref.IsHash() {
		return nil
	}

	rev := resource.Ref.Name()
	if rev == "HEAD" {
		rev = "refs/remotes/origin/HEAD"
	}
	h, err := r.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return
	}

	hash, err := retriever.NewHash(h.String())
	if err != nil {
		return
	}
	resource.Ref.SetHash(hash)
	return
}
