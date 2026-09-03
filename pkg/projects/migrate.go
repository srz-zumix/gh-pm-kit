// Package projects provides GitHub Project v2 migration logic for gh-pm-kit.
package projects

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// projectCleanupTimeout bounds the best-effort restore of a project's closed
// state when a migration fails, so cleanup cannot hang on a slow API.
const projectCleanupTimeout = 30 * time.Second

// reopenForMigration reopens a closed destination project so it accepts content
// mutations during migration. It returns a restore function to be deferred by the
// caller: on migration failure (*retErr != nil) it best-effort re-closes the project
// to preserve its original state; on success it is a no-op because the migration's
// applyProjectClosedState already sets the intended state. When the project was
// already open the restore is a no-op.
func reopenForMigration(ctx context.Context, dst *gh.GitHubClient, project *gh.ProjectV2) (func(retErr *error), error) {
	if !project.Closed {
		return func(*error) {}, nil
	}
	if _, err := gh.SetProjectV2Closed(ctx, dst, project.ID, false); err != nil {
		return nil, fmt.Errorf("failed to reopen destination project '%s' for migration: %w", project.Title, err)
	}
	project.Closed = false
	return func(retErr *error) {
		if retErr == nil || *retErr == nil {
			return
		}
		// Derive a fresh, bounded context so restore still runs even if the
		// migration failed because ctx was canceled or timed out.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), projectCleanupTimeout)
		defer cancel()
		if _, err := gh.SetProjectV2Closed(cleanupCtx, dst, project.ID, true); err != nil {
			*retErr = errors.Join(*retErr, fmt.Errorf("failed to restore closed state on destination project '%s' after a failed migration: %w", project.Title, err))
			return
		}
		project.Closed = true
	}, nil
}

// MigrateOptions controls migration behaviour for GitHub Projects v2.
type MigrateOptions struct {
	// Overwrite deletes the previously-migrated item (identified by migration marker)
	// and recreates it. Without this option, already-migrated items are skipped.
	Overwrite bool
	// IssueRepo, if set, searches for an existing issue with the migration marker in this
	// repository and links it to the project. If no matching issue is found and CreateIssue
	// is true, a new issue is created instead. If CreateIssue is false, falls back to draft issue.
	IssueRepo *repository.Repository
	// CreateIssue controls whether a new issue is created when IssueRepo is set but no
	// matching issue is found in the repository. Requires IssueRepo to be set.
	CreateIssue bool
}

// migratedItemMarker returns the HTML comment marker embedded in migrated draft-issue bodies.
// The source project identity (host, owner, number) is SHA-256-hashed to avoid leaking it
// and to prevent cross-host collisions when the same owner/number exists on multiple hosts.
func migratedItemMarker(srcHost, srcOwner string, srcProjectNumber int, srcItemID string) string {
	projectKey := fmt.Sprintf("%s:%s#%d", srcHost, srcOwner, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	itemHash := fmt.Sprintf("%x", sha256.Sum256([]byte(srcItemID)))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-project-item:%s/%s -->", projectHash, itemHash)
}

// isMigratedItem reports whether a draft-issue body contains the given marker.
func isMigratedItem(body, marker string) bool {
	return strings.Contains(body, marker)
}

// projectMarkerPrefix returns the prefix shared by all items migrated from the same source project.
func projectMarkerPrefix(srcHost, srcOwner string, srcProjectNumber int) string {
	projectKey := fmt.Sprintf("%s:%s#%d", srcHost, srcOwner, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-project-item:%s/", projectHash)
}

// migratedProjectMarker returns the HTML comment marker embedded in migrated project readmes
// to identify the migration source, enabling idempotent re-runs.
func migratedProjectMarker(srcHost, srcOwner string, srcProjectNumber int) string {
	projectKey := fmt.Sprintf("%s:%s#%d", srcHost, srcOwner, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-project:%s -->", projectHash)
}

// migratedStatusUpdateMarker returns the HTML comment marker embedded in migrated status update bodies.
func migratedStatusUpdateMarker(srcHost, srcOwner string, srcProjectNumber int, srcStatusUpdateID string) string {
	projectKey := fmt.Sprintf("%s:%s#%d", srcHost, srcOwner, srcProjectNumber)
	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey)))[:16]
	updateHash := fmt.Sprintf("%x", sha256.Sum256([]byte(srcStatusUpdateID)))[:16]
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-project-status-update:%s/%s -->", projectHash, updateHash)
}

// findProjectByMarker returns a pointer to the first project whose readme contains marker, or nil.
func findProjectByMarker(projects []gh.ProjectV2, marker string) *gh.ProjectV2 {
	for i := range projects {
		if projects[i].Readme != nil && strings.Contains(*projects[i].Readme, marker) {
			return &projects[i]
		}
	}
	return nil
}

// embedMarker appends marker to s (separated by a blank line) if not already present.
func embedMarker(s, marker string) string {
	if strings.Contains(s, marker) {
		return s
	}
	if s == "" {
		return marker
	}
	return s + "\n\n" + marker
}

// dstProjectContext holds pre-fetched destination project state shared across multiple item migrations.
type dstProjectContext struct {
	projectID string
	// fieldByName maps destination field name to field for quick lookup.
	fieldByName map[string]*gh.ProjectV2Field
	// items holds every existing item of the destination project as returned by
	// ListProjectV2Items (draft issues, issues, and pull requests).
	items []gh.ProjectV2Item
	// itemByContentKey maps an issue/pull-request content key ("owner/repo#number") to the ID of
	// the project item that already links it, so re-runs reuse the link instead of creating a duplicate.
	itemByContentKey map[string]string
}

// newDstProjectContext creates an empty context for the given destination project.
func newDstProjectContext(projectID string, fieldByName map[string]*gh.ProjectV2Field) *dstProjectContext {
	return &dstProjectContext{
		projectID:        projectID,
		fieldByName:      fieldByName,
		itemByContentKey: make(map[string]string),
	}
}

