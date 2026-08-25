package meridian

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultGitTimeout = 15 * time.Second

// RepositoryState is the revision and worktree state used by staging policy.
type RepositoryState struct {
	Revision string
	Dirty    bool
}

// RepositoryStateReader reads repository state without changing the checkout.
type RepositoryStateReader interface {
	Read(ctx context.Context, root string) (RepositoryState, error)
}

// GitRepositoryStateReader reads repository state through fixed Git commands.
type GitRepositoryStateReader struct {
	Executable string
	Timeout    time.Duration
}

// Read returns the current revision and whether tracked or untracked changes exist.
func (reader GitRepositoryStateReader) Read(ctx context.Context, root string) (RepositoryState, error) {
	executable := reader.Executable
	if executable == "" {
		executable = "git.exe"
	}
	timeout := reader.Timeout
	if timeout <= 0 {
		timeout = defaultGitTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	revision, err := exec.CommandContext(commandCtx, executable, "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return RepositoryState{}, fmt.Errorf("read repository revision: %w", err)
	}
	if commandCtx.Err() != nil {
		return RepositoryState{}, fmt.Errorf("read repository revision timed out")
	}
	commandCtx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	status, err := exec.CommandContext(commandCtx, executable, "-C", root, "status", "--porcelain=v1", "--untracked-files=normal").Output()
	if err != nil {
		return RepositoryState{}, fmt.Errorf("read repository status: %w", err)
	}
	if commandCtx.Err() != nil {
		return RepositoryState{}, fmt.Errorf("read repository status timed out")
	}
	return RepositoryState{Revision: strings.TrimSpace(string(revision)), Dirty: len(status) != 0}, nil
}

func normalizeRepository(repository Repository) (Repository, error) {
	if repository.Identity == "" || repository.Root == "" || repository.DME == "" || len(repository.Targets) == 0 {
		return Repository{}, fmt.Errorf("repository configuration is invalid")
	}
	root, err := filepath.Abs(repository.Root)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve repository root: %w", err)
	}
	repository.Root = filepath.Clean(root)
	if repository.DMEPath == "" {
		repository.DMEPath = repository.DME
	}
	if _, err := resolveContained(repository.Root, repository.DMEPath); err != nil {
		return Repository{}, err
	}
	targets := make(map[string]string, len(repository.Targets))
	for id, target := range repository.Targets {
		if id == "" {
			return Repository{}, fmt.Errorf("repository target identifier is empty")
		}
		if _, err := resolveContained(repository.Root, target); err != nil {
			return Repository{}, err
		}
		targets[id] = target
	}
	repository.Targets = targets
	return repository, nil
}

func canonicalExistingDirectory(path, label string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	return filepath.Clean(resolved), nil
}
