package gitutils

import (
	"strings"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

//HandleRepo returns a ref, object tree, if all are handled. Error otherwise
func HandleRepo(url string, branch string) (*plumbing.Reference, *object.Tree, error) {
	r, err := cloneRepo(url, branch)
	if err != nil {
		return nil, nil, err
	}
	ref, err := r.Head()
	if err != nil {
		return nil, nil, err
	}
	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, nil, err
	}
	return ref, tree, err
}

func cloneRepo(url string, branch string) (*git.Repository, error) {
	var refName plumbing.ReferenceName
	if strings.ToLower(branch) == "master" {
		refName = plumbing.Master
	} else {
		refName = plumbing.NewBranchReferenceName(branch)
	}
	r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:           url,
		ReferenceName: refName,
	})
	return r, err
}