// isLinkableContent reports whether the content is an issue or pull request that carries a usable
// repository name and number, so it can participate in content-based linking and deduplication.
func isLinkableContent(c *gh.ProjectV2ItemContent) bool {
	if c.Type != gh.ProjectV2ItemTypeIssue && c.Type != gh.ProjectV2ItemTypePullRequest {
		return false
	}
	return c.RepoName != "" && c.Number != 0
}

// itemContentKey returns the identity key "owner/repo#number" (lowercased) of an existing
// destination issue or pull request item, or "" when the item is not linkable or its owner is
// unknown. The owner is included so items from repositories that share a name and number under
// different owners never collide in itemByContentKey.
func itemContentKey(c *gh.ProjectV2ItemContent) string {
	if !isLinkableContent(c) || c.RepoOwner == "" {
		return ""
	}
	return strings.ToLower(c.RepoOwner+"/"+c.RepoName) + "#" + strconv.Itoa(c.Number)
}

// expectedDstContentKey returns the identity key the destination copy of a source item is expected
// to have: the source repository name and number under dstOwner. Migration mirrors <owner>/repo#N
// from the source to dstOwner/repo#N in the destination, so the source owner is deliberately ignored.
func expectedDstContentKey(dstOwner string, c *gh.ProjectV2ItemContent) string {
	if !isLinkableContent(c) {
		return ""
	}
	return strings.ToLower(dstOwner+"/"+c.RepoName) + "#" + strconv.Itoa(c.Number)
}

// addItem caches a destination item and indexes it by content key when it links an issue or PR.
func (c *dstProjectContext) addItem(item gh.ProjectV2Item) {
	c.items = append(c.items, item)
	if key := itemContentKey(&item.Content); key != "" {
		c.itemByContentKey[key] = item.ID
	}
}

// findItemByID returns a pointer to the cached item with the given ID, or nil.
func (c *dstProjectContext) findItemByID(itemID string) *gh.ProjectV2Item {
	for i := range c.items {
		if c.items[i].ID == itemID {
			return &c.items[i]
		}
	}
	return nil
}

// removeItem removes an item by ID from the cached items slice.
func (c *dstProjectContext) removeItem(itemID string) {
	for i := range c.items {
		if c.items[i].ID == itemID {
			if key := itemContentKey(&c.items[i].Content); key != "" {
				delete(c.itemByContentKey, key)
			}
			c.items[i] = c.items[len(c.items)-1]
			c.items = c.items[:len(c.items)-1]
			return
		}
	}
}

// findItemByMarker returns a pointer to the first draft-issue or issue item whose body
// contains marker, or nil if none is found.
func findItemByMarker(items []gh.ProjectV2Item, marker string) *gh.ProjectV2Item {
	for i := range items {
		t := items[i].Content.Type
		if (t == gh.ProjectV2ItemTypeDraftIssue || t == gh.ProjectV2ItemTypeIssue) &&
			isMigratedItem(items[i].Content.Body, marker) {
			return &items[i]
		}
	}
	return nil
}

// prepareDstContext fetches the destination project ID, fields, and existing items once
// to share across multiple item migrations.
func prepareDstContext(ctx context.Context, dst *gh.GitHubClient, dstOwner string, dstProjectNumber int) (*dstProjectContext, error) {
	project, err := gh.GetProjectV2ByNumber(ctx, dst, dstOwner, dstProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination project #%d for '%s': %w", dstProjectNumber, dstOwner, err)
	}
	fields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for destination project #%d of '%s': %w", dstProjectNumber, dstOwner, err)
	}
	fieldByName := make(map[string]*gh.ProjectV2Field, len(fields))
	for i := range fields {
		fieldByName[fields[i].Name] = &fields[i]
	}
	items, err := gh.ListProjectV2Items(ctx, dst, dstOwner, dstProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for destination project #%d of '%s': %w", dstProjectNumber, dstOwner, err)
	}
	dstCtx := newDstProjectContext(string(project.ID), fieldByName)
	for i := range items {
		dstCtx.addItem(items[i])
	}
	return dstCtx, nil
}

// migratableDataTypes are the field data types that can be migrated.
var migratableDataTypes = map[string]bool{
	"TEXT":          true,
	"NUMBER":        true,
	"DATE":          true,
	"SINGLE_SELECT": true,
	"MULTI_SELECT":  true,
	"ITERATION":     true,
}

// MigrateProject migrates a ProjectV2 from srcOwner to dstOwner.
// It creates the destination project, copies custom fields, then migrates items as draft issues.
// A migration marker is embedded in the project readme to enable idempotent re-runs.
func MigrateProject(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, projectNumber int, opts *MigrateOptions) (*gh.ProjectV2, error) {
	srcProject, err := gh.GetProjectV2ByNumber(ctx, src, srcOwner, projectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source project #%d for '%s': %w", projectNumber, srcOwner, err)
	}
	srcFields, err := gh.ListProjectV2Fields(ctx, src, srcOwner, projectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for source project #%d of '%s': %w", projectNumber, srcOwner, err)
	}
	srcItems, err := gh.ListProjectV2Items(ctx, src, srcOwner, projectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for source project #%d of '%s': %w", projectNumber, srcOwner, err)
	}

	projectMarker := migratedProjectMarker(srcHost, srcOwner, projectNumber)
	dstProjects, err := gh.ListProjectsV2(ctx, dst, dstOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to list destination projects for '%s': %w", dstOwner, err)
	}

	if prev := findProjectByMarker(dstProjects, projectMarker); prev != nil {
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated project", "title", prev.Title, "projectID", prev.ID)
			return prev, nil
		}
		// Overwrite: update the existing destination project in-place.
		return migrateIntoProject(ctx, src, dst, srcHost, srcOwner, dstOwner, srcProject, srcFields, srcItems, prev, projectMarker, opts)
	}

	dstOwnerID, err := gh.GetOwnerNodeID(ctx, dst, dstOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to get node ID for destination owner '%s': %w", dstOwner, err)
	}
	dstProject, err := gh.CreateProjectV2(ctx, dst, *dstOwnerID, string(srcProject.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to create destination project '%s' for '%s': %w", string(srcProject.Title), dstOwner, err)
	}
	if err := updateProjectMetadata(ctx, dst, dstProject, srcProject, projectMarker); err != nil {
		return dstProject, err
	}
	dstFieldByName, err := createProjectFields(ctx, dst, dstOwner, dstProject, srcFields)
	if err != nil {
		return dstProject, err
	}
	migrateViews(ctx, src, dst, srcOwner, dstOwner, projectNumber, dstProject, dstFieldByName)
	reportWorkflows(ctx, src, dst, srcOwner, dstOwner, projectNumber, dstProject)
	dstCtx := newDstProjectContext(string(dstProject.ID), dstFieldByName)
	if _, err := migrateItems(ctx, dst, srcHost, srcOwner, projectNumber, dstOwner, srcItems, dstCtx, opts); err != nil {
		return dstProject, err
	}
	migrateStatusUpdates(ctx, src, dst, srcHost, srcOwner, dstOwner, projectNumber, dstProject, opts)
	if err := applyProjectClosedState(ctx, dst, dstProject, srcProject); err != nil {
		return dstProject, err
	}
	return dstProject, nil
}

