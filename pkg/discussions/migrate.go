// Package discussions provides discussion migration logic for gh-pm-kit.
package discussions

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// MigrateOptions controls migration behaviour.
type MigrateOptions struct {
	// CategorySlug overrides category matching. If empty, the source category slug is used.
	CategorySlug string
	// EnableDiscussions enables Discussions on the destination repository if not already enabled.
	EnableDiscussions bool
	// Overwrite updates the previously-migrated discussion in place (identified by marker) and replaces its comments.
	// Without this option, an already-migrated discussion is skipped.
	Overwrite bool
	// Prune deletes ALL discussions matching the source title before migrating.
	// This is a destructive operation and overrides Overwrite.
	Prune bool
	// IncludeReactions embeds reaction summaries into migrated discussion and comment bodies.
	IncludeReactions bool
}

// migratedFromMarker returns the hidden HTML comment embedded in migrated discussion bodies
// to identify the migration source, enabling idempotent re-runs.
// The source repository identity is SHA-256 hashed to avoid leaking host/owner/repo names.
func migratedFromMarker(srcRepo repository.Repository, number int) string {
	repoKey := fmt.Sprintf("%s:%s/%s", srcRepo.Host, srcRepo.Owner, srcRepo.Name)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(repoKey)))
	return fmt.Sprintf("<!-- gh-pm-kit:migrated-from:%s#%d -->", hash, number)
}

// isMigratedDiscussion reports whether a discussion body contains the given migration marker.
func isMigratedDiscussion(body, marker string) bool {
	return strings.Contains(body, marker)
}

// dstMigrationContext holds pre-fetched destination state shared across multiple migrations.
// This avoids redundant API calls when migrating many discussions in a loop.
type dstMigrationContext struct {
	repoID     string
	categories []gh.DiscussionCategory
	all        []gh.Discussion            // all existing discussions (scanned for markers)
	byTitle    map[string][]gh.Discussion // title → matching discussions (used for --prune only)
}

// removeDiscussion removes the discussion with the given node ID from both caches.
func (c *dstMigrationContext) removeDiscussion(id string) {
	for i := range c.all {
		if c.all[i].ID == id {
			c.all[i] = c.all[len(c.all)-1]
			c.all = c.all[:len(c.all)-1]
			break
		}
	}
	for title, list := range c.byTitle {
		for i, d := range list {
			if d.ID == id {
				list[i] = list[len(list)-1]
				list = list[:len(list)-1]
				if len(list) == 0 {
					delete(c.byTitle, title)
				} else {
					c.byTitle[title] = list
				}
				return
			}
		}
	}
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
		t := d.Title
		byTitle[t] = append(byTitle[t], d)
	}

	return &dstMigrationContext{
		repoID:     repoID,
		categories: categories,
		all:        existing,
		byTitle:    byTitle,
	}, nil
}

// findDiscussionCategoryBySlug returns the category matching slug (case-insensitive).
func findDiscussionCategoryBySlug(categories []gh.DiscussionCategory, slug string) *gh.DiscussionCategory {
	for i := range categories {
		if strings.EqualFold(categories[i].Slug, slug) {
			return &categories[i]
		}
	}
	return nil
}

