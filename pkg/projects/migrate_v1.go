// Package projects provides GitHub Project v1 (classic) to v2 migration logic for gh-pm-kit.
package projects

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// MigrateV1Options controls migration behaviour for GitHub Projects (classic) v1 → v2.
type MigrateV1Options struct {
	// Overwrite skips the idempotency check and re-creates items in an existing destination project.
	Overwrite bool
	// Prune deletes ALL destination v2 projects carrying the source-v1 migration marker before migrating.
	Prune bool
}

// v1ProjectMarker returns the HTML comment marker embedded in a migrated v2 project readme
// to identify the v1 migration source and enable idempotent re-runs.
// The key includes host, owner, repo, and project number to avoid collisions across hosts
// and between org-level vs repo-level classic projects.
func v1ProjectMarker(srcHost, srcOwner, srcRepoName string, srcProjectNumber int) string {
	projectKey := fmt.Sprintf("v1:%s:%s/%s#%d", srcHost, srcOwner, srcRepoName, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-v1-project:%s -->", projectHash)
}

// v1ItemMarker returns the HTML comment marker embedded in a migrated draft-issue body
// to identify the v1 card source and enable idempotent per-card re-runs.
// The key includes host, owner, repo, and project number to avoid collisions across hosts
// and between org-level vs repo-level classic projects.
func v1ItemMarker(srcHost, srcOwner, srcRepoName string, srcProjectNumber int, cardID int64) string {
	projectKey := fmt.Sprintf("v1:%s:%s/%s#%d", srcHost, srcOwner, srcRepoName, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	cardHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d", cardID))))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-v1-project-item:%s/%s -->", projectHash, cardHash)
}

// columnFieldName is the name of the SINGLE_SELECT field created in the v2 project to represent v1 columns.
const columnFieldName = "Column"

// MigrateProjectV1ToV2 migrates a GitHub Project (classic) to a new GitHub Projects v2 project.
// It creates the destination project, adds a Column single-select field for each source column,
// and migrates all cards as draft-issue items.
// A migration marker is embedded in the destination project readme for idempotent re-runs.
func MigrateProjectV1ToV2(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, srcRepoName, dstOwner string, srcNumber int, opts *MigrateV1Options) (*gh.ProjectV2, error) {
	srcRepo := repository.Repository{Owner: srcOwner, Name: srcRepoName}
	srcProject, err := gh.GetProjectV1ByNumber(ctx, src, srcRepo, srcNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source classic project #%d for '%s': %w", srcNumber, srcOwner, err)
	}

	srcColumns, err := src.ListProjectV1Columns(ctx, srcProject.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns for classic project #%d of '%s': %w", srcNumber, srcOwner, err)
	}

	// Fetch all cards per column upfront.
	cardsByColumn := make([][]gh.ProjectV1Card, len(srcColumns))
	for i, col := range srcColumns {
		cards, err := src.ListProjectV1Cards(ctx, col.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list cards for column '%s' (id=%d): %w", col.Name, col.ID, err)
		}
		cardsByColumn[i] = cards
	}

	marker := v1ProjectMarker(srcHost, srcOwner, srcRepoName, srcNumber)
	dstProjects, err := gh.ListProjectsV2(ctx, dst, dstOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to list destination projects for '%s': %w", dstOwner, err)
	}

	if opts != nil && opts.Prune {
		for _, p := range findAllProjectsByMarker(dstProjects, marker) {
			if err := gh.DeleteProjectV2(ctx, dst, p.ID); err != nil {
				return nil, fmt.Errorf("failed to delete destination project '%s' during prune: %w", p.Title, err)
			}
			logger.Info("pruned previously migrated project", "title", p.Title, "projectID", p.ID)
		}
	} else if prev := findProjectByMarker(dstProjects, marker); prev != nil {
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated v1 project", "title", prev.Title, "projectID", prev.ID)
			return prev, nil
		}
		// Overwrite: update items in the existing destination project.
	return migrateV1ItemsInto(ctx, dst, srcHost, srcOwner, srcRepoName, dstOwner, srcNumber, srcColumns, cardsByColumn, prev, opts)
	}

	// Create a new v2 project.
	dstOwnerID, err := gh.GetOwnerNodeID(ctx, dst, dstOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to get node ID for destination owner '%s': %w", dstOwner, err)
	}
	dstProject, err := gh.CreateProjectV2(ctx, dst, *dstOwnerID, srcProject.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination project '%s' for '%s': %w", srcProject.Name, dstOwner, err)
	}

	// Embed the migration marker and source body in the readme.
	readme := embedMarker(srcProject.Body, marker)
	if _, err := gh.UpdateProjectV2Metadata(ctx, dst, dstProject.ID, nil, &readme, nil); err != nil {
		return dstProject, fmt.Errorf("failed to update readme for destination project: %w", err)
	}

	// Create the Column single-select field with an option per source column.
	// The GitHub API requires a non-empty color for each single-select option; use GRAY as the default.
	columnOptions := make([]gh.ProjectV2SingleSelectOption, len(srcColumns))
	for i, col := range srcColumns {
		columnOptions[i] = gh.ProjectV2SingleSelectOption{Name: col.Name, Color: "GRAY"}
	}
	if err := gh.CreateProjectV2Field(ctx, dst, dstProject.ID, "SINGLE_SELECT", columnFieldName, columnOptions); err != nil {
		return dstProject, fmt.Errorf("failed to create '%s' field in destination project: %w", columnFieldName, err)
	}

	return migrateV1ItemsInto(ctx, dst, srcHost, srcOwner, srcRepoName, dstOwner, srcNumber, srcColumns, cardsByColumn, dstProject, opts)
}