// migrateIntoProject migrates source project contents into an existing destination project
// in-place: refreshes metadata, creates any missing fields, and migrates items according to opts.
// Item-level idempotency (skip / overwrite) is governed by opts as in any other migration mode.
func migrateIntoProject(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcProject *gh.ProjectV2, srcFields []gh.ProjectV2Field, srcItems []gh.ProjectV2Item, prev *gh.ProjectV2, marker string, opts *MigrateOptions) (_ *gh.ProjectV2, retErr error) {
	// A closed project rejects content mutations, so reopen it for the duration of
	// the migration and restore its original state if the migration fails.
	restore, err := reopenForMigration(ctx, dst, prev)
	if err != nil {
		return prev, err
	}
	defer restore(&retErr)
	if err := updateProjectMetadata(ctx, dst, prev, srcProject, marker); err != nil {
		return prev, err
	}
	dstCtx, err := prepareDstContext(ctx, dst, dstOwner, prev.Number)
	if err != nil {
		return prev, fmt.Errorf("failed to prepare destination context for overwrite: %w", err)
	}
	dstFieldByName, err := createProjectFields(ctx, dst, dstOwner, prev, srcFields)
	if err != nil {
		return prev, err
	}
	migrateViews(ctx, src, dst, srcOwner, dstOwner, srcProject.Number, prev, dstFieldByName)
	reportWorkflows(ctx, src, dst, srcOwner, dstOwner, srcProject.Number, prev)
	dstCtx.fieldByName = dstFieldByName
	if _, err := migrateItems(ctx, dst, srcHost, srcOwner, srcProject.Number, dstOwner, srcItems, dstCtx, opts); err != nil {
		return prev, err
	}
	migrateStatusUpdates(ctx, src, dst, srcHost, srcOwner, dstOwner, srcProject.Number, prev, opts)
	if err := applyProjectClosedState(ctx, dst, prev, srcProject); err != nil {
		return prev, err
	}
	return prev, nil
}

// MigrateProjectTo migrates a source project into a specific existing destination project.
// Metadata and missing custom fields are always applied; item-level behaviour (skip already-migrated
// items or overwrite) is controlled by opts the same way as in MigrateProject.
func MigrateProjectTo(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcProjectNumber, dstProjectNumber int, opts *MigrateOptions) (*gh.ProjectV2, error) {
	srcProject, err := gh.GetProjectV2ByNumber(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source project #%d for '%s': %w", srcProjectNumber, srcOwner, err)
	}
	srcFields, err := gh.ListProjectV2Fields(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for source project #%d of '%s': %w", srcProjectNumber, srcOwner, err)
	}
	srcItems, err := gh.ListProjectV2Items(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for source project #%d of '%s': %w", srcProjectNumber, srcOwner, err)
	}
	dstProject, err := gh.GetProjectV2ByNumber(ctx, dst, dstOwner, dstProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination project #%d for '%s': %w", dstProjectNumber, dstOwner, err)
	}
	marker := migratedProjectMarker(srcHost, srcOwner, srcProjectNumber)
	return migrateIntoProject(ctx, src, dst, srcHost, srcOwner, dstOwner, srcProject, srcFields, srcItems, dstProject, marker, opts)
}

// MigrateProjectItems migrates only the items of an existing source project into an existing
// destination project. Both projects must already exist and fields must be set up in the destination.
func MigrateProjectItems(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcProjectNumber, dstProjectNumber int, opts *MigrateOptions) ([]*gh.ProjectV2Item, error) {
	srcItems, err := gh.ListProjectV2Items(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for source project #%d of '%s': %w", srcProjectNumber, srcOwner, err)
	}
	dstCtx, err := prepareDstContext(ctx, dst, dstOwner, dstProjectNumber)
	if err != nil {
		return nil, err
	}
	return migrateItems(ctx, dst, srcHost, srcOwner, srcProjectNumber, dstOwner, srcItems, dstCtx, opts)
}

// migrateItems migrates every source item into the destination project and then restores
// the source item order. Returns the destination items in source order.
func migrateItems(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner string, srcProjectNumber int, dstOwner string, srcItems []gh.ProjectV2Item, dstCtx *dstProjectContext, opts *MigrateOptions) ([]*gh.ProjectV2Item, error) {
	var results []*gh.ProjectV2Item
	for i := range srcItems {
		item, err := migrateItem(ctx, dst, srcHost, srcOwner, srcProjectNumber, dstOwner, &srcItems[i], dstCtx, opts)
		if err != nil {
			return results, err
		}
		if item != nil {
			results = append(results, item)
		}
	}
	applyItemOrder(ctx, dst, dstCtx.projectID, results)
	return results, nil
}

