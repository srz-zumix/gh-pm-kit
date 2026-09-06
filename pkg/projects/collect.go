package projects

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// CollectedProject bundles every element of a single project that the audit
// commands (stats, lint) need, so that they share a single fetch pass.
type CollectedProject struct {
	Host          string
	Owner         string
	Number        int
	Project       *gh.ProjectV2
	Fields        []gh.ProjectV2Field
	Items         []gh.ProjectV2Item
	Views         []gh.ProjectV2View
	StatusUpdates []gh.ProjectV2StatusUpdate
}

// Label builds a human readable identifier for the collected project.
func (c *CollectedProject) Label() string {
	return projectLabel(c.Host, c.Owner, c.Number)
}

// FindField returns the field definition matching name, ignoring case.
func (c *CollectedProject) FindField(name string) *gh.ProjectV2Field {
	for i := range c.Fields {
		if strings.EqualFold(c.Fields[i].Name, name) {
			return &c.Fields[i]
		}
	}
	return nil
}

// CollectProject fetches the metadata, fields, items, views and status updates
// of a single project.
func CollectProject(ctx context.Context, g *gh.GitHubClient, repo repository.Repository, number int) (*CollectedProject, error) {
	project, err := gh.GetProjectV2ByNumber(ctx, g, repo.Owner, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get project #%d of '%s': %w", number, repo.Owner, err)
	}
	fields, err := gh.ListProjectV2Fields(ctx, g, repo.Owner, number)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for project #%d of '%s': %w", number, repo.Owner, err)
	}
	items, err := gh.ListProjectV2Items(ctx, g, repo.Owner, number)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for project #%d of '%s': %w", number, repo.Owner, err)
	}
	views, err := gh.ListProjectV2Views(ctx, g, repo.Owner, number)
	if err != nil {
		return nil, fmt.Errorf("failed to list views for project #%d of '%s': %w", number, repo.Owner, err)
	}
	statusUpdates, err := gh.ListProjectV2StatusUpdates(ctx, g, repo.Owner, number)
	if err != nil {
		return nil, fmt.Errorf("failed to list status updates for project #%d of '%s': %w", number, repo.Owner, err)
	}

	return &CollectedProject{
		Host:          repo.Host,
		Owner:         repo.Owner,
		Number:        number,
		Project:       project,
		Fields:        fields,
		Items:         items,
		Views:         views,
		StatusUpdates: statusUpdates,
	}, nil
}
