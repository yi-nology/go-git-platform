package gitbackend

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// --- Remote operations ---

func (b *GoGitBackend) AddRemote(ctx context.Context, repoPath, name, url string) error {
	repo, err := openRepo("AddRemote", repoPath)
	if err != nil {
		return err
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return newGitError("AddRemote", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) RemoveRemote(ctx context.Context, repoPath, name string) error {
	repo, err := openRepo("RemoveRemote", repoPath)
	if err != nil {
		return err
	}

	err = repo.DeleteRemote(name)
	if err != nil {
		return newGitError("RemoveRemote", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) GetRemoteURL(ctx context.Context, repoPath, name string) (string, error) {
	repo, err := openRepo("GetRemoteURL", repoPath)
	if err != nil {
		return "", err
	}

	remote, err := repo.Remote(name)
	if err != nil {
		return "", newGitError("GetRemoteURL", repoPath, "", ErrRemoteNotFound)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", newGitError("GetRemoteURL", repoPath, "", fmt.Errorf("no URLs for remote %s", name))
	}
	return urls[0], nil
}

func (b *GoGitBackend) GetRemotes(ctx context.Context, repoPath string) ([]string, error) {
	repo, err := openRepo("GetRemotes", repoPath)
	if err != nil {
		return nil, err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return nil, newGitError("GetRemotes", repoPath, "", err)
	}
	var names []string
	for _, r := range remotes {
		names = append(names, r.Config().Name)
	}
	return names, nil
}

// --- Tag operations ---

func (b *GoGitBackend) CreateTag(ctx context.Context, repoPath, name, ref string) error {
	repo, err := openRepo("CreateTag", repoPath)
	if err != nil {
		return err
	}

	var hash plumbing.Hash
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return newGitError("CreateTag", repoPath, "", err)
		}
		hash = head.Hash()
	} else {
		// Resolve any rev (branch, tag, HEAD, hash...). plumbing.NewHash
		// alone used to yield a zero hash for branch names, silently tagging
		// HEAD instead of the requested ref.
		hash, err = resolveRev(repo, ref)
		if err != nil {
			return newGitError("CreateTag", repoPath, "", err)
		}
	}

	if _, err = repo.CreateTag(name, hash, nil); err != nil {
		if errors.Is(err, git.ErrTagExists) {
			return newGitError("CreateTag", repoPath, "", ErrTagExists)
		}
		return newGitError("CreateTag", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) DeleteTag(ctx context.Context, repoPath, name string) error {
	repo, err := openRepo("DeleteTag", repoPath)
	if err != nil {
		return err
	}

	err = repo.DeleteTag(name)
	if err != nil {
		return newGitError("DeleteTag", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) PushTag(ctx context.Context, repoPath, remote, name string, auth AuthConfig) error {
	repo, err := openRepo("PushTag", repoPath)
	if err != nil {
		return err
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", name, name))
	am, err := buildTransportAuth(auth)
	if err != nil {
		return newGitError("PushTag", repoPath, "", err)
	}
	pushOpts := &git.PushOptions{
		RemoteName:      remote,
		RefSpecs:        []config.RefSpec{refSpec},
		Auth:            am,
		InsecureSkipTLS: auth.InsecureSkipTLS,
	}

	err = repo.PushContext(ctx, pushOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return newGitError("PushTag", repoPath, "", err)
	}
	return nil
}

func (b *GoGitBackend) GetTagList(ctx context.Context, repoPath string) ([]TagInfo, error) {
	repo, err := openRepo("GetTagList", repoPath)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Tags()
	if err != nil {
		return nil, newGitError("GetTagList", repoPath, "", err)
	}

	var tags []TagInfo
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		tagObj, tagErr := repo.TagObject(ref.Hash())
		if tagErr == nil {
			// Annotated Tag
			tags = append(tags, TagInfo{
				Name:    ref.Name().Short(),
				Hash:    ref.Hash().String(),
				Message: tagObj.Message,
				Author:  tagObj.Tagger.Name,
			})
		} else {
			// Lightweight Tag (commit)
			commit, commitErr := repo.CommitObject(ref.Hash())
			if commitErr == nil {
				tags = append(tags, TagInfo{
					Name:    ref.Name().Short(),
					Hash:    ref.Hash().String(),
					Message: commit.Message,
					Author:  commit.Author.Name,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, newGitError("GetTagList", repoPath, "", err)
	}
	return tags, nil
}