// applyItemOrder repositions destination items so that they follow the source item order.
// Archived items are skipped because they cannot be repositioned.
func applyItemOrder(ctx context.Context, dst *gh.GitHubClient, projectID string, items []*gh.ProjectV2Item) {
	var afterID *string
	for _, item := range items {
		if item.IsArchived {
			continue
		}
		if err := gh.MoveProjectV2Item(ctx, dst, projectID, item.ID, afterID); err != nil {
			logger.Warn("failed to restore item order", "title", item.Content.Title, "itemID", item.ID, "error", err)
			return
		}
		id := item.ID
		afterID = &id
	}
}

// updateProjectMetadata sets the description, readme (with migration marker), and public flag on the destination project.
func updateProjectMetadata(ctx context.Context, dst *gh.GitHubClient, dstProject *gh.ProjectV2, srcProject *gh.ProjectV2, marker string) error {
	var shortDesc *string
	if srcProject.ShortDescription != nil {
		s := *srcProject.ShortDescription
		shortDesc = &s
	}
	readmeStr := ""
	if srcProject.Readme != nil {
		readmeStr = *srcProject.Readme
	}
	readmeStr = embedMarker(readmeStr, marker)
	public := srcProject.Public
	if _, err := gh.UpdateProjectV2Metadata(ctx, dst, dstProject.ID, shortDesc, &readmeStr, &public); err != nil {
		return fmt.Errorf("failed to update metadata for destination project '%s': %w", dstProject.Title, err)
	}
	return nil
}

// applyProjectClosedState aligns the destination project open/closed state with the source.
// It must run after all content mutations because a closed project rejects them.
func applyProjectClosedState(ctx context.Context, dst *gh.GitHubClient, dstProject *gh.ProjectV2, srcProject *gh.ProjectV2) error {
	if dstProject.Closed == srcProject.Closed {
		return nil
	}
	if _, err := gh.SetProjectV2Closed(ctx, dst, dstProject.ID, srcProject.Closed); err != nil {
		return fmt.Errorf("failed to set closed=%t on destination project '%s': %w", srcProject.Closed, dstProject.Title, err)
	}
	dstProject.Closed = srcProject.Closed
	return nil
}

// migrateStatusUpdates copies the source project status updates into the destination project.
// Each destination status update carries a hidden marker so re-runs are idempotent: existing
// ones are skipped, or refreshed in place when opts.Overwrite is set. Failures are reported as
// warnings because status updates are supplementary to the project contents.
func migrateStatusUpdates(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcProjectNumber int, dstProject *gh.ProjectV2, opts *MigrateOptions) {
	srcUpdates, err := gh.ListProjectV2StatusUpdates(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		logger.Warn("failed to list source project status updates", "error", err)
		return
	}
	if len(srcUpdates) == 0 {
		return
	}
	dstUpdates, err := gh.ListProjectV2StatusUpdates(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		logger.Warn("failed to list destination project status updates", "error", err)
		return
	}
	for i := range srcUpdates {
		su := &srcUpdates[i]
		marker := migratedStatusUpdateMarker(srcHost, srcOwner, srcProjectNumber, su.ID)
		body := statusUpdateBody(su, marker)
		status := statusUpdateStatus(su)
		startDate := optionalDate(su.StartDate)
		targetDate := optionalDate(su.TargetDate)
		if prev := findStatusUpdateByMarker(dstUpdates, marker); prev != nil {
			if opts == nil || !opts.Overwrite {
				logger.Info("skipping already-migrated status update", "statusUpdateID", prev.ID)
				continue
			}
			if err := gh.UpdateProjectV2StatusUpdate(ctx, dst, prev.ID, &body, status, startDate, targetDate); err != nil {
				logger.Warn("failed to overwrite migrated status update", "statusUpdateID", prev.ID, "error", err)
			}
			continue
		}
		if _, err := gh.CreateProjectV2StatusUpdate(ctx, dst, dstProject.ID, &body, status, startDate, targetDate); err != nil {
			logger.Warn("failed to create status update in destination project", "sourceStatusUpdateID", su.ID, "error", err)
		}
	}
}

// findStatusUpdateByMarker returns the first status update whose body contains marker, or nil.
func findStatusUpdateByMarker(updates []gh.ProjectV2StatusUpdate, marker string) *gh.ProjectV2StatusUpdate {
	for i := range updates {
		if strings.Contains(updates[i].Body, marker) {
			return &updates[i]
		}
	}
	return nil
}

// statusUpdateBody builds the destination status update body with an attribution header
// (the creator and creation time cannot be reproduced on the destination) and the marker.
func statusUpdateBody(su *gh.ProjectV2StatusUpdate, marker string) string {
	creator := su.Creator
	if creator == "" {
		creator = "unknown"
	}
	header := fmt.Sprintf("> *Originally posted by @%s \u2014 %s*", creator, su.CreatedAt)
	return header + "\n\n" + su.Body + "\n\n" + marker
}

// statusUpdateStatus returns the status to apply, or nil when the source has none.
func statusUpdateStatus(su *gh.ProjectV2StatusUpdate) *gh.ProjectV2StatusUpdateStatus {
	if su.Status == "" {
		return nil
	}
	s := su.Status
	return &s
}

// optionalDate returns a pointer to date, or nil when it is empty.
func optionalDate(date string) *string {
	if date == "" {
		return nil
	}
	return &date
}