// MigrateDiscussion migrates a single discussion from src repo to dst repo.
// It copies the title, body, category, reactions, and comments (with replies and reactions).
// src and dst may be different hosts (GHE ↔ github.com).
// number must be a discussion number (e.g. "42") or a discussion URL.
func MigrateDiscussion(ctx context.Context, src, dst *gh.GitHubClient, srcRepo, dstRepo repository.Repository, number string, opts *MigrateOptions) (*gh.Discussion, error) {
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
	slug := srcDisc.Category.Slug
	if opts != nil && opts.CategorySlug != "" {
		slug = opts.CategorySlug
	}

	dstCategory := findDiscussionCategoryBySlug(dstCtx.categories, slug)
	if dstCategory == nil {
		availableSlugs := make([]string, len(dstCtx.categories))
		for i, c := range dstCtx.categories {
			availableSlugs[i] = c.Slug
		}
		return nil, fmt.Errorf("category with slug %q not found in destination repository '%s/%s' (available: %s)", slug, dstRepo.Owner, dstRepo.Name, strings.Join(availableSlugs, ", "))
	}

	marker := migratedFromMarker(srcRepo, srcDisc.Number)
	title := srcDisc.Title

	if opts != nil && opts.Prune {
		// Prune all title-matched discussions regardless of migration marker
		if matches, ok := dstCtx.byTitle[title]; ok {
			for _, d := range matches {
				if err := gh.DeleteDiscussion(ctx, dst, d); err != nil {
					return nil, fmt.Errorf("failed to delete discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
				}
				dstCtx.removeDiscussion(d.ID)
			}
		}
	} else if prev := findDiscussionByMarker(dstCtx.all, marker); prev != nil {
		// Found our previously-migrated copy (marker-based, title-independent).
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated discussion", "title", prev.Title, "number", prev.Number)
			copyPrev := *prev
			return &copyPrev, nil
		}
		// Overwrite: update the discussion body and rebuild comments in-place.
		return overwriteDiscussion(ctx, src, srcRepo, dst, dstRepo, prev, srcDisc, dstCtx, opts)
	}

	discBody, err := buildDiscBody(ctx, src, srcRepo, srcDisc, marker, opts)
	if err != nil {
		return nil, err
	}

	created, err := gh.CreateDiscussion(ctx, dst, gh.CreateDiscussionOption{
		RepositoryID: dstCtx.repoID,
		CategoryID:   dstCategory.ID,
		Title:        srcDisc.Title,
		Body:         discBody,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
	}

	if err := migrateReactionsAndComments(ctx, src, srcRepo, dst, created, srcDisc, opts); err != nil {
		return created, fmt.Errorf("failed to migrate reactions and comments: %w", err)
	}

	return created, nil
}

// findDiscussionByMarker returns a pointer to the first discussion in all whose body contains
// marker, or nil if none is found. The search is title-independent so it still works when the
// destination discussion title was edited after a previous migration run.
func findDiscussionByMarker(all []gh.Discussion, marker string) *gh.Discussion {
	for i := range all {
		if isMigratedDiscussion(all[i].Body, marker) {
			return &all[i]
		}
	}
	return nil
}

// overwriteDiscussion updates the body of prev in place, deletes all its existing comments,
// and re-populates them from srcDisc. The discussion node itself is preserved.
func overwriteDiscussion(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstRepo repository.Repository, prev *gh.Discussion, srcDisc *gh.Discussion, dstCtx *dstMigrationContext, opts *MigrateOptions) (*gh.Discussion, error) {
	marker := migratedFromMarker(srcRepo, int(srcDisc.Number))

	discBody, err := buildDiscBody(ctx, src, srcRepo, srcDisc, marker, opts)
	if err != nil {
		return nil, err
	}

	if err := gh.UpdateDiscussion(ctx, dst, prev, discBody); err != nil {
		return nil, fmt.Errorf("failed to update discussion %q in '%s/%s': %w", prev.Title, dstRepo.Owner, dstRepo.Name, err)
	}

	// Delete all existing comments (and their replies) on the destination discussion.
	// Replies are deleted explicitly before the parent because GitHub does not guarantee
	// cascade deletion of replies via the API.
	existingComments, err := gh.ListDiscussionComments(ctx, dst, dstRepo, prev.Number)
	if err != nil {
		return nil, fmt.Errorf("failed to list comments for existing discussion %q: %w", prev.Title, err)
	}
	for _, c := range existingComments {
		for _, r := range c.Replies.Nodes {
			if err := gh.DeleteDiscussionComment(ctx, dst, r.ID); err != nil {
				logger.Warn("failed to delete reply during overwrite", "reply", r.ID, "error", err)
			}
		}
		// Verify the parent comment still exists before deleting. GitHub may have
		// cascade-deleted it when its last reply was removed (e.g. corrupted state
		// from a previous run), so skip silently if already gone.
		exists, err := gh.DiscussionCommentExists(ctx, dst, c.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check comment %s on discussion %q: %w", c.ID, prev.Title, err)
		}
		if !exists {
			continue
		}
		if err := gh.DeleteDiscussionComment(ctx, dst, c.ID); err != nil {
			return nil, fmt.Errorf("failed to delete comment %s on discussion %q: %w", c.ID, prev.Title, err)
		}
	}

	// Re-populate comments from source
	if err := migrateReactionsAndComments(ctx, src, srcRepo, dst, prev, srcDisc, opts); err != nil {
		return prev, fmt.Errorf("failed to migrate reactions and comments: %w", err)
	}

	return prev, nil
}

// buildDiscBody builds the body string for a migrated discussion, optionally including a
// reaction summary and always appending the idempotency marker.
func buildDiscBody(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, srcDisc *gh.Discussion, marker string, opts *MigrateOptions) (string, error) {
	body := srcDisc.Body
	if opts != nil && opts.IncludeReactions {
		discReactions, err := gh.GetDiscussionReactions(ctx, src, srcRepo, srcDisc.Number)
		if err != nil {
			return "", fmt.Errorf("failed to get reactions for discussion #%d in '%s/%s': %w", srcDisc.Number, srcRepo.Owner, srcRepo.Name, err)
		}
		if len(discReactions) > 0 {
			body += "\n\n" + formatReactionsComment(discReactions)
		}
	}
	body += "\n\n" + marker
	return body, nil
}

// migrateReactionsAndComments copies comments (with replies) from a source discussion.
// When opts.IncludeReactions is true, reaction summaries are embedded into each body.
func migrateReactionsAndComments(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstDisc *gh.Discussion, srcDisc *gh.Discussion, opts *MigrateOptions) error {
	number := srcDisc.Number
	includeReactions := opts != nil && opts.IncludeReactions

	// Migrate comments
	comments, err := gh.ListDiscussionComments(ctx, src, srcRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get comments for discussion #%d: %w", number, err)
	}
	for _, comment := range comments {
		var commentReactions []gh.Reaction
		if includeReactions {
			commentReactions = comment.Reactions.Nodes
		}
		body := formatMigratedBody(comment.Author.Login, comment.Body, commentReactions)
		dstCommentID, err := gh.CreateDiscussionComment(ctx, dst, dstDisc, body)
		if err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}
		// Migrate replies; fetch reply reactions separately to avoid GraphQL node limit
		for _, reply := range comment.Replies.Nodes {
			var replyReactions []gh.Reaction
			if includeReactions {
				replyReactions, err = gh.GetNodeReactions(ctx, src, reply.ID)
				if err != nil {
					logger.Warn("failed to get reactions for reply", "reply", reply.ID, "error", err)
				}
			}
			replyBody := formatMigratedBody(reply.Author.Login, reply.Body, replyReactions)
			if _, err := gh.AddDiscussionCommentReply(ctx, dst, dstDisc, dstCommentID, replyBody); err != nil {
				return fmt.Errorf("failed to create reply: %w", err)
			}
		}
	}
	return nil
}

