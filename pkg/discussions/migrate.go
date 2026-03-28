// Package discussions provides discussion migration logic for gh-pm-kit.
package discussions

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/shurcooL/githubv4"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// MigrateOptions controls migration behaviour.
type MigrateOptions struct {
	// CategorySlug overrides category matching. If empty, the source category slug is used.
	CategorySlug string
	// EnableDiscussions enables Discussions on the destination repository if not already enabled.
	EnableDiscussions bool
	// Overwrite deletes the previously-migrated discussion (identified by marker) and recreates it.
	// Without this option, an already-migrated discussion is skipped.
	Overwrite bool
	// Purge deletes ALL discussions matching the source title before migrating.
	// This is a destructive operation and overrides Overwrite.
	Purge bool
}

// migratedFromMarker returns the hidden HTML comment embedded in migrated discussion bodies
// to identify the migration source, enabling idempotent re-runs.
func migratedFromMarker(srcRepo repository.Repository, number int) string {
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-from:%s/%s#%d -->", srcRepo.Owner, srcRepo.Name, number)
}

// isMigratedDiscussion reports whether a discussion body contains the given migration marker.
func isMigratedDiscussion(body, marker string) bool {
	return strings.Contains(body, marker)
}

// dstMigrationContext holds pre-fetched destination state shared across multiple migrations.
// This avoids redundant API calls when migrating many discussions in a loop.
type dstMigrationContext struct {
	repoID     githubv4.ID
	categories []gh.DiscussionCategory
	byTitle    map[string][]gh.Discussion // title → all matching discussions
}

