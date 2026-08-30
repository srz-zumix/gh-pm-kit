// Package projects provides GitHub Project v1 (classic) to v2 migration logic for gh-pm-kit.
package projects

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// MigrateV1Options controls migration behaviour for GitHub Projects (classic) v1 → v2.
type MigrateV1Options struct {
	// Overwrite skips the idempotency check and re-creates items in an existing destination project.
	Overwrite bool
	// IssueRepo, if set, searches for an existing issue with the migration marker in this
	// repository and links it to the project. If no matching issue is found and CreateIssue
	// is true, a new issue is created instead. If CreateIssue is false, falls back to draft issue.
	IssueRepo *repository.Repository
	// CreateIssue controls whether a new issue is created when IssueRepo is set but no
	// matching issue is found in the repository. Requires IssueRepo to be set.
	CreateIssue bool
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

// v1BoardViewName is the name of the board view created to mirror the classic project layout.
const v1BoardViewName = "Board"

// MigrateProjectV1ToV2 migrates a GitHub Project (classic) to a new GitHub Projects v2 project.
// It creates the destination project, adds a Column single-select field for each source column,
// and migrates all cards (including archived ones) as items in the source card order.
// A migration marker is embedded in the destination project readme for idempotent re-runs.
func MigrateProjectV1ToV2(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, srcRepoName, dstOwner string, srcNumber int, opts *MigrateV1Options) (_ *gh.ProjectV2, retErr error) {
	srcRepo := repository.Repository{Owner: srcOwner, Name: srcRepoName}
	srcProject, err := gh.GetProjectV1ByNumber(ctx, src, srcRepo, srcNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source classic project #%d for '%s': %w", srcNumber, srcOwner, err)
	}

	srcColumns, err := gh.ListProjectV1Columns(ctx, src, srcRepo, srcProject.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns for classic project #%d of '%s': %w", srcNumber, srcOwner, err)
	}

	// Fetch all cards per column upfront, including archived ones.
	cardsByColumn := make([][]gh.ProjectV1Card, len(srcColumns))
	for i, col := range srcColumns {
		cards, err := gh.ListProjectV1CardsAll(ctx, src, srcRepo, col.ID)
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

	if prev := findProjectByMarker(dstProjects, marker); prev != nil {
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated v1 project", "title", prev.Title, "projectID", prev.ID)
			return prev, nil
		}
		// A closed project rejects content mutations, so reopen it for the duration
		// of the migration and restore its original state if the migration fails.
		restore, err := reopenForMigration(ctx, dst, prev)
		if err != nil {
			return prev, err
		}
		defer restore(&retErr)
		columnField, err := resolveColumnField(ctx, dst, dstOwner, prev)
		if err != nil {
			return prev, err
		}
		if err := migrateV1ItemsInto(ctx, src, dst, srcHost, srcOwner, srcRepoName, dstOwner, srcNumber, srcColumns, cardsByColumn, prev, columnField, opts); err != nil {
			return prev, err
		}
		if err := applyV1ProjectClosedState(ctx, dst, prev, srcProject); err != nil {
			return prev, err
		}
		return prev, nil
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

	columnField, err := resolveColumnField(ctx, dst, dstOwner, dstProject)
	if err != nil {
		return dstProject, err
	}
	createV1BoardView(ctx, dst, dstOwner, dstProject, columnField)
	if err := migrateV1ItemsInto(ctx, src, dst, srcHost, srcOwner, srcRepoName, dstOwner, srcNumber, srcColumns, cardsByColumn, dstProject, columnField, opts); err != nil {
		return dstProject, err
	}
	if err := applyV1ProjectClosedState(ctx, dst, dstProject, srcProject); err != nil {
		return dstProject, err
	}
	return dstProject, nil
}

// resolveColumnField returns the destination SINGLE_SELECT field that represents the v1 columns,
// or nil when the destination project does not have it.
func resolveColumnField(ctx context.Context, dst *gh.GitHubClient, dstOwner string, dstProject *gh.ProjectV2) (*gh.ProjectV2Field, error) {
	dstFields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for destination project #%d: %w", dstProject.Number, err)
	}
	for i := range dstFields {
		if dstFields[i].Name == columnFieldName && dstFields[i].DataType == "SINGLE_SELECT" {
			return &dstFields[i], nil
		}
	}
	return nil, nil
}

// createV1BoardView replaces the default view of a freshly created destination project with a
// board view grouped by the Column field, which mirrors the classic project layout.
func createV1BoardView(ctx context.Context, dst *gh.GitHubClient, dstOwner string, dstProject *gh.ProjectV2, columnField *gh.ProjectV2Field) {
	if columnField == nil || columnField.DatabaseID == 0 {
		return
	}
	defaultViews, err := gh.ListProjectV2Views(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		logger.Warn("failed to list destination project views", "error", err)
		return
	}
	input := gh.ProjectV2ViewInput{
		Name:            v1BoardViewName,
		Layout:          "BOARD_LAYOUT",
		GroupByFieldIDs: []int64{columnField.DatabaseID},
	}
	if _, err := gh.CreateProjectV2View(ctx, dst, dstOwner, dstProject.Number, input); err != nil {
		logger.Warn("failed to create the board view in the destination project", "name", v1BoardViewName, "error", err)
		return
	}
	// Remove the default view only after the board view exists, because a project must always
	// keep at least one view.
	for _, v := range defaultViews {
		if err := gh.DeleteProjectV2View(ctx, dst, v.ID); err != nil {
			logger.Warn("failed to delete the default view of the destination project", "name", v.Name, "error", err)
		}
	}
}

// applyV1ProjectClosedState aligns the destination project open/closed state with the classic source project.
// It must run after all content mutations because a closed project rejects them.
func applyV1ProjectClosedState(ctx context.Context, dst *gh.GitHubClient, dstProject *gh.ProjectV2, srcProject *gh.ProjectV1) error {
	closed := srcProject.State == "closed"
	if dstProject.Closed == closed {
		return nil
	}
	if _, err := gh.SetProjectV2Closed(ctx, dst, dstProject.ID, closed); err != nil {
		return fmt.Errorf("failed to set closed=%t on destination project '%s': %w", closed, dstProject.Title, err)
	}
	dstProject.Closed = closed
	return nil
}

// migrateV1ItemsInto migrates all v1 cards into an existing v2 destination project and restores
// the source card order. columnField may be nil, in which case column values are not set.
func migrateV1ItemsInto(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, srcRepoName, dstOwner string, srcNumber int, srcColumns []gh.ProjectV1Column, cardsByColumn [][]gh.ProjectV1Card, dstProject *gh.ProjectV2, columnField *gh.ProjectV2Field, opts *MigrateV1Options) error {
	columnOptionIDByName := make(map[string]string)
	if columnField == nil {
		logger.Warn("destination project is missing the expected SINGLE_SELECT field; column values will not be set",
			"field", columnFieldName, "projectNumber", dstProject.Number)
	} else {
		for _, opt := range columnField.Options {
			columnOptionIDByName[opt.Name] = opt.ID
		}
	}

	// Fetch existing items in the destination project for idempotency checks.
	existingItems, err := gh.ListProjectV2Items(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return fmt.Errorf("failed to list items in destination project #%d: %w", dstProject.Number, err)
	}

	var migrated []*gh.ProjectV2Item
	for i, col := range srcColumns {
		optionID := columnOptionIDByName[col.Name]
		if columnField != nil && optionID == "" {
			logger.Warn("no matching option found for source column; column value will not be set for cards in this column",
				"column", col.Name, "projectNumber", dstProject.Number)
		}
		for _, card := range cardsByColumn[i] {
			itemMarker := v1ItemMarker(srcHost, srcOwner, srcRepoName, srcNumber, card.ID)

			// Check whether this card was already migrated.
			if existing := findItemByMarker(existingItems, itemMarker); existing != nil {
				if opts == nil || !opts.Overwrite {
					logger.Info("skipping already-migrated v1 card", "cardID", card.ID, "column", col.Name)
					// Copy the item because existingItems is mutated later in the loop.
					item := *existing
					syncItemArchiveState(ctx, dst, dstProject.ID, &item, card.Archived)
					migrated = append(migrated, &item)
					continue
				}
				// Overwrite: delete the existing item before re-creating it.
				if err := gh.DeleteProjectV2Item(ctx, dst, dstProject.ID, existing.ID); err != nil {
					return fmt.Errorf("failed to delete existing item %s for card %d: %w", existing.ID, card.ID, err)
				}
				// Remove the deleted item from the cache to keep it consistent.
				for j, it := range existingItems {
					if it.ID == existing.ID {
						existingItems = append(existingItems[:j], existingItems[j+1:]...)
						break
					}
				}
			}

			item, err := migrateV1Card(ctx, src, dst, card, itemMarker, dstProject.ID, opts)
			if err != nil {
				return err
			}
			logger.Info("migrated v1 card", "cardID", card.ID, "column", col.Name, "itemID", item.ID)

			// Set the Column field to the source column name.
			if columnField != nil && optionID != "" {
				if err := gh.SetProjectV2ItemSingleSelectValue(ctx, dst, dstProject.ID, item.ID, columnField.ID, optionID); err != nil {
					return fmt.Errorf("failed to set '%s' field for item %s: %w", columnFieldName, item.ID, err)
				}
			}
			if card.Archived {
				if err := gh.ArchiveProjectV2Item(ctx, dst, dstProject.ID, item.ID); err != nil {
					logger.Warn("failed to archive item", "cardID", card.ID, "itemID", item.ID, "error", err)
				} else {
					item.IsArchived = true
				}
			}
			existingItems = append(existingItems, *item)
			migrated = append(migrated, item)
		}
	}
	applyItemOrder(ctx, dst, dstProject.ID, migrated)
	return nil
}

// migrateV1Card creates the destination item for a single classic project card.
// When opts.IssueRepo is set, an existing issue carrying the migration marker is linked, or a new
// issue is created when opts.CreateIssue is also set. Otherwise the card becomes a draft issue.
func migrateV1Card(ctx context.Context, src, dst *gh.GitHubClient, card gh.ProjectV1Card, marker, dstProjectID string, opts *MigrateV1Options) (*gh.ProjectV2Item, error) {
	title, body := v1CardContent(ctx, src, card)
	body = embedMarker(body, marker)

	var itemID string
	itemType := gh.ProjectV2ItemTypeDraftIssue

	if opts != nil && opts.IssueRepo != nil {
		// Search for an existing issue that carries the migration marker.
		issues, err := gh.SearchIssues(ctx, dst, *opts.IssueRepo, fmt.Sprintf("%q", marker))
		if err != nil {
			logger.Warn("failed to search issues for migration marker; falling back to draft issue", "error", err)
		} else if len(issues) > 0 {
			itemID, err = gh.AddProjectV2ItemByID(ctx, dst, dstProjectID, issues[0].GetNodeID())
			if err != nil {
				return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
			}
			itemType = gh.ProjectV2ItemTypeIssue
		} else if opts.CreateIssue {
			issue, err := gh.CreateIssue(ctx, dst, *opts.IssueRepo, title, body, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create issue '%s' in repository '%s/%s': %w", title, opts.IssueRepo.Owner, opts.IssueRepo.Name, err)
			}
			itemID, err = gh.AddProjectV2ItemByID(ctx, dst, dstProjectID, issue.GetNodeID())
			if err != nil {
				return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
			}
			itemType = gh.ProjectV2ItemTypeIssue
		}
	}

	if itemID == "" {
		var err error
		itemID, err = gh.AddProjectV2DraftIssue(ctx, dst, dstProjectID, title, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create draft issue '%s' for card %d: %w", title, card.ID, err)
		}
		itemType = gh.ProjectV2ItemTypeDraftIssue
	}

	return &gh.ProjectV2Item{
		ID:      itemID,
		Content: gh.ProjectV2ItemContent{Type: itemType, Title: title, Body: body},
	}, nil
}

// v1CardContent returns the title and body to migrate for a card. Note cards use the note text,
// while issue and pull-request cards are fetched from the source host so that their title and
// body can be reproduced, with an attribution header pointing at the original.
func v1CardContent(ctx context.Context, src *gh.GitHubClient, card gh.ProjectV1Card) (title, body string) {
	if card.Note != nil && strings.TrimSpace(*card.Note) != "" {
		return cardTitleAndBody(card)
	}
	if card.ContentURL == nil || *card.ContentURL == "" {
		return "(no note)", ""
	}
	contentRepo, number, ok := parseCardContentURL(*card.ContentURL)
	if !ok {
		logger.Warn("failed to parse the content URL of a card; using the URL as the title", "cardID", card.ID, "contentURL", *card.ContentURL)
		return *card.ContentURL, ""
	}
	issue, err := gh.GetIssue(ctx, src, contentRepo, number)
	if err != nil {
		logger.Warn("failed to get the issue referenced by a card; using the content URL as the title", "cardID", card.ID, "contentURL", *card.ContentURL, "error", err)
		return *card.ContentURL, ""
	}
	author := issue.GetUser().GetLogin()
	if author == "" {
		author = "unknown"
	}
	header := fmt.Sprintf("> *Originally posted by @%s \u2014 %s*", author, issue.GetHTMLURL())
	return issue.GetTitle(), header + "\n\n" + issue.GetBody()
}

// parseCardContentURL extracts the repository and issue number from a classic project card
// content URL such as https://api.github.com/repos/OWNER/REPO/issues/1. GitHub Enterprise Server
// URLs carry an /api/v3 prefix, so the /repos/ segment is located instead of a fixed offset.
func parseCardContentURL(rawURL string) (repository.Repository, int, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return repository.Repository{}, 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+4 < len(parts); i++ {
		if parts[i] != "repos" {
			continue
		}
		number, err := strconv.Atoi(parts[i+4])
		if err != nil {
			return repository.Repository{}, 0, false
		}
		return repository.Repository{Owner: parts[i+1], Name: parts[i+2]}, number, true
	}
	return repository.Repository{}, 0, false
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