// formatMigratedBody prepends an attribution header and appends an optional reaction summary
// to a migrated comment or reply body.
func formatMigratedBody(authorLogin, body string, reactions []gh.Reaction) string {
	if authorLogin == "" {
		authorLogin = "unknown"
	}
	s := fmt.Sprintf("> *Originally posted by @%s*\n\n%s", authorLogin, body)
	if len(reactions) > 0 {
		s += "\n\n" + formatReactionsComment(reactions)
	}
	return s
}

// formatReactionsComment formats a list of reactions as a summary comment.
// Each emoji is listed with the usernames of those who reacted.
func formatReactionsComment(reactions []gh.Reaction) string {
	type entry struct {
		emoji string
		users []string
	}
	order := []string{}
	byEmoji := map[string]*entry{}
	for _, r := range reactions {
		emoji := reactionEmoji(r.Content)
		login := r.User.Login
		if e, ok := byEmoji[emoji]; ok {
			e.users = append(e.users, login)
		} else {
			byEmoji[emoji] = &entry{emoji: emoji, users: []string{login}}
			order = append(order, emoji)
		}
	}
	var sb strings.Builder
	sb.WriteString("> *Reactions from original discussion:*")
	for _, emoji := range order {
		e := byEmoji[emoji]
		users := make([]string, len(e.users))
		for i, u := range e.users {
			users[i] = "@" + u
		}
		fmt.Fprintf(&sb, "\n> %s %s", emoji, strings.Join(users, ", "))
	}
	return sb.String()
}

// reactionEmoji converts a GitHub reaction content string to its emoji representation.
func reactionEmoji(content string) string {
	switch content {
	case "THUMBS_UP":
		return "👍"
	case "THUMBS_DOWN":
		return "👎"
	case "LAUGH":
		return "😄"
	case "HOORAY":
		return "🎉"
	case "CONFUSED":
		return "😕"
	case "HEART":
		return "❤️"
	case "ROCKET":
		return "🚀"
	case "EYES":
		return "👀"
	default:
		return content
	}
}