// prepareDstContext fetches the destination repository node ID, discussion categories
// (enabling Discussions if requested), and builds a title→discussions lookup once.
func prepareDstContext(ctx context.Context, dst *gh.GitHubClient, dstRepo repository.Repository, opts *MigrateOptions) (*dstMigrationContext, error) {
	repoID, err := gh.GetRepositoryNodeID(ctx, dst, dstRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository ID for '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}

	categories, err := gh.ListDiscussionCategories(ctx, dst, dstRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}

	if len(categories) == 0 {
		if opts == nil || !opts.EnableDiscussions {
			return nil, fmt.Errorf("discussions are not enabled in destination repository '%s/%s'", dstRepo.Owner, dstRepo.Name)
		}
		// Enable Discussions on the destination repository
		if _, err := gh.EnableDiscussions(ctx, dst, dstRepo); err != nil {
			return nil, fmt.Errorf("failed to enable discussions in destination repository '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
		}
		// Re-fetch categories after enabling Discussions
		categories, err = gh.ListDiscussionCategories(ctx, dst, dstRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to list categories in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
		}
	}

	existing, err := gh.ListDiscussions(ctx, dst, dstRepo, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list discussions in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}
	byTitle := make(map[string][]gh.Discussion, len(existing))
	for _, d := range existing {
		t := string(d.Title)
		byTitle[t] = append(byTitle[t], d)
	}

	return &dstMigrationContext{
		repoID:     repoID,
		categories: categories,
		byTitle:    byTitle,
	}, nil
}

// findDiscussionCategoryBySlug returns the category matching slug (case-insensitive).
func findDiscussionCategoryBySlug(categories []gh.DiscussionCategory, slug string) *gh.DiscussionCategory {
	for i := range categories {
		if strings.EqualFold(string(categories[i].Slug), slug) {
			return &categories[i]
		}
	}
	return nil
}

// MigrateDiscussion migrates a single discussion from src repo to dst repo.
// It copies the title, body, category, reactions, and comments (with replies and reactions).
// src and dst may be different hosts (GHE ↔ github.com).
func MigrateDiscussion(ctx context.Context, src, dst *gh.GitHubClient, srcRepo, dstRepo repository.Repository, number any, opts *MigrateOptions) (*gh.Discussion, error) {
	discussionNumber, err := gh.GetDiscussionNumber(number)
	if err != nil {
		return nil, fmt.Errorf("failed to parse discussion number: %w", err)
	}

	srcDiscussion, err := gh.GetDiscussionByNumber(ctx, src, srcRepo, discussionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source discussion #%d in '%s/%s': %w", discussionNumber, srcRepo.Owner, srcRepo.Name, err)
	}

	dstCtx, err := prepareDstContext(ctx, dst, dstRepo, opts)
	if err != nil {
		return nil, err
	}

	return migrateDiscussion(ctx, src, srcRepo, dst, dstRepo, srcDiscussion, dstCtx, opts)
}

// MigrateDiscussions migrates all discussions from src repo to dst repo.
// It prefetches destination repository state once before iterating source discussions.
func MigrateDiscussions(ctx context.Context, src, dst *gh.GitHubClient, srcRepo, dstRepo repository.Repository, opts *MigrateOptions) ([]*gh.Discussion, error) {
	srcDiscussions, err := gh.ListDiscussions(ctx, src, srcRepo, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list discussions in '%s/%s': %w", srcRepo.Owner, srcRepo.Name, err)
	}

	dstCtx, err := prepareDstContext(ctx, dst, dstRepo, opts)
	if err != nil {
		return nil, err
	}

	var results []*gh.Discussion
	for i := range srcDiscussions {
		d, err := migrateDiscussion(ctx, src, srcRepo, dst, dstRepo, &srcDiscussions[i], dstCtx, opts)
		if err != nil {
			return results, err
		}
		results = append(results, d)
	}
	return results, nil
}

// migrateDiscussion copies srcDisc to dstRepo using pre-fetched dstCtx.
func migrateDiscussion(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstRepo repository.Repository, srcDisc *gh.Discussion, dstCtx *dstMigrationContext, opts *MigrateOptions) (*gh.Discussion, error) {
	slug := string(srcDisc.Category.Slug)
	if opts != nil && opts.CategorySlug != "" {
		slug = opts.CategorySlug
	}

	dstCategory := findDiscussionCategoryBySlug(dstCtx.categories, slug)
	if dstCategory == nil {
		availableSlugs := make([]string, len(dstCtx.categories))
		for i, c := range dstCtx.categories {
			availableSlugs[i] = string(c.Slug)
		}
		return nil, fmt.Errorf("category with slug %q not found in destination repository '%s/%s' (available: %s)", slug, dstRepo.Owner, dstRepo.Name, strings.Join(availableSlugs, ", "))
	}

	marker := migratedFromMarker(srcRepo, int(srcDisc.Number))
	title := string(srcDisc.Title)

	if matches, ok := dstCtx.byTitle[title]; ok {
		if opts != nil && opts.Purge {
			// Purge all title-matched discussions regardless of migration marker
			for _, d := range matches {
				if err := gh.DeleteDiscussion(ctx, dst, d); err != nil {
					return nil, fmt.Errorf("failed to delete discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
				}
			}
			delete(dstCtx.byTitle, title)
		} else if idx := findMigratedDiscussionIndex(matches, marker); idx >= 0 {
			if opts == nil || !opts.Overwrite {
				// Already migrated by us; skip
				d := &matches[idx]
				logger.Info("skipping already-migrated discussion", "title", title, "number", int(d.Number))
				return d, nil
			}
			// Overwrite: delete only the previously-migrated discussion
			if err := gh.DeleteDiscussion(ctx, dst, matches[idx]); err != nil {
				return nil, fmt.Errorf("failed to delete existing discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
			}
			// Remove the deleted entry from the cache
			remaining := make([]gh.Discussion, 0, len(matches)-1)
			for i, d := range matches {
				if i != idx {
					remaining = append(remaining, d)
				}
			}
			if len(remaining) == 0 {
				delete(dstCtx.byTitle, title)
			} else {
				dstCtx.byTitle[title] = remaining
			}
		}
		// No marker match and no purge: other discussions share this title but weren't migrated
		// by us; proceed to create a new migration alongside them.
	}

	created, err := gh.CreateDiscussion(ctx, dst, gh.CreateDiscussionInput{
		RepositoryID: dstCtx.repoID,
		CategoryID:   githubv4.ID(dstCategory.ID),
		Title:        srcDisc.Title,
		Body:         githubv4.String(string(srcDisc.Body) + "\n\n" + marker),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
	}

	if err := migrateReactionsAndComments(ctx, src, srcRepo, dst, created, srcDisc); err != nil {
		return created, fmt.Errorf("failed to migrate reactions and comments: %w", err)
	}

	return created, nil
}

// findMigratedDiscussionIndex returns the index in discussions whose body contains marker,
// or -1 if none is found.
func findMigratedDiscussionIndex(discussions []gh.Discussion, marker string) int {
	for i := range discussions {
		if isMigratedDiscussion(string(discussions[i].Body), marker) {
			return i
		}
	}
	return -1
}

// migrateReactionsAndComments copies reactions and comments (with replies and their reactions) from a source discussion.
func migrateReactionsAndComments(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstDisc *gh.Discussion, srcDisc *gh.Discussion) error {
	number := int(srcDisc.Number)

	// Migrate discussion-level reactions
	reactions, err := gh.GetDiscussionReactions(ctx, src, srcRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get reactions for discussion #%d: %w", number, err)
	}
	for _, r := range uniqueReactions(reactions) {
		// Ignore errors (e.g. already reacted, or reaction not supported on destination)
		_ = gh.AddReaction(ctx, dst, dstDisc, string(r.Content))
	}

	// Migrate comments
	comments, err := gh.ListDiscussionComments(ctx, src, srcRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get comments for discussion #%d: %w", number, err)
	}
	for _, comment := range comments {
		body := formatMigratedBody(string(comment.Author.Login), string(comment.Body))
		dstCommentID, err := gh.CreateDiscussionComment(ctx, dst, dstDisc, body)
		if err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}
		for _, r := range uniqueReactions(comment.Reactions.Nodes) {
			_ = gh.AddReaction(ctx, dst, dstCommentID, string(r.Content))
		}
		// Migrate replies; fetch reply reactions separately to avoid GraphQL node limit
		for _, reply := range comment.Replies.Nodes {
			replyBody := formatMigratedBody(string(reply.Author.Login), string(reply.Body))
			dstReplyID, err := gh.AddDiscussionCommentReply(ctx, dst, dstDisc, dstCommentID, replyBody)
			if err != nil {
				return fmt.Errorf("failed to create reply: %w", err)
			}
			replyReactions, err := gh.GetNodeReactions(ctx, src, string(reply.ID))
			if err != nil {
				return fmt.Errorf("failed to get reactions for reply %s: %w", reply.ID, err)
			}
			for _, r := range uniqueReactions(replyReactions) {
				_ = gh.AddReaction(ctx, dst, dstReplyID, string(r.Content))
			}
		}
	}
	return nil
}

// formatMigratedBody prepends an attribution header to a migrated comment or reply body.
func formatMigratedBody(authorLogin, body string) string {
	if authorLogin == "" {
		authorLogin = "unknown"
	}
	return fmt.Sprintf("> *Originally posted by @%s*\n\n%s", authorLogin, body)
}

// uniqueReactions deduplicates reactions by content, keeping the first occurrence.
func uniqueReactions(reactions []gh.Reaction) []gh.Reaction {
	seen := make(map[string]bool)
	var result []gh.Reaction
	for _, r := range reactions {
		c := string(r.Content)
		if !seen[c] {
			seen[c] = true
			result = append(result, r)
		}
	}
	return result
}
