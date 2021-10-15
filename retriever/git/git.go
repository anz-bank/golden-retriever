package git

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anz-bank/golden-retriever/retriever"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.WarnLevel)
}

func isReferenceNotFoundErr(err error) bool {
	return nomatchspecErr.Is(err) || plumbing.ErrReferenceNotFound == err
}

var nomatchspecErr = git.NoMatchingRefSpecError{}

// Clone a repository into the given cache directory.
func (a Git) Clone(ctx context.Context, resource *retriever.Resource) (r *git.Repository, err error) {
	repo := resource.Repo
	c, isPlain := a.cacher.(PlainFsCache)

	if resource.Ref.IsHash() {
		if isPlain {
			r, err = git.PlainInit(c.RepoDir(repo), false)
		} else {
			r, err = git.Init(a.cacher.NewStorer(repo), nil)
		}
		if err != nil {
			return nil, err
		}

		err = a.FetchCommit(ctx, r, repo, resource.Ref.Hash())
		return
	}

	tried := []string{}

	for _, meth := range a.authMethods {
		auth, url := meth.AuthMethod(repo)
		options := &git.CloneOptions{
			URL:   url,
			Depth: 1,
			Auth:  auth,
		}

		ref := resource.Ref.Name()
		rules := retriever.RefRules
		if ref == "HEAD" {
			ref = "heads"
			rules = []string{"refs/%s/master", "refs/%s/main"}
		}

		for iter := retriever.NewRefIterator(rules, ref); iter.Next(); {
			options.ReferenceName = plumbing.ReferenceName(iter.Current())

			if isPlain {
				r, err = git.PlainCloneContext(ctx, c.RepoDir(repo), false, options)
			} else {
				r, err = git.CloneContext(ctx, a.cacher.NewStorer(repo), memfs.New(), options)
			}
			if err == nil {
				return r, nil
			}
		}

		errmsg := err.Error()
		if isReferenceNotFoundErr(err) {
			errmsg = fmt.Sprintf("reference %s not found", ref)
		}
		if transport.ErrRepositoryNotFound == err {
			errmsg = fmt.Sprintf("repository %s not found", repo)
		}
		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), errmsg))
	}

	return nil, fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
}

func (a Git) Fetch(ctx context.Context, r *git.Repository, resource *retriever.Resource) error {
	if resource.Ref.IsHash() {
		return a.FetchCommit(ctx, r, resource.Repo, resource.Ref.Hash())
	}
	return a.FetchRef(ctx, r, resource.Repo, resource.Ref.Name())
}

// FetchRef fetches specific reference
func (a Git) FetchRef(ctx context.Context, r *git.Repository, repo string, ref string) (err error) {
	options := &git.FetchOptions{
		Depth:    1,
		Progress: os.Stdout,
	}

	tried := []string{}
	for _, meth := range a.authMethods {
		auth, _ := meth.AuthMethod(repo)
		options.Auth = auth

		for iter := retriever.NewRefIterator(retriever.RefRules, ref); iter.Next(); {
			refSpec := iter.Current()
			if refSpec == "HEAD" {
				refSpec = fmt.Sprintf("+%s:refs/remotes/origin/%[1]s", "HEAD")
			} else {
				refSpec = fmt.Sprintf("+%s:%[1]s", refSpec)
			}
			options.RefSpecs = []config.RefSpec{config.RefSpec(refSpec)}

			err = r.FetchContext(ctx, options)
			if err == nil || err == git.NoErrAlreadyUpToDate {
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

	refSpec := fmt.Sprintf("%s:%[1]s", hash)
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
				if err != nil && err != git.ErrRemoteNotFound {
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
				}); err != nil && err != git.ErrRemoteExists {
					return err
				}
			}
		}

		err = r.FetchContext(ctx, options)
		if err == nil || err == git.NoErrAlreadyUpToDate {
			return nil
		}

		tried = append(tried, fmt.Sprintf("    - %s: %s", meth.Name(), err.Error()))
	}

	return fmt.Errorf("Unable to authenticate, tried: \n%s", strings.Join(tried, ",\n"))
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
		if err == plumbing.ErrObjectNotFound {
			return nil, fmt.Errorf("object of commit %s not found", resource.Ref.Hash())
		}
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

	var h *plumbing.Hash
	rev := resource.Ref.Name()
	if rev == "HEAD" {
		ref, e := r.Reference("HEAD", false)
		if e == nil {
			resource.Ref.SetName(strings.TrimPrefix(ref.Target().String(), "refs/heads/"))
		}
		h, err = r.ResolveRevision(plumbing.Revision("refs/remotes/origin/HEAD"))
	}

	if err != nil || rev != "HEAD" {
		h, err = r.ResolveRevision(plumbing.Revision(rev))
		if err != nil {
			if err == plumbing.ErrReferenceNotFound {
				return fmt.Errorf("reference %s not found", rev)
			}
			return
		}
	}

	hash, err := retriever.NewHash(h.String())
	if err != nil {
		return
	}
	resource.Ref.SetHash(hash)
	return
}