// migrateV1ItemsInto migrates all v1 cards into an existing v2 destination project.
func migrateV1ItemsInto(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner, srcRepoName, dstOwner string, srcNumber int, srcColumns []gh.ProjectV1Column, cardsByColumn [][]gh.ProjectV1Card, dstProject *gh.ProjectV2, opts *MigrateV1Options) (*gh.ProjectV2, error) {
	// Resolve the Column field ID and its option map (name → option ID).
	dstFields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return dstProject, fmt.Errorf("failed to list fields for destination project #%d: %w", dstProject.Number, err)
	}
	var columnFieldID string
	columnOptionIDByName := make(map[string]string)
	for _, f := range dstFields {
		if f.Name == columnFieldName && f.DataType == "SINGLE_SELECT" {
			columnFieldID = f.ID
			for _, opt := range f.Options {
				columnOptionIDByName[opt.Name] = opt.ID
			}
			break
		}
	}
	if columnFieldID == "" {
		logger.Warn("destination project is missing the expected SINGLE_SELECT field; column values will not be set",
			"field", columnFieldName, "projectNumber", dstProject.Number)
	}

	// Fetch existing items in the destination project for idempotency checks.
	existingItems, err := gh.ListProjectV2Items(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return dstProject, fmt.Errorf("failed to list items in destination project #%d: %w", dstProject.Number, err)
	}

	for i, col := range srcColumns {
		optionID := columnOptionIDByName[col.Name]
		if columnFieldID != "" && optionID == "" {
			logger.Warn("no matching option found for source column; column value will not be set for cards in this column",
				"column", col.Name, "projectNumber", dstProject.Number)
		}
		for _, card := range cardsByColumn[i] {
			itemMarker := v1ItemMarker(srcHost, srcOwner, srcRepoName, srcNumber, card.ID)

			// Check whether this card was already migrated.
			if existing := findItemByMarker(existingItems, itemMarker); existing != nil {
				if opts == nil || !opts.Overwrite {
					logger.Info("skipping already-migrated v1 card", "cardID", card.ID, "column", col.Name)
					continue
				}
				// Overwrite: delete the existing item before re-creating it.
				if err := gh.DeleteProjectV2Item(ctx, dst, dstProject.ID, existing.ID); err != nil {
					return dstProject, fmt.Errorf("failed to delete existing item %s for card %d: %w", existing.ID, card.ID, err)
				}
				// Remove the deleted item from the cache to keep it consistent.
				for j, it := range existingItems {
					if it.ID == existing.ID {
						existingItems = append(existingItems[:j], existingItems[j+1:]...)
						break
					}
				}
			}

			// Derive title and body from the card note.
			title, body := cardTitleAndBody(card)
			body = embedMarker(body, itemMarker)

			itemID, err := gh.AddProjectV2DraftIssue(ctx, dst, dstProject.ID, title, body)
			if err != nil {
				return dstProject, fmt.Errorf("failed to add draft issue for card %d in column '%s': %w", card.ID, col.Name, err)
			}
			logger.Info("migrated v1 card", "cardID", card.ID, "column", col.Name, "itemID", itemID)

			// Set the Column field to the source column name.
			if columnFieldID != "" && optionID != "" {
				if err := gh.SetProjectV2ItemSingleSelectValue(ctx, dst, dstProject.ID, itemID, columnFieldID, optionID); err != nil {
					return dstProject, fmt.Errorf("failed to set '%s' field for item %s: %w", columnFieldName, itemID, err)
				}
			}
		}
	}

	return dstProject, nil
}

// cardTitleAndBody splits a card note into a title (first line) and body (remaining lines).
// If the note is nil or empty, a placeholder is returned.
func cardTitleAndBody(card gh.ProjectV1Card) (title, body string) {
	if card.Note == nil || strings.TrimSpace(*card.Note) == "" {
		return "(no note)", ""
	}
	lines := strings.SplitN(*card.Note, "\n", 2)
	title = strings.TrimSpace(lines[0])
	if len(lines) > 1 {
		body = lines[1]
	}
	if title == "" {
		title = "(no note)"
	}
	return title, body
}
