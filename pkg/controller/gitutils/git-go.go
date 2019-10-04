package gitutils

import (
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
	//oapi "github.com/openshift/api"
)

//HandleRepo returns a slice of objects from a repo, if all are handled.
func HandleRepo(str string) (*object.FileIter, error) {
	r, err := cloneRepo(str)
	if err != nil {
		return nil, err
	}
	tree, err := retrieveTreeFromRef(r)
	if err != nil {
		return nil, err
	}
	return tree.Files(), nil
}

func cloneRepo(url string) (*git.Repository, error) {
	r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL: url,
	})
	return r, err
}

//setReference retrieves only HEAD reference for now
func retrieveTreeFromRef(r *git.Repository) (*object.Tree, error) {
	ref, err := r.Head()
	if err != nil {
		return nil, err
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	return tree, err
}
