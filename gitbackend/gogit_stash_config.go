package gitbackend

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/config"
)

// Stash operations are not supported by the go-git backend. Git implements
// stash as a reflog on refs/stash, which the go-git storage layer cannot
// represent (earlier attempts wrote refs/stash@{N} — a name git's own
// check-ref-format rejects — creating stash entries invisible to native git
// and a pop path that could drop entries whose apply half-failed). Use the
// native backend (requires the git binary) for stash.
func stashUnsupported(op, repoPath string) error {
	return newGitError(op, repoPath, "", fmt.Errorf("%w: the go-git backend cannot represent git's stash reflog; use the native backend", ErrStashUnsupported))
}

func (b *GoGitBackend) StashList(ctx context.Context, repoPath string) ([]StashEntry, error) {
	return nil, stashUnsupported("StashList", repoPath)
}

func (b *GoGitBackend) StashSave(ctx context.Context, repoPath, message string) error {
	return stashUnsupported("StashSave", repoPath)
}

func (b *GoGitBackend) StashApply(ctx context.Context, repoPath string, stashIdx int) error {
	return stashUnsupported("StashApply", repoPath)
}

func (b *GoGitBackend) StashPop(ctx context.Context, repoPath string, stashIdx int) error {
	return stashUnsupported("StashPop", repoPath)
}

func (b *GoGitBackend) StashDrop(ctx context.Context, repoPath string, stashIdx int) error {
	return stashUnsupported("StashDrop", repoPath)
}

func (b *GoGitBackend) StashClear(ctx context.Context, repoPath string) error {
	return stashUnsupported("StashClear", repoPath)
}

// --- Config operations ---

// parseConfigKey splits a git config key like "remote.origin.url" or "core.bare"
// into (section, subsection, option). For "section.option" keys the subsection is empty.
func parseConfigKey(key string) (section, subsection, option string, err error) {
	parts := strings.Split(key, ".")
	switch len(parts) {
	case 2:
		// section.option (e.g. "core.bare")
		return parts[0], "", parts[1], nil
	case 3:
		// section.subsection.option (e.g. "remote.origin.url")
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("invalid config key: %s (expected section.option or section.subsection.option)", key)
	}
}

// GetConfig reads any git config value. Supports both simple keys (e.g.
// "core.bare") and subsection keys (e.g. "remote.origin.url"). The special
// keys "user.name" and "user.email" are handled via the high-level Author
// struct for backward compatibility. A missing key fails with an error
// wrapping ErrConfigKeyNotFound; a key explicitly set to the empty string
// returns ("", nil) — the two are distinguishable.
func (b *GoGitBackend) GetConfig(ctx context.Context, repoPath, key string) (string, error) {
	repo, err := openRepo("GetConfig", repoPath)
	if err != nil {
		return "", err
	}

	cfg, err := repo.Config()
	if err != nil {
		return "", newGitError("GetConfig", repoPath, "", err)
	}

	// Fast path for the two most common keys.
	if key == "user.name" {
		return cfg.Author.Name, nil
	}
	if key == "user.email" {
		return cfg.Author.Email, nil
	}

	section, subsection, option, err := parseConfigKey(key)
	if err != nil {
		return "", newGitError("GetConfig", repoPath, "", err)
	}

	if cfg.Raw == nil {
		return "", newGitError("GetConfig", repoPath, "", fmt.Errorf("%w: %s", ErrConfigKeyNotFound, key))
	}

	sec := cfg.Raw.Section(section)
	var val string
	var exists bool
	find := func(opts config.Options) (string, bool) {
		for _, o := range opts {
			if o.IsKey(option) {
				return o.Value, true
			}
		}
		return "", false
	}
	if subsection != "" {
		val, exists = find(sec.Subsection(subsection).Options)
	} else {
		val, exists = find(sec.Options)
	}
	if !exists {
		return "", newGitError("GetConfig", repoPath, "", fmt.Errorf("%w: %s", ErrConfigKeyNotFound, key))
	}
	return val, nil
}

// SetConfig writes any git config value. Supports both simple keys and
// subsection keys, matching the same format as GetConfig. The special keys
// "user.name" and "user.email" are written via the high-level Author struct.
func (b *GoGitBackend) SetConfig(ctx context.Context, repoPath, key, value string) error {
	repo, err := openRepo("SetConfig", repoPath)
	if err != nil {
		return err
	}

	cfg, err := repo.Config()
	if err != nil {
		return newGitError("SetConfig", repoPath, "", err)
	}

	// Fast path for the two most common keys.
	if key == "user.name" {
		cfg.Author.Name = value
		if err := repo.Storer.SetConfig(cfg); err != nil {
			return newGitError("SetConfig", repoPath, "", err)
		}
		return nil
	}
	if key == "user.email" {
		cfg.Author.Email = value
		if err := repo.Storer.SetConfig(cfg); err != nil {
			return newGitError("SetConfig", repoPath, "", err)
		}
		return nil
	}

	section, subsection, option, err := parseConfigKey(key)
	if err != nil {
		return newGitError("SetConfig", repoPath, "", err)
	}

	// Use the Raw config helpers which accept section/subsection strings
	// directly, avoiding the Section/Subsection type mismatch.
	cfg.Raw.SetOption(section, subsection, option, value)

	if err := repo.Storer.SetConfig(cfg); err != nil {
		return newGitError("SetConfig", repoPath, "", err)
	}
	return nil
}
