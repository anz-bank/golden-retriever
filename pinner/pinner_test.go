package pinner

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anz-bank/golden-retriever/retriever"
	"github.com/anz-bank/golden-retriever/retriever/mock"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	_, err := New("tmp_modules.yaml", nil)
	require.Error(t, err)

	_, err = New("", &mock.Retriever{})
	require.NoError(t, err)

	_, err = New("tmp_modules.yaml", &mock.Retriever{})
	require.NoError(t, err)
}

func TestPinnerRetrieve(t *testing.T) {
	retr := &mock.Retriever{}

	h, err := retriever.NewHash("433416d690dbffc8fe321e12bdd4f21d79e2a479")
	require.NoError(t, err)

	tests := []struct {
		refname string
		refhash retriever.Hash
		content []byte
		hash    retriever.Hash
	}{
		{"", retriever.ZeroHash, retr.HEADContent(), retr.HEADHash()},
		{retriever.HEAD, retriever.ZeroHash, retr.HEADContent(), retr.HEADHash()},
		{"master", retriever.ZeroHash, retr.BranchContent(), retr.BranchHash()},
		{"v1", retriever.ZeroHash, retr.TagContent(), retr.TagHash()},
		{"", h, retr.HashContent(), h},
	}

	for i, test := range tests {
		s := test.refhash.String()
		if test.refhash.IsZero() {
			s = "nohash"
		}
		t.Run(test.refname+s, func(t *testing.T) {
			modFile := fmt.Sprintf("tmp_modules%d.yaml", i)
			pinner, err := New(modFile, retr)
			require.NoError(t, err)

			ref, err := retriever.NewReference(test.refname, test.refhash)
			require.NoError(t, err)

			resource := &retriever.Resource{
				Repo:     "github.com/foo/bar",
				Filepath: "baz.md",
				Ref:      ref,
			}
			c, err := pinner.Retrieve(context.Background(), resource)
			require.NoError(t, err)
			require.Equal(t, test.content, c)
			require.Equal(t, test.hash, resource.Ref.Hash())

			if test.refhash.IsZero() {
				err = os.Remove(modFile)
				require.NoError(t, err)
			}
		})
	}
}

func TestPinnerRetrieveModFile(t *testing.T) {
	retr := &mock.Retriever{}
	modFile := "tmp_modules.yaml"
	defer func() {
		err := os.Remove(modFile)
		require.NoError(t, err)
	}()

	pinner, err := New(modFile, retr)
	require.NoError(t, err)

	h, err := retriever.NewHash("433416d690dbffc8fe321e12bdd4f21d79e2a479")
	require.NoError(t, err)

	tests := []struct {
		refname string
		refhash retriever.Hash
		name    string
		hash    retriever.Hash
	}{
		{"", retriever.ZeroHash, "master", retr.HEADHash()},
		{retriever.HEAD, retriever.ZeroHash, "master", retr.HEADHash()},
		{"master", retriever.ZeroHash, "master", retr.HEADHash()},
		{"", h, "", h},
	}

	resource := &retriever.Resource{
		Repo:     "github.com/foo/bar",
		Filepath: "baz.md",
		Ref:      nil,
	}
	_, err = pinner.Retrieve(context.Background(), resource)
	require.NoError(t, err)
	require.Equal(t, retr.HEADHash(), resource.Ref.Hash())
	require.Equal(t, "master", resource.Ref.Name())
	b, err := ioutil.ReadFile(modFile)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("imports:\n    github.com/foo/bar:\n        ref: %s\n        pinned: %s\n", "master", retr.HEADHash()), string(b))

	ref, err := retriever.NewReference("v1", retriever.ZeroHash)
	require.NoError(t, err)
	resource = &retriever.Resource{
		Repo:     "github.com/foo/bar",
		Filepath: "baz.md",
		Ref:      ref,
	}
	_, err = pinner.Retrieve(context.Background(), resource)
	require.EqualError(t, err, "cannot import multiple versions (v1, master) of a single repo github.com/foo/bar")
	b, err = ioutil.ReadFile(modFile)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("imports:\n    github.com/foo/bar:\n        ref: %s\n        pinned: %s\n", "master", retr.HEADHash()), string(b))

	for _, test := range tests {
		s := test.refhash.String()
		if test.refhash.IsZero() {
			s = "nohash"
		}
		t.Run(test.refname+s, func(t *testing.T) {
			ref, err = retriever.NewReference(test.refname, test.refhash)
			require.NoError(t, err)

			resource = &retriever.Resource{
				Repo:     "github.com/foo/bar",
				Filepath: "baz.md",
				Ref:      ref,
			}
			_, err = pinner.Retrieve(context.Background(), resource)
			require.NoError(t, err)
			require.Equal(t, test.name, resource.Ref.Name())
			require.Equal(t, test.hash, resource.Ref.Hash())

			b, err = ioutil.ReadFile(modFile)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("imports:\n    github.com/foo/bar:\n        ref: %s\n        pinned: %s\n", "master", retr.HEADHash()), string(b))
		})
	}
}