// migrateViews recreates the source project views in the destination project through the
// Project views REST API. Views are matched by name and existing ones are left untouched
// because there is no view update endpoint. Destination views that do not exist in the
// source (such as the default view of a newly created project) are removed afterwards.
// Field references are resolved by field name.
func migrateViews(ctx context.Context, src, dst *gh.GitHubClient, srcOwner, dstOwner string, srcProjectNumber int, dstProject *gh.ProjectV2, dstFieldByName map[string]*gh.ProjectV2Field) {
	srcViews, err := gh.ListProjectV2Views(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		logger.Warn("failed to list source project views", "error", err)
		return
	}
	if len(srcViews) == 0 {
		return
	}
	dstViews, err := gh.ListProjectV2Views(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		logger.Warn("failed to list destination project views", "error", err)
		return
	}
	existing := make(map[string]bool, len(dstViews))
	for _, v := range dstViews {
		existing[v.Name] = true
	}
	srcNames := make(map[string]bool, len(srcViews))
	for i := range srcViews {
		v := &srcViews[i]
		srcNames[v.Name] = true
		if existing[v.Name] {
			logger.Info("skipping existing view", "name", v.Name)
			continue
		}
		input := gh.ProjectV2ViewInput{
			Name:                    v.Name,
			Layout:                  v.Layout,
			Filter:                  v.Filter,
			VisibleFieldIDs:         resolveFieldIDs(dstFieldByName, v.VisibleFields),
			GroupByFieldIDs:         resolveFieldIDs(dstFieldByName, v.GroupByFields),
			VerticalGroupByFieldIDs: resolveFieldIDs(dstFieldByName, v.VerticalGroupByFields),
		}
		for _, s := range v.SortBy {
			f, ok := dstFieldByName[s.FieldName]
			if !ok || f.DatabaseID == 0 {
				continue
			}
			input.SortBy = append(input.SortBy, gh.ProjectV2ViewSortByInput{FieldID: f.DatabaseID, Direction: s.Direction})
		}
		if _, err := gh.CreateProjectV2View(ctx, dst, dstOwner, dstProject.Number, input); err != nil {
			logger.Warn("failed to create view in destination project", "name", v.Name, "layout", v.Layout, "error", err)
			continue
		}
		logger.Info("created view", "name", v.Name, "layout", v.Layout)
	}
	// Delete destination-only views after the source views exist, because a project
	// must always keep at least one view.
	for i := range dstViews {
		v := &dstViews[i]
		if srcNames[v.Name] {
			continue
		}
		if err := gh.DeleteProjectV2View(ctx, dst, v.ID); err != nil {
			logger.Warn("failed to delete view missing in the source project", "name", v.Name, "error", err)
			continue
		}
		logger.Info("deleted view missing in the source project", "name", v.Name)
	}
}

// reportWorkflows warns about built-in automations that must be re-enabled by hand. The GraphQL
// API exposes only a workflow's name and enabled flag, and offers no create or update mutation.
func reportWorkflows(ctx context.Context, src, dst *gh.GitHubClient, srcOwner, dstOwner string, srcProjectNumber int, dstProject *gh.ProjectV2) {
	srcWorkflows, err := gh.ListProjectV2Workflows(ctx, src, srcOwner, srcProjectNumber)
	if err != nil {
		logger.Warn("failed to list source project workflows", "error", err)
		return
	}
	var enabled []string
	for _, w := range srcWorkflows {
		if w.Enabled {
			enabled = append(enabled, w.Name)
		}
	}
	if len(enabled) == 0 {
		return
	}
	dstEnabled := make(map[string]bool)
	dstWorkflows, err := gh.ListProjectV2Workflows(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		logger.Warn("failed to list destination project workflows", "error", err)
	} else {
		for _, w := range dstWorkflows {
			dstEnabled[w.Name] = w.Enabled
		}
	}
	var missing []string
	for _, name := range enabled {
		if !dstEnabled[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	logger.Warn("workflows cannot be migrated because the API provides no way to create or enable them; enable them manually in the destination project settings",
		"workflows", strings.Join(missing, ", "), "url", dstProject.URL)
}

// resolveFieldIDs maps source field names to the REST field IDs of the destination project,
// dropping names that have no destination counterpart.
func resolveFieldIDs(dstFieldByName map[string]*gh.ProjectV2Field, names []string) []int64 {
	var ids []int64
	for _, name := range names {
		f, ok := dstFieldByName[name]
		if !ok || f.DatabaseID == 0 {
			continue
		}
		ids = append(ids, f.DatabaseID)
	}
	return ids
}

// createProjectFields creates all migratable custom fields from srcFields in the destination project.
// Fields whose names already exist in the destination (built-in or previously created) are skipped.
// Returns a map of field name to destination ProjectV2Field (including pre-existing ones).
func createProjectFields(ctx context.Context, dst *gh.GitHubClient, dstOwner string, dstProject *gh.ProjectV2, srcFields []gh.ProjectV2Field) (map[string]*gh.ProjectV2Field, error) {
	dstFieldByName := make(map[string]*gh.ProjectV2Field)
	// Fetch existing destination fields first to skip built-in and already-created fields.
	existingFields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return dstFieldByName, fmt.Errorf("failed to list fields for destination project '%s': %w", string(dstProject.Title), err)
	}
	existingByName := make(map[string]bool, len(existingFields))
	for i := range existingFields {
		f := &existingFields[i]
		existingByName[f.Name] = true
		dstFieldByName[f.Name] = f
	}
	for _, f := range srcFields {
		if !migratableDataTypes[f.DataType] {
			continue
		}
		if existingByName[f.Name] {
			if err := syncSelectOptions(ctx, dst, dstFieldByName[f.Name], f); err != nil {
				return dstFieldByName, err
			}
			if err := syncIterations(ctx, dst, dstFieldByName[f.Name], f); err != nil {
				return dstFieldByName, err
			}
			continue
		}
		dataType := f.DataType
		if (dataType == "SINGLE_SELECT" || dataType == "MULTI_SELECT") && len(f.Options) == 0 {
			// A select field without options cannot be created (API requires at least one option).
			// This can happen when the field is a built-in type on some GitHub Enterprise Server versions.
			// Fall back to TEXT so the option name can still be stored as plain text.
			logger.Info("converting select field with no options to TEXT", "field", f.Name, "dataType", dataType)
			dataType = "TEXT"
		}
		if dataType == "ITERATION" {
			if err := gh.CreateProjectV2IterationField(ctx, dst, string(dstProject.ID), f.Name, f.AllIterations()); err != nil {
				return dstFieldByName, fmt.Errorf("failed to create iteration field '%s' in destination project '%s': %w", f.Name, string(dstProject.Title), err)
			}
			continue
		}
		if dataType == "MULTI_SELECT" {
			// The destination may run a GitHub version without multi-select support, so keep
			// migrating the remaining fields instead of aborting the whole migration.
			if err := gh.CreateProjectV2MultiSelectField(ctx, dst, string(dstProject.ID), f.Name, f.Options); err != nil {
				logger.Warn("failed to create multi select field; skipping it", "field", f.Name, "error", err)
			}
			continue
		}
		if err := gh.CreateProjectV2Field(ctx, dst, string(dstProject.ID), dataType, f.Name, f.Options); err != nil {
			return dstFieldByName, fmt.Errorf("failed to create field '%s' in destination project '%s': %w", f.Name, string(dstProject.Title), err)
		}
	}
	// Re-fetch to pick up newly created fields and their IDs.
	dstFields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstProject.Number)
	if err != nil {
		return dstFieldByName, fmt.Errorf("failed to list fields for destination project '%s': %w", string(dstProject.Title), err)
	}
	for i := range dstFields {
		f := &dstFields[i]
		dstFieldByName[f.Name] = f
	}
	return dstFieldByName, nil
}

