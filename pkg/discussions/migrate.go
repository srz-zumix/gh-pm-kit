// Package discussions provides discussion migration logic for gh-pm-kit.
package discussions

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/shurcooL/githubv4"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/gh/client"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// MigrateOptions controls migration behaviour.
type MigrateOptions struct {
	// CategorySlug overrides category matching. If empty, the source category slug is used.
	CategorySlug string
	// EnableDiscussions enables Discussions on the destination repository if not already enabled.
	EnableDiscussions bool
	// Overwrite deletes an existing discussion with the same title before migrating.
	// Without this option, an already-migrated discussion is skipped.
	Overwrite bool
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

	srcDiscussion, err := src.GetDiscussion(ctx, srcRepo.Owner, srcRepo.Name, discussionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source discussion #%d in '%s/%s': %w", discussionNumber, srcRepo.Owner, srcRepo.Name, err)
	}

	return migrateDiscussion(ctx, src, srcRepo, dst, dstRepo, srcDiscussion, opts)
}

// MigrateDiscussions migrates all discussions from src repo to dst repo.
func MigrateDiscussions(ctx context.Context, src, dst *gh.GitHubClient, srcRepo, dstRepo repository.Repository, opts *MigrateOptions) ([]*gh.Discussion, error) {
	srcDiscussions, err := src.ListDiscussions(ctx, srcRepo.Owner, srcRepo.Name, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list discussions in '%s/%s': %w", srcRepo.Owner, srcRepo.Name, err)
	}

	var results []*gh.Discussion
	for i := range srcDiscussions {
		d, err := migrateDiscussion(ctx, src, srcRepo, dst, dstRepo, &srcDiscussions[i], opts)
		if err != nil {
			return results, err
		}
		results = append(results, d)
	}
	return results, nil
}

// migrateDiscussion copies a single Discussion (with reactions and comments) to dstRepo on dst client.
func migrateDiscussion(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstRepo repository.Repository, srcDisc *gh.Discussion, opts *MigrateOptions) (*gh.Discussion, error) {
	dstRepoID, err := dst.GetRepositoryNodeID(ctx, dstRepo.Owner, dstRepo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository ID for '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}

	dstCategories, err := dst.ListDiscussionCategories(ctx, dstRepo.Owner, dstRepo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}

	slug := string(srcDisc.Category.Slug)
	if opts != nil && opts.CategorySlug != "" {
		slug = opts.CategorySlug
	}

	if len(dstCategories) == 0 {
		if opts == nil || !opts.EnableDiscussions {
			return nil, fmt.Errorf("discussions are not enabled in destination repository '%s/%s'", dstRepo.Owner, dstRepo.Name)
		}
		// Enable Discussions on the destination repository
		_, err := gh.EnableDiscussions(ctx, dst, dstRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to enable discussions in destination repository '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
		}
		// Re-fetch categories after enabling Discussions
		dstCategories, err = dst.ListDiscussionCategories(ctx, dstRepo.Owner, dstRepo.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to list categories in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
		}
	}

	dstCategory := findDiscussionCategoryBySlug(dstCategories, slug)
	if dstCategory == nil {
		availableSlugs := make([]string, len(dstCategories))
		for i, c := range dstCategories {
			availableSlugs[i] = string(c.Slug)
		}
		return nil, fmt.Errorf("category with slug %q not found in destination repository '%s/%s' (available: %s)", slug, dstRepo.Owner, dstRepo.Name, strings.Join(availableSlugs, ", "))
	}

	// Check for an existing discussion with the same title
	dstDiscussions, err := dst.ListDiscussions(ctx, dstRepo.Owner, dstRepo.Name, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list discussions in '%s/%s': %w", dstRepo.Owner, dstRepo.Name, err)
	}
	for _, d := range dstDiscussions {
		if d.Title == srcDisc.Title {
			if opts == nil || !opts.Overwrite {
				// Already migrated; skip
				logger.Info("skipping already-migrated discussion", "title", string(d.Title), "number", int(d.Number))
				return &d, nil
			}
			// Overwrite: delete existing discussion first
			if err := dst.DeleteDiscussion(ctx, string(d.ID)); err != nil {
				return nil, fmt.Errorf("failed to delete existing discussion %q in '%s/%s': %w", string(d.Title), dstRepo.Owner, dstRepo.Name, err)
			}
			break
		}
	}

	created, err := dst.CreateDiscussion(ctx, client.CreateDiscussionInput{
		RepositoryID: dstRepoID,
		CategoryID:   githubv4.ID(dstCategory.ID),
		Title:        srcDisc.Title,
		Body:         srcDisc.Body,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create discussion %q in '%s/%s': %w", string(srcDisc.Title), dstRepo.Owner, dstRepo.Name, err)
	}

	if err := migrateReactionsAndComments(ctx, src, srcRepo, dst, created, srcDisc); err != nil {
		return created, fmt.Errorf("failed to migrate reactions and comments: %w", err)
	}

	return created, nil
}

// migrateReactionsAndComments copies reactions and comments (with replies and their reactions) from a source discussion.
func migrateReactionsAndComments(ctx context.Context, src *gh.GitHubClient, srcRepo repository.Repository, dst *gh.GitHubClient, dstDisc *gh.Discussion, srcDisc *gh.Discussion) error {
	number := int(srcDisc.Number)
	dstDiscID := string(dstDisc.ID)

	// Migrate discussion-level reactions
	reactions, err := src.GetDiscussionReactions(ctx, srcRepo.Owner, srcRepo.Name, number)
	if err != nil {
		return fmt.Errorf("failed to get reactions for discussion #%d: %w", number, err)
	}
	for _, r := range uniqueReactions(reactions) {
		// Ignore errors (e.g. already reacted, or reaction not supported on destination)
		_ = dst.AddReaction(ctx, dstDiscID, string(r.Content))
	}

	// Migrate comments
	comments, err := src.ListDiscussionComments(ctx, srcRepo.Owner, srcRepo.Name, number)
	if err != nil {
		return fmt.Errorf("failed to get comments for discussion #%d: %w", number, err)
	}
	for _, comment := range comments {
		body := formatMigratedBody(string(comment.Author.Login), string(comment.Body))
		dstCommentID, err := dst.CreateDiscussionComment(ctx, dstDiscID, body)
		if err != nil {
			return fmt.Errorf("failed to create comment: %w", err)
		}
		for _, r := range uniqueReactions(comment.Reactions.Nodes) {
			_ = dst.AddReaction(ctx, dstCommentID, string(r.Content))
		}
		// Migrate replies; fetch reply reactions separately to avoid GraphQL node limit
		for _, reply := range comment.Replies.Nodes {
			replyBody := formatMigratedBody(string(reply.Author.Login), string(reply.Body))
			dstReplyID, err := dst.AddDiscussionCommentReply(ctx, dstDiscID, dstCommentID, replyBody)
			if err != nil {
				return fmt.Errorf("failed to create reply: %w", err)
			}
			replyReactions, err := src.GetNodeReactions(ctx, string(reply.ID))
			if err != nil {
				return fmt.Errorf("failed to get reactions for reply %s: %w", reply.ID, err)
			}
			for _, r := range uniqueReactions(replyReactions) {
				_ = dst.AddReaction(ctx, dstReplyID, string(r.Content))
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
func uniqueReactions(reactions []client.Reaction) []client.Reaction {
	seen := make(map[string]bool)
	var result []client.Reaction
	for _, r := range reactions {
		c := string(r.Content)
		if !seen[c] {
			seen[c] = true
			result = append(result, r)
		}
	}
	return result
}
