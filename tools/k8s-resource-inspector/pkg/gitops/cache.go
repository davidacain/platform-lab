package gitops

import (
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type cacheEntry struct {
	dir      string
	repo     *gogit.Repository
	headHash plumbing.Hash
	auth     *githttp.BasicAuth
}

// RepoCache clones each repo URL once per session and reuses the clone for
// subsequent operations on the same repo. Call Close when done to clean up.
type RepoCache struct {
	token   string
	entries map[string]*cacheEntry
}

// NewRepoCache creates a cache that authenticates clones with the given token.
func NewRepoCache(token string) *RepoCache {
	return &RepoCache{token: token, entries: make(map[string]*cacheEntry)}
}

// get returns the cached clone for repoURL, cloning on first access.
// On subsequent calls the worktree is reset to the original HEAD so the next
// branch starts from a clean base.
func (c *RepoCache) get(repoURL string) (*cacheEntry, error) {
	if e, ok := c.entries[repoURL]; ok {
		w, err := e.repo.Worktree()
		if err != nil {
			return nil, fmt.Errorf("get worktree: %w", err)
		}
		if err := w.Checkout(&gogit.CheckoutOptions{Hash: e.headHash, Force: true}); err != nil {
			return nil, fmt.Errorf("reset to HEAD: %w", err)
		}
		return e, nil
	}

	tmpDir, err := os.MkdirTemp("", "kri-push-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	auth := &githttp.BasicAuth{Username: "x-oauth-token", Password: c.token}
	repo, err := gogit.PlainClone(tmpDir, false, &gogit.CloneOptions{
		URL:   repoURL,
		Auth:  auth,
		Depth: 1,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("clone %s: %w", repoURL, err)
	}

	head, err := repo.Head()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}

	e := &cacheEntry{dir: tmpDir, repo: repo, headHash: head.Hash(), auth: auth}
	c.entries[repoURL] = e
	return e, nil
}

// Close removes all temporary clone directories.
func (c *RepoCache) Close() {
	for _, e := range c.entries {
		os.RemoveAll(e.dir)
	}
}