// syncSelectOptions aligns the options of an existing destination SINGLE_SELECT or MULTI_SELECT
// field with the source field. Source options are placed first in source order so that board layout
// columns keep the same order as the source project, and destination-only options are appended
// afterwards because the update mutation replaces the whole option list. Options matched by name
// keep their destination ID so existing item values are not lost.
func syncSelectOptions(ctx context.Context, dst *gh.GitHubClient, dstField *gh.ProjectV2Field, srcField gh.ProjectV2Field) error {
	if dstField == nil || dstField.DataType != srcField.DataType || len(srcField.Options) == 0 {
		return nil
	}
	if dstField.DataType != "SINGLE_SELECT" && dstField.DataType != "MULTI_SELECT" {
		return nil
	}
	dstByName := make(map[string]gh.ProjectV2SingleSelectOption, len(dstField.Options))
	for _, o := range dstField.Options {
		dstByName[o.Name] = o
	}
	srcByName := make(map[string]bool, len(srcField.Options))
	merged := make([]gh.ProjectV2SingleSelectOption, 0, len(dstField.Options)+len(srcField.Options))
	for _, s := range srcField.Options {
		srcByName[s.Name] = true
		o := s
		if d, ok := dstByName[s.Name]; ok {
			// Keep the destination option ID so already assigned item values stay valid.
			o.ID = d.ID
		} else {
			// Clear the source option ID so the destination assigns a new one.
			o.ID = ""
		}
		merged = append(merged, o)
	}
	for _, d := range dstField.Options {
		if srcByName[d.Name] {
			continue
		}
		merged = append(merged, d)
	}
	if selectOptionsEqual(dstField.Options, merged) {
		return nil
	}
	update := gh.UpdateProjectV2FieldSingleSelectOptions
	if dstField.DataType == "MULTI_SELECT" {
		update = gh.UpdateProjectV2FieldMultiSelectOptions
	}
	if err := update(ctx, dst, dstField.ID, merged); err != nil {
		return fmt.Errorf("failed to update options of %s field '%s' in destination project: %w", dstField.DataType, dstField.Name, err)
	}
	dstField.Options = merged
	return nil
}

// selectOptionsEqual reports whether two option lists match in order, name, color and description.
func selectOptionsEqual(a, b []gh.ProjectV2SingleSelectOption) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Color != b[i].Color || a[i].Description != b[i].Description {
			return false
		}
	}
	return true
}

// syncIterations aligns the iterations of an existing destination ITERATION field with the source
// field. Source iterations are merged with destination-only ones and ordered by start date, so
// past sprints of the source project are reproduced. The iteration input carries no ID, so the
// destination reassigns iteration IDs on every update.
func syncIterations(ctx context.Context, dst *gh.GitHubClient, dstField *gh.ProjectV2Field, srcField gh.ProjectV2Field) error {
	if dstField == nil || dstField.DataType != "ITERATION" || srcField.DataType != "ITERATION" {
		return nil
	}
	srcIterations := srcField.AllIterations()
	if len(srcIterations) == 0 {
		return nil
	}
	dstIterations := dstField.AllIterations()
	srcByTitle := make(map[string]bool, len(srcIterations))
	for _, it := range srcIterations {
		srcByTitle[it.Title] = true
	}
	merged := make([]gh.ProjectV2IterationOption, 0, len(srcIterations)+len(dstIterations))
	merged = append(merged, srcIterations...)
	for _, it := range dstIterations {
		if srcByTitle[it.Title] {
			continue
		}
		merged = append(merged, it)
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].StartDate < merged[j].StartDate })
	if iterationsEqual(dstIterations, merged) {
		return nil
	}
	if err := gh.UpdateProjectV2FieldIterations(ctx, dst, dstField.ID, merged); err != nil {
		return fmt.Errorf("failed to update iterations of iteration field '%s' in destination project: %w", dstField.Name, err)
	}
	return nil
}

// iterationsEqual reports whether two iteration lists match in order, title, start date and duration.
func iterationsEqual(a, b []gh.ProjectV2IterationOption) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Title != b[i].Title || a[i].StartDate != b[i].StartDate || a[i].Duration != b[i].Duration {
			return false
		}
	}
	return true
}

