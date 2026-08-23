// Package projects provides GitHub Project v2 migration logic for gh-pm-kit.
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

// MigrateOptions controls migration behaviour for GitHub Projects v2.
type MigrateOptions struct {
	// Overwrite deletes the previously-migrated item (identified by migration marker)
	// and recreates it. Without this option, already-migrated items are skipped.
	Overwrite bool
	// Prune deletes ALL destination projects that carry the source-project migration
	// marker, as well as any destination project whose title matches the source project
	// title, before creating a new destination project. Only meaningful in MigrateProject
	// (i.e. when no explicit destination project is given); ignored in MigrateProjectTo and
	// MigrateProjectItems where the destination project is fixed. This is a highly
	// destructive operation: entire projects are removed.
	Prune bool
	// PruneItems deletes all destination project items that carry the source-project migration
	// marker before migrating. Applies in all migration modes (MigrateProject,
	// MigrateProjectTo, MigrateProjectItems). This is destructive and overrides Overwrite.
	PruneItems bool
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

// findAllProjectsByMarker returns all projects whose readme contains marker.
func findAllProjectsByMarker(projects []gh.ProjectV2, marker string) []*gh.ProjectV2 {
	var result []*gh.ProjectV2
	for i := range projects {
		if projects[i].Readme != nil && strings.Contains(*projects[i].Readme, marker) {
			result = append(result, &projects[i])
		}
	}
	return result
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
	// items holds all existing draft-issue items in the destination project.
	items []gh.ProjectV2Item
}

// removeItem removes an item by ID from the cached items slice.
func (c *dstProjectContext) removeItem(itemID string) {
	for i := range c.items {
		if c.items[i].ID == itemID {
			c.items[i] = c.items[len(c.items)-1]
			c.items = c.items[:len(c.items)-1]
			return
		}
	}
}

// pruneProjectItems deletes all items in dstCtx that carry any migration marker from the
// given source project, identified by the project marker prefix. Item values are collected
// before any deletion to avoid iterator invalidation, and dstCtx.items is rebuilt
// afterwards. Must be called at most once per migration run, before the items loop.
func pruneProjectItems(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner string, srcProjectNumber int, dstCtx *dstProjectContext) error {
	prefix := projectMarkerPrefix(srcHost, srcOwner, srcProjectNumber)
	// Collect matching items first to avoid mutating the slice during iteration.
	var toDelete []gh.ProjectV2Item
	for _, item := range dstCtx.items {
		if (item.Content.Type == gh.ProjectV2ItemTypeDraftIssue || item.Content.Type == gh.ProjectV2ItemTypeIssue) &&
			strings.Contains(item.Content.Body, prefix) {
			toDelete = append(toDelete, item)
		}
	}
	if len(toDelete) == 0 {
		return nil
	}
	deleted := make(map[string]bool, len(toDelete))
	for _, item := range toDelete {
		if err := gh.DeleteProjectV2Item(ctx, dst, dstCtx.projectID, item.ID); err != nil {
			return fmt.Errorf("failed to delete item '%s' during prune: %w", item.Content.Title, err)
		}
		deleted[item.ID] = true
		logger.Info("pruned migrated item", "title", item.Content.Title, "itemID", item.ID)
	}
	// Rebuild the items cache excluding deleted entries.
	remaining := make([]gh.ProjectV2Item, 0, len(dstCtx.items)-len(toDelete))
	for _, item := range dstCtx.items {
		if !deleted[item.ID] {
			remaining = append(remaining, item)
		}
	}
	dstCtx.items = remaining
	return nil
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
	return &dstProjectContext{
		projectID:   string(project.ID),
		fieldByName: fieldByName,
		items:       items,
	}, nil
}

// migratableDataTypes are the field data types that can be migrated.
var migratableDataTypes = map[string]bool{
	"TEXT":          true,
	"NUMBER":        true,
	"DATE":          true,
	"SINGLE_SELECT": true,
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

	if opts != nil && opts.Prune {
		// Delete ALL destination projects matching by marker OR by title.
		deleted := make(map[string]bool)
		for _, p := range findAllProjectsByMarker(dstProjects, projectMarker) {
			if err := gh.DeleteProjectV2(ctx, dst, p.ID); err != nil {
				return nil, fmt.Errorf("failed to delete destination project '%s' during prune: %w", p.Title, err)
			}
			deleted[p.ID] = true
			logger.Info("pruned previously migrated project", "title", p.Title, "projectID", p.ID)
		}
		for i := range dstProjects {
			p := &dstProjects[i]
			if deleted[p.ID] {
				continue
			}
			if p.Title == srcProject.Title {
				if err := gh.DeleteProjectV2(ctx, dst, p.ID); err != nil {
					return nil, fmt.Errorf("failed to delete destination project '%s' during prune: %w", p.Title, err)
				}
				logger.Info("pruned same-title project", "title", p.Title, "projectID", p.ID)
			}
		}
	} else if prev := findProjectByMarker(dstProjects, projectMarker); prev != nil {
		// Continue with overwriteProject when either full overwrite or item pruning is requested.
		if opts == nil || (!opts.Overwrite && !opts.PruneItems) {
			logger.Info("skipping already-migrated project", "title", prev.Title, "projectID", prev.ID)
			return prev, nil
		}
		// Overwrite or item-prune: update the existing destination project in-place.
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
	dstCtx := &dstProjectContext{
		projectID:   string(dstProject.ID),
		fieldByName: dstFieldByName,
	}
	if opts != nil && opts.PruneItems {
		if err := pruneProjectItems(ctx, dst, srcHost, srcOwner, projectNumber, dstCtx); err != nil {
			return dstProject, err
		}
	}
	if _, err := migrateItems(ctx, dst, srcHost, srcOwner, projectNumber, srcItems, dstCtx, opts); err != nil {
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
// Item-level idempotency (skip / overwrite / prune) is governed by opts as in any other migration mode.
func migrateIntoProject(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcProject *gh.ProjectV2, srcFields []gh.ProjectV2Field, srcItems []gh.ProjectV2Item, prev *gh.ProjectV2, marker string, opts *MigrateOptions) (*gh.ProjectV2, error) {
	// A closed project rejects content mutations, so reopen it for the duration of the migration.
	if prev.Closed {
		if _, err := gh.SetProjectV2Closed(ctx, dst, prev.ID, false); err != nil {
			return prev, fmt.Errorf("failed to reopen destination project '%s' for migration: %w", prev.Title, err)
		}
		prev.Closed = false
	}
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
	dstCtx.fieldByName = dstFieldByName
	if opts != nil && opts.PruneItems {
		if err := pruneProjectItems(ctx, dst, srcHost, srcOwner, srcProject.Number, dstCtx); err != nil {
			return prev, err
		}
	}
	if _, err := migrateItems(ctx, dst, srcHost, srcOwner, srcProject.Number, srcItems, dstCtx, opts); err != nil {
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
// items, overwrite, or prune) is controlled by opts the same way as in MigrateProject.
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
	if opts != nil && opts.PruneItems {
		if err := pruneProjectItems(ctx, dst, srcHost, srcOwner, srcProjectNumber, dstCtx); err != nil {
			return nil, err
		}
	}
	return migrateItems(ctx, dst, srcHost, srcOwner, srcProjectNumber, srcItems, dstCtx, opts)
}

// migrateItems migrates every source item into the destination project and then restores
// the source item order. Returns the destination items in source order.
func migrateItems(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner string, srcProjectNumber int, srcItems []gh.ProjectV2Item, dstCtx *dstProjectContext, opts *MigrateOptions) ([]*gh.ProjectV2Item, error) {
	var results []*gh.ProjectV2Item
	for i := range srcItems {
		item, err := migrateItem(ctx, dst, srcHost, srcOwner, srcProjectNumber, &srcItems[i], dstCtx, opts)
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
// because there is no view update endpoint. Field references are resolved by field name.
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
	for i := range srcViews {
		v := &srcViews[i]
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
			if err := syncSingleSelectOptions(ctx, dst, dstFieldByName[f.Name], f); err != nil {
				return dstFieldByName, err
			}
			continue
		}
		dataType := f.DataType
		if dataType == "SINGLE_SELECT" && len(f.Options) == 0 {
			// A SINGLE_SELECT field without options cannot be created (API requires at least one option).
			// This can happen when the field is a built-in type on some GitHub Enterprise Server versions.
			// Fall back to TEXT so the option name can still be stored as plain text.
			logger.Info("converting SINGLE_SELECT field with no options to TEXT", "field", f.Name)
			dataType = "TEXT"
		}
		if dataType == "ITERATION" {
			if err := gh.CreateProjectV2IterationField(ctx, dst, string(dstProject.ID), f.Name, f.Iterations); err != nil {
				return dstFieldByName, fmt.Errorf("failed to create iteration field '%s' in destination project '%s': %w", f.Name, string(dstProject.Title), err)
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

// syncSingleSelectOptions adds source options missing from an existing destination SINGLE_SELECT
// field and refreshes the color/description of options matched by name. Destination-only options
// are preserved because the update mutation replaces the whole option list.
func syncSingleSelectOptions(ctx context.Context, dst *gh.GitHubClient, dstField *gh.ProjectV2Field, srcField gh.ProjectV2Field) error {
	if dstField == nil || dstField.DataType != "SINGLE_SELECT" || srcField.DataType != "SINGLE_SELECT" || len(srcField.Options) == 0 {
		return nil
	}
	srcByName := make(map[string]gh.ProjectV2SingleSelectOption, len(srcField.Options))
	for _, o := range srcField.Options {
		srcByName[o.Name] = o
	}
	changed := false
	merged := make([]gh.ProjectV2SingleSelectOption, 0, len(dstField.Options)+len(srcField.Options))
	dstByName := make(map[string]bool, len(dstField.Options))
	for _, o := range dstField.Options {
		dstByName[o.Name] = true
		if s, ok := srcByName[o.Name]; ok && (o.Color != s.Color || o.Description != s.Description) {
			o.Color = s.Color
			o.Description = s.Description
			changed = true
		}
		merged = append(merged, o)
	}
	for _, o := range srcField.Options {
		if dstByName[o.Name] {
			continue
		}
		// Clear the source option ID so the destination assigns a new one.
		o.ID = ""
		merged = append(merged, o)
		changed = true
	}
	if !changed {
		return nil
	}
	if err := gh.UpdateProjectV2FieldSingleSelectOptions(ctx, dst, dstField.ID, merged); err != nil {
		return fmt.Errorf("failed to update options of single select field '%s' in destination project: %w", dstField.Name, err)
	}
	return nil
}

// migrateItem migrates a single source item into the destination project.
// When opts.IssueRepo is set, it first searches for an existing issue with the migration
// marker in that repository and links it. If no issue is found and opts.CreateIssue is true,
// a new issue is created. Otherwise the item falls back to a draft issue.
func migrateItem(ctx context.Context, dst *gh.GitHubClient, srcHost, srcOwner string, srcProjectNumber int, srcItem *gh.ProjectV2Item, dstCtx *dstProjectContext, opts *MigrateOptions) (*gh.ProjectV2Item, error) {
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
		if err := gh.DeleteProjectV2Item(ctx, dst, dstCtx.projectID, prev.ID); err != nil {
			return nil, fmt.Errorf("failed to delete existing item '%s' for overwrite: %w", prev.Content.Title, err)
		}
		dstCtx.removeItem(prev.ID)
	}
	title, body := itemDraftContent(srcItem)
	body = body + "\n\n" + marker

	var itemID string
	var itemType gh.ProjectV2ItemType

	if opts != nil && opts.IssueRepo != nil {
		// Search for an existing issue that carries the migration marker.
		issues, err := gh.SearchIssues(ctx, dst, *opts.IssueRepo, fmt.Sprintf("%q", marker))
		if err != nil {
			logger.Warn("failed to search issues for migration marker; falling back to draft issue", "error", err)
		} else if len(issues) > 0 {
			// Link the first matching issue to the project.
			issueNodeID := issues[0].GetNodeID()
			itemID, err = gh.AddProjectV2ItemByID(ctx, dst, dstCtx.projectID, issueNodeID)
			if err != nil {
				return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
			}
			itemType = gh.ProjectV2ItemTypeIssue
		} else if opts.CreateIssue {
			// No existing issue found and creation is enabled.
			issue, err := gh.CreateIssue(ctx, dst, *opts.IssueRepo, title, body, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create issue '%s' in repository '%s/%s': %w", title, opts.IssueRepo.Owner, opts.IssueRepo.Name, err)
			}
			itemID, err = gh.AddProjectV2ItemByID(ctx, dst, dstCtx.projectID, issue.GetNodeID())
			if err != nil {
				return nil, fmt.Errorf("failed to link issue '%s' to destination project: %w", title, err)
			}
			itemType = gh.ProjectV2ItemTypeIssue
		}
	}

	if itemID == "" {
		// Fall back to draft issue.
		var err error
		itemID, err = gh.AddProjectV2DraftIssue(ctx, dst, dstCtx.projectID, title, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create draft issue '%s' in destination project: %w", title, err)
		}
		itemType = gh.ProjectV2ItemTypeDraftIssue
	}

	for _, fv := range srcItem.FieldValues {
		if err := setFieldValue(ctx, dst, dstCtx, itemID, itemType, fv); err != nil {
			logger.Warn("failed to set field value", "field", fv.FieldName, "itemID", itemID, "error", err)
		}
	}
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
	dstCtx.items = append(dstCtx.items, *result)
	return result, nil
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
	case "ITERATION":
		// Match by iteration title to find the corresponding destination iteration ID.
		for _, it := range dstField.Iterations {
			if it.Title == fv.IterationTitle {
				return gh.SetProjectV2ItemIterationValue(ctx, dst, dstCtx.projectID, itemID, dstField.ID, it.ID)
			}
		}
		return nil
	default:
		return nil
	}
}
