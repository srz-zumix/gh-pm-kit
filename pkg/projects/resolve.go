package projects

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// ResolveProject resolves a project number and its owner repository from a
// project number or project URL. An owner embedded in the URL takes precedence
// over ownerFlag.
func ResolveProject(arg string, ownerFlag string) (repository.Repository, int, error) {
	number, err := parser.GetProjectNumberFromString(arg)
	if err != nil {
		return repository.Repository{}, 0, fmt.Errorf("invalid project number or URL %q: %w", arg, err)
	}
	owner := ownerFlag
	if projectURL, _ := parser.ParseProjectURL(arg); projectURL != nil {
		owner = projectURL.Repo.Host + "/" + projectURL.Repo.Owner
	}
	repo, err := parser.Repository(parser.RepositoryOwnerWithHost(owner))
	if err != nil {
		return repository.Repository{}, 0, fmt.Errorf("failed to resolve owner: %w", err)
	}
	return repo, number, nil
}