// migrateItem migrates a single source item into the destination project.
// The destination content is resolved in this order: an item that already carries the migration
// marker, an issue in opts.IssueRepo carrying the marker, the issue or pull request with the same
// repository name and number under dstOwner, a newly created issue when opts.CreateIssue is set,
// and finally a draft issue.
func migrateItem(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner string, srcProjectNumber int, dstOwner string, srcItem *gh.ProjectV2Item, dstCtx *dstProjectContext, opts *MigrateOptions) (*gh.ProjectV2Item, error) {
	if srcItem.Content.Type == gh.ProjectV2ItemTypeRedacted {
		logger.Info("skipping redacted item", "itemID", srcItem.ID)
		return nil, nil
	}
	marker := migratedItemMarker(srcHost, srcOwner, srcProjectNumber, srcItem.ID)
	if prev := findItemByMarker(dstCtx.items, marker); prev != nil {
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated item", "title", prev.Content.Title, "itemID", prev.ID)
			syncItemArchiveState(ctx, dst, dstCtx.projectID, prev, srcItem.IsArchived)
			return prev, nil
		}
		// A draft issue is owned by the project, so overwrite deletes and recreates it.
		// An issue or pull request is external content: keep the link and only re-apply its
		// field values, matching the content-key reuse path and the documented invariant that
		// items linked to existing issues or pull requests are never deleted on overwrite.
		if prev.Content.Type != gh.ProjectV2ItemTypeDraftIssue {
			applyItemFieldValues(ctx, dst, dstCtx, prev.ID, prev.Content.Type, srcItem.FieldValues)
			syncItemArchiveState(ctx, dst, dstCtx.projectID, prev, srcItem.IsArchived)
			return prev, nil
		}
		if err := gh.DeleteProjectV2Item(ctx, dst, dstCtx.projectID, prev.ID); err != nil {
			return nil, fmt.Errorf("failed to delete existing item '%s' for overwrite: %w", prev.Content.Title, err)
		}
		dstCtx.removeItem(prev.ID)
	}
	// An issue or pull request that is already linked to the destination project is reused as is.
	// The link is never deleted, because the content is owned by the destination repository.
	if key := expectedDstContentKey(dstOwner, &srcItem.Content); key != "" {
		if prev := dstCtx.findItemByID(dstCtx.itemByContentKey[key]); prev != nil {
			if opts != nil && opts.Overwrite {
				applyItemFieldValues(ctx, dst, dstCtx, prev.ID, prev.Content.Type, srcItem.FieldValues)
			} else {
				logger.Info("skipping already-linked item", "title", prev.Content.Title, "itemID", prev.ID)
			}
			syncItemArchiveState(ctx, dst, dstCtx.projectID, prev, srcItem.IsArchived)
			return prev, nil
		}
	}
	title, body := itemDraftContent(srcItem)
	body = body + "\n\n" + marker

	var itemID string
	var itemType gh.ProjectV2ItemType
	linked := false

	if opts != nil && opts.IssueRepo != nil {
		// Search for an existing issue that carries the migration marker.
		issues, err := gh.SearchIssues(ctx, dst, *opts.IssueRepo, fmt.Sprintf("%q", marker))
		if err != nil {
			logger.Warn("failed to search issues for migration marker", "error", err)
		} else if len(issues) > 0 {
			// Link the first matching issue to the project.
			id, err := gh.AddProjectV2ItemByID(ctx, dst, dstCtx.projectID, issues[0].GetNodeID())
			if err != nil {
				return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
			}
			itemID = id
			itemType = gh.ProjectV2ItemTypeIssue
		}
	}

	if itemID == "" {
		// Link the issue or pull request that has the same repository name and number under dstOwner.
		if nodeID := resolveDstContentNodeID(ctx, dst, dstOwner, &srcItem.Content); nodeID != "" {
			id, err := gh.AddProjectV2ItemByID(ctx, dst, dstCtx.projectID, nodeID)
			if err != nil {
				logger.Warn("failed to link existing content to destination project", "title", title, "error", err)
			} else {
				itemID = id
				itemType = srcItem.Content.Type
				linked = true
			}
		}
	}

	if itemID == "" && opts != nil && opts.IssueRepo != nil && opts.CreateIssue {
		// No existing content found and creation is enabled.
		issue, err := gh.CreateIssue(ctx, dst, *opts.IssueRepo, title, body, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create issue '%s' in repository '%s/%s': %w", title, opts.IssueRepo.Owner, opts.IssueRepo.Name, err)
		}
		id, err := gh.AddProjectV2ItemByID(ctx, dst, dstCtx.projectID, issue.GetNodeID())
		if err != nil {
			return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
		}
		itemID = id
		itemType = gh.ProjectV2ItemTypeIssue
	}

	if itemID == "" {
		// Fall back to draft issue.
		id, err := gh.AddProjectV2DraftIssue(ctx, dst, dstCtx.projectID, title, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create draft issue '%s' in destination project: %w", title, err)
		}
		itemID = id
		itemType = gh.ProjectV2ItemTypeDraftIssue
	}

	applyItemFieldValues(ctx, dst, dstCtx, itemID, itemType, srcItem.FieldValues)
	if srcItem.IsArchived {
		if err := gh.ArchiveProjectV2Item(ctx, dst, dstCtx.projectID, itemID); err != nil {
			logger.Warn("failed to archive item", "title", title, "itemID", itemID, "error", err)
		}
	}
	result := &gh.ProjectV2Item{
		ID: itemID,
		Content: gh.ProjectV2ItemContent{
			Type:  itemType,
			Title: title,
			Body:  body,
		},
		IsArchived: srcItem.IsArchived,
	}
	if linked {
		// Keep the destination content identity so a re-run finds the link again.
		result.Content = srcItem.Content
		result.Content.RepoOwner = dstOwner
	}
	dstCtx.addItem(*result)
	return result, nil
}

// applyItemFieldValues copies the source field values onto a destination item.
// Failures are logged because a single unsupported field must not abort the migration.
func applyItemFieldValues(ctx context.Context, dst *gh.GitHubClient, dstCtx *dstProjectContext, itemID string, itemType gh.ProjectV2ItemType, values []gh.ProjectV2FieldValue) {
	for _, fv := range values {
		if err := setFieldValue(ctx, dst, dstCtx, itemID, itemType, fv); err != nil {
			logger.Warn("failed to set field value", "field", fv.FieldName, "itemID", itemID, "error", err)
		}
	}
}

