// Package discussions provides discussion migration logic for gh-pm-kit.
package discussions

import (
	"context"
	"crypto/sha256"
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
	repoID     githubv4.ID
	categories []gh.DiscussionCategory
	all        []gh.Discussion            // all existing discussions (scanned for markers)
	byTitle    map[string][]gh.Discussion // title → matching discussions (used for --purge only)
}

// removeDiscussion removes the discussion with the given node ID from both caches.
func (c *dstMigrationContext) removeDiscussion(id string) {
	for i := range c.all {
		if string(c.all[i].ID) == id {
			c.all[i] = c.all[len(c.all)-1]
			c.all = c.all[:len(c.all)-1]
			break
		}
	}
	for title, list := range c.byTitle {
		for i, d := range list {
			if string(d.ID) == id {
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
		t := string(d.Title)
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
		if strings.EqualFold(string(categories[i].Slug), slug) {
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

	if opts != nil && opts.Purge {
		// Purge all title-matched discussions regardless of migration marker
		if matches, ok := dstCtx.byTitle[title]; ok {
			for _, d := range matches {
				if err := gh.DeleteDiscussion(ctx, dst, d); err != nil {
					return nil, fmt.Errorf("failed to delete discussion %q in '%s/%s': %w", title, dstRepo.Owner, dstRepo.Name, err)
				}
				dstCtx.removeDiscussion(string(d.ID))
			}
		}
	} else if prev := findDiscussionByMarker(dstCtx.all, marker); prev != nil {
		// Found our previously-migrated copy (marker-based, title-independent).
		if opts == nil || !opts.Overwrite {
			logger.Info("skipping already-migrated discussion", "title", string(prev.Title), "number", int(prev.Number))
			return prev, nil
		}
		// Overwrite: delete only the previously-migrated discussion
		if err := gh.DeleteDiscussion(ctx, dst, prev); err != nil {
			return nil, fmt.Errorf("failed to delete existing discussion %q in '%s/%s': %w", string(prev.Title), dstRepo.Owner, dstRepo.Name, err)
		}
		dstCtx.removeDiscussion(string(prev.ID))
		// No marker and no purge: proceed to create alongside any other same-title discussions.
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

// findDiscussionByMarker returns a pointer to the first discussion in all whose body contains
// marker, or nil if none is found. The search is title-independent so it still works when the
// destination discussion title was edited after a previous migration run.
func findDiscussionByMarker(all []gh.Discussion, marker string) *gh.Discussion {
	for i := range all {
		if isMigratedDiscussion(string(all[i].Body), marker) {
			return &all[i]
		}
	}
	return nil
}

// migrateReactionsAndComments copies reactions and comments (with replies and their reactions) from a source discussion.
func migrateReactionsAndComments(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstDisc *gh.Discussion, srcDisc *gh.Discussion) error {
	number := int(srcDisc.Number)

	// Migrate discussion-level reactions as a summary comment
	reactions, err := gh.GetDiscussionReactions(ctx, src, srcRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get reactions for discussion #%d: %w", number, err)
	}
	if len(reactions) > 0 {
		if _, err := gh.CreateDiscussionComment(ctx, dst, dstDisc, formatReactionsComment(reactions)); err != nil {
			logger.Warn("failed to post reaction summary for discussion", "discussion", number, "error", err)
		}
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
		if len(comment.Reactions.Nodes) > 0 {
			replyBody := formatReactionsComment(comment.Reactions.Nodes)
			if _, err := gh.AddDiscussionCommentReply(ctx, dst, dstDisc, dstCommentID, replyBody); err != nil {
				logger.Warn("failed to post reaction summary for comment", "comment", string(comment.ID), "error", err)
			}
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
				logger.Warn("failed to get reactions for reply", "reply", string(reply.ID), "error", err)
				continue
			}
			if len(replyReactions) > 0 {
				reactionBody := formatReactionsComment(replyReactions)
				if _, err := gh.AddDiscussionCommentReply(ctx, dst, dstDisc, dstReplyID, reactionBody); err != nil {
					logger.Warn("failed to post reaction summary for reply", "reply", string(reply.ID), "error", err)
				}
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
		emoji := reactionEmoji(string(r.Content))
		login := string(r.User.Login)
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
