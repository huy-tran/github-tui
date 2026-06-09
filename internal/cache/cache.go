// Package cache persists the repository list to disk so the first screen can
// render instantly while fresh data is fetched in the background.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/huy-tran/github-tui/internal/gh"
)

// repoCache is the on-disk shape.
type repoCache struct {
	SavedAt time.Time `json:"savedAt"`
	Repos   []gh.Repo `json:"repos"`
}

// reposPath returns the cache file location under the user cache dir.
func reposPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-tui", "repos.json"), nil
}

// ReadRepos returns the cached repositories and when they were saved. A missing
// or unreadable cache returns nil with no error (a miss is not exceptional).
func ReadRepos() ([]gh.Repo, time.Time, error) {
	path, err := reposPath()
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, nil // cache miss
	}
	var c repoCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, time.Time{}, nil // corrupt cache: treat as miss
	}
	return c.Repos, c.SavedAt, nil
}

// vulnCache is the on-disk shape for per-repo vulnerability counts.
type vulnCache struct {
	SavedAt time.Time                `json:"savedAt"`
	Counts  map[string]gh.VulnCounts `json:"counts"`
}

func vulnsPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-tui", "vulns.json"), nil
}

// ReadVulns returns the cached per-repo alert counts and when they were saved.
// A missing or unreadable cache returns nil with no error.
func ReadVulns() (map[string]gh.VulnCounts, time.Time, error) {
	path, err := vulnsPath()
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, nil
	}
	var c vulnCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, time.Time{}, nil
	}
	return c.Counts, c.SavedAt, nil
}

// WriteVulns persists per-repo alert counts with the current timestamp.
func WriteVulns(counts map[string]gh.VulnCounts) error {
	path, err := vulnsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(vulnCache{SavedAt: time.Now(), Counts: counts})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// lastCommitCache is the on-disk shape for per-repo last-committer info.
type lastCommitCache struct {
	SavedAt time.Time                `json:"savedAt"`
	Commits map[string]gh.LastCommit `json:"commits"`
}

func lastCommitsPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-tui", "commits.json"), nil
}

// ReadLastCommits returns the cached per-repo last-commit info and when it was
// saved. A missing or unreadable cache returns nil with no error.
func ReadLastCommits() (map[string]gh.LastCommit, time.Time, error) {
	path, err := lastCommitsPath()
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, nil
	}
	var c lastCommitCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, time.Time{}, nil
	}
	return c.Commits, c.SavedAt, nil
}

// WriteLastCommits persists per-repo last-commit info with the current timestamp.
func WriteLastCommits(commits map[string]gh.LastCommit) error {
	path, err := lastCommitsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(lastCommitCache{SavedAt: time.Now(), Commits: commits})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// WriteRepos persists the repository list with the current timestamp.
func WriteRepos(repos []gh.Repo) error {
	path, err := reposPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(repoCache{SavedAt: time.Now(), Repos: repos})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
