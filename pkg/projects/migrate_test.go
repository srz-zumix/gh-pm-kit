package projects

import (
	"testing"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

func issueContent(owner, name string, number int) gh.ProjectV2ItemContent {
	return gh.ProjectV2ItemContent{
		Type:      gh.ProjectV2ItemTypeIssue,
		RepoOwner: owner,
		RepoName:  name,
		Number:    number,
	}
}

func TestItemContentKeyIncludesOwner(t *testing.T) {
	a := issueContent("owner-a", "api", 5)
	b := issueContent("owner-b", "api", 5)
	if itemContentKey(&a) == itemContentKey(&b) {
		t.Fatalf("keys for different owners must differ: %q", itemContentKey(&a))
	}
	if got, want := itemContentKey(&a), "owner-a/api#5"; got != want {
		t.Fatalf("itemContentKey = %q, want %q", got, want)
	}
}

func TestItemContentKeyCaseInsensitive(t *testing.T) {
	upper := issueContent("Owner-A", "API", 7)
	lower := issueContent("owner-a", "api", 7)
	if itemContentKey(&upper) != itemContentKey(&lower) {
		t.Fatalf("owner/repo matching must be case-insensitive: %q vs %q", itemContentKey(&upper), itemContentKey(&lower))
	}
}

func TestItemContentKeyEmptyWhenNotLinkable(t *testing.T) {
	draft := gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue}
	if itemContentKey(&draft) != "" {
		t.Fatalf("draft issue must not produce a content key")
	}
	noOwner := issueContent("", "api", 5)
	if itemContentKey(&noOwner) != "" {
		t.Fatalf("issue without owner must not be indexed under an ambiguous key")
	}
}

func TestExpectedDstContentKeyMapsToDstOwner(t *testing.T) {
	// A source item from any owner is expected to resolve to dstOwner/repo#number.
	src := issueContent("src-owner", "api", 5)
	dst := issueContent("dst-owner", "api", 5)
	if got, want := expectedDstContentKey("dst-owner", &src), itemContentKey(&dst); got != want {
		t.Fatalf("expectedDstContentKey = %q, want %q", got, want)
	}
	// It must not collide with an unrelated external item that shares repo name and number.
	ext := issueContent("ext-owner", "api", 5)
	if expectedDstContentKey("dst-owner", &src) == itemContentKey(&ext) {
		t.Fatalf("expected destination key must not match an external owner's item")
	}
}

func TestExpectedDstContentKeyEmptyForDraft(t *testing.T) {
	draft := gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue}
	if expectedDstContentKey("dst-owner", &draft) != "" {
		t.Fatalf("draft issue must not produce an expected destination key")
	}
}