// resolveDstContentNodeID looks up the issue or pull request under dstOwner that has the same
// repository name and number as the source item. It returns "" when no content of the same type
// exists, so the caller can fall back to creating a new issue or a draft issue.
func resolveDstContentNodeID(ctx context.Context, dst *gh.GitHubClient, dstOwner string, content *gh.ProjectV2ItemContent) string {
	if !isLinkableContent(content) {
		return ""
	}
	repo := repository.Repository{Owner: dstOwner, Name: content.RepoName}
	ref, err := gh.GetIssueOrPullRequestNodeID(ctx, dst, repo, content.Number)
	if err != nil {
		logger.Warn("failed to look up destination issue or pull request", "repo", dstOwner+"/"+content.RepoName, "number", content.Number, "error", err)
		return ""
	}
	if ref == nil {
		return ""
	}
	if !contentTypeMatches(content.Type, ref.Typename) {
		logger.Warn("destination number exists but has a different type; not linking",
			"repo", dstOwner+"/"+content.RepoName, "number", content.Number, "typename", ref.Typename)
		return ""
	}
	return ref.ID
}

// contentTypeMatches reports whether the GraphQL type name matches the source item content type.
func contentTypeMatches(t gh.ProjectV2ItemType, typename string) bool {
	switch t {
	case gh.ProjectV2ItemTypeIssue:
		return typename == "Issue"
	case gh.ProjectV2ItemTypePullRequest:
		return typename == "PullRequest"
	default:
		return false
	}
}

// syncItemArchiveState aligns the archive state of an already-migrated destination item with the source.
func syncItemArchiveState(ctx context.Context, dst *gh.GitHubClient, projectID string, item *gh.ProjectV2Item, archived bool) {
	if item.IsArchived == archived {
		return
	}
	var err error
	if archived {
		err = gh.ArchiveProjectV2Item(ctx, dst, projectID, item.ID)
	} else {
		err = gh.UnarchiveProjectV2Item(ctx, dst, projectID, item.ID)
	}
	if err != nil {
		logger.Warn("failed to sync item archive state", "title", item.Content.Title, "itemID", item.ID, "archived", archived, "error", err)
		return
	}
	item.IsArchived = archived
}

// itemDraftContent returns the title and body for a migrated draft issue.
// For Issue and PR items, an attribution header with a link to the original is prepended.
func itemDraftContent(item *gh.ProjectV2Item) (title, body string) {
	c := &item.Content
	switch c.Type {
	case gh.ProjectV2ItemTypeDraftIssue:
		return c.Title, c.Body
	case gh.ProjectV2ItemTypeIssue:
		author := c.Author
		if author == "" {
			author = "unknown"
		}
		header := fmt.Sprintf("> *Originally posted by @%s \u2014 %s*", author, c.URL)
		return c.Title, header + "\n\n" + c.Body
	case gh.ProjectV2ItemTypePullRequest:
		author := c.Author
		if author == "" {
			author = "unknown"
		}
		header := fmt.Sprintf("> *Originally posted by @%s \u2014 %s*", author, c.URL)
		return c.Title, header + "\n\n" + c.Body
	default:
		return "(redacted)", ""
	}
}

// setFieldValue copies a single field value from the source into the destination item.
// It looks up the destination field by name in dstCtx.
func setFieldValue(ctx context.Context, dst *gh.GitHubClient, dstCtx *dstProjectContext, itemID string, itemType gh.ProjectV2ItemType, fv gh.ProjectV2FieldValue) error {
	dstField, ok := dstCtx.fieldByName[fv.FieldName]
	if !ok {
		return nil
	}
	switch fv.ValueType {
	case "TEXT":
		// The built-in Title field (DataType=TITLE) can only be updated on DraftIssues.
		if dstField.DataType == "TITLE" && itemType != gh.ProjectV2ItemTypeDraftIssue {
			return nil
		}
		return gh.SetProjectV2ItemTextValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, fv.Text)
	case "NUMBER":
		if fv.Number == nil {
			return nil
		}
		return gh.SetProjectV2ItemNumberValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, *fv.Number)
	case "DATE":
		if fv.Date == "" {
			return nil
		}
		return gh.SetProjectV2ItemDateValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, fv.Date)
	case "SINGLE_SELECT":
		// If the destination field was converted to TEXT (no options available at source), store the name as text.
		if dstField.DataType == "TEXT" {
			return gh.SetProjectV2ItemTextValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, fv.SelectName)
		}
		for _, opt := range dstField.Options {
			if opt.Name == fv.SelectName {
				return gh.SetProjectV2ItemSingleSelectValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, opt.ID)
			}
		}
		return nil
	case "MULTI_SELECT":
		if len(fv.SelectNames) == 0 {
			return nil
		}
		// If the destination field was converted to TEXT (no options available at source), store the names as text.
		if dstField.DataType == "TEXT" {
			return gh.SetProjectV2ItemTextValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, strings.Join(fv.SelectNames, ", "))
		}
		optionIDByName := make(map[string]string, len(dstField.Options))
		for _, opt := range dstField.Options {
			optionIDByName[opt.Name] = opt.ID
		}
		optionIDs := make([]string, 0, len(fv.SelectNames))
		for _, name := range fv.SelectNames {
			if id, ok := optionIDByName[name]; ok {
				optionIDs = append(optionIDs, id)
			}
		}
		if len(optionIDs) == 0 {
			return nil
		}
		return gh.SetProjectV2ItemMultiSelectValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, optionIDs)
	case "ITERATION":
		// Match by iteration title to find the corresponding destination iteration ID.
		// Past sprints live in the completed iterations, so both lists are searched.
		for _, it := range dstField.AllIterations() {
			if it.Title == fv.IterationTitle {
				return gh.SetProjectV2ItemIterationValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, it.ID)
			}
		}
		return nil
	default:
		return nil
	}
}
