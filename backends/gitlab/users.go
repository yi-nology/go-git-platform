package gitlab

import (
	"context"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// resolveUserIDs resolves usernames to GitLab user IDs using ListUsers with
// the exact-match username filter, memoized in a per-provider TTL cache.
// op is the public operation the resolution serves; an unknown username
// surfaces as a NotFound under that op.
func (p *Provider) resolveUserIDs(ctx context.Context, op string, usernames []string) ([]int64, error) {
	ids := make([]int64, 0, len(usernames))
	for _, name := range usernames {
		if id, ok := p.userIDs.Get(name); ok {
			ids = append(ids, id)
			continue
		}
		users, _, err := p.client.Users.ListUsers(
			&gitlab.ListUsersOptions{Username: new(name)},
			gitlab.WithContext(ctx))
		if err != nil {
			return nil, provider.Wrap(model.PlatformGitLab, op, err)
		}
		var (
			id    int64
			found bool
		)
		for _, u := range users {
			if u != nil && u.Username == name {
				id, found = u.ID, true
				break
			}
		}
		if !found {
			return nil, provider.New(model.PlatformGitLab, op, http.StatusNotFound, fmt.Sprintf("user %q not found", name))
		}
		p.userIDs.Put(name, id)
		ids = append(ids, id)
	}
	return ids, nil
}

// GetUser implements provider.UserManager.
func (p *Provider) GetUser(ctx context.Context, username string) (*model.CRUser, error) {
	users, _, err := p.client.Users.ListUsers(
		&gitlab.ListUsersOptions{Username: new(username)},
		gitlab.WithContext(ctx))
	if err != nil {
		return nil, provider.Wrap(model.PlatformGitLab, "GetUser", err)
	}
	for _, u := range users {
		if u != nil && u.Username == username {
			return &model.CRUser{
				ID:        u.ID,
				Username:  u.Username,
				Name:      u.Name,
				AvatarURL: u.AvatarURL,
			}, nil
		}
	}
	return nil, provider.New(model.PlatformGitLab, "GetUser", http.StatusNotFound, fmt.Sprintf("user %q not found", username))
}

// ResolveUsernames implements provider.UserManager.
func (p *Provider) ResolveUsernames(ctx context.Context, usernames []string) ([]*model.CRUser, error) {
	ids, err := p.resolveUserIDs(ctx, "ResolveUsernames", usernames)
	if err != nil {
		return nil, err
	}
	result := make([]*model.CRUser, len(usernames))
	for i, name := range usernames {
		result[i] = &model.CRUser{ID: ids[i], Username: name}
	}
	return result, nil
}

var _ provider.UserManager = (*Provider)(nil)
