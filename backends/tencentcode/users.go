package tencentcode

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/yi-nology/go-git-platform/model"
	"github.com/yi-nology/go-git-platform/provider"
)

// resolveUserIDs resolves usernames to 工蜂 user IDs through the Users API
// (GET /users/{username} — the endpoint accepts either an ID or a username),
// memoized in a per-provider TTL cache. op is the public operation the
// resolution serves; an unknown username surfaces as a NotFound under that
// op, and resolution failures stop the write before it reaches the wire.
func (p *Provider) resolveUserIDs(ctx context.Context, op string, usernames []string) ([]int, error) {
	ids := make([]int, 0, len(usernames))
	for _, name := range usernames {
		if id, ok := p.userIDs.Get(name); ok {
			ids = append(ids, int(id))
			continue
		}
		u, _, err := p.client.Users.GetUser(ctx, name)
		if err != nil {
			// Wrap classifies a 404 lookup as NotFound, so an unknown
			// username keeps the unified "user not found" shape.
			return nil, provider.Wrap(model.PlatformTencentCode, op, err)
		}
		if u == nil || u.ID == 0 {
			return nil, provider.New(model.PlatformTencentCode, op, http.StatusNotFound,
				"user "+strconv.Quote(name)+" not found")
		}
		p.userIDs.Put(name, int64(u.ID))
		ids = append(ids, u.ID)
	}
	return ids, nil
}

// assigneeIDsCSV renders resolved user IDs as the csv string 工蜂's
// assignee_ids field carries on the wire.
func assigneeIDsCSV(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// GetUser implements provider.UserManager.
func (p *Provider) GetUser(ctx context.Context, username string) (*model.CRUser, error) {
	u, _, err := p.client.Users.GetUser(ctx, username)
	if err != nil {
		return nil, provider.Wrap(model.PlatformTencentCode, "GetUser", err)
	}
	if u == nil || u.ID == 0 {
		return nil, provider.New(model.PlatformTencentCode, "GetUser", http.StatusNotFound,
			"user "+strconv.Quote(username)+" not found")
	}
	return &model.CRUser{
		ID:       int64(u.ID),
		Username: u.Username,
		Name:     u.Name,
	}, nil
}

// ResolveUsernames implements provider.UserManager.
func (p *Provider) ResolveUsernames(ctx context.Context, usernames []string) ([]*model.CRUser, error) {
	ids, err := p.resolveUserIDs(ctx, "ResolveUsernames", usernames)
	if err != nil {
		return nil, err
	}
	result := make([]*model.CRUser, len(usernames))
	for i, name := range usernames {
		result[i] = &model.CRUser{ID: int64(ids[i]), Username: name}
	}
	return result, nil
}

var _ provider.UserManager = (*Provider)(nil)
