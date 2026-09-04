package projects

import (
	"context"
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

func draftItem(id string) gh.ProjectV2Item {
	return gh.ProjectV2Item{ID: id, Content: gh.ProjectV2ItemContent{Type: gh.ProjectV2ItemTypeDraftIssue}}
}

func assertIndexConsistent(t *testing.T, c *dstProjectContext) {
	t.Helper()
	if len(c.itemByID) != len(c.items) {
		t.Fatalf("itemByID size %d != items size %d", len(c.itemByID), len(c.items))
	}
	for id, idx := range c.itemByID {
		if idx < 0 || idx >= len(c.items) {
			t.Fatalf("index %d for %q out of range", idx, id)
		}
		if c.items[idx].ID != id {
			t.Fatalf("itemByID[%q]=%d points at %q", id, idx, c.items[idx].ID)
		}
	}
}

func TestRemoveItemMaintainsIndex(t *testing.T) {
	for _, target := range []string{"a", "b", "c", "missing"} {
		c := newDstProjectContext("proj", nil)
		c.addItem(draftItem("a"))
		c.addItem(draftItem("b"))
		c.addItem(draftItem("c"))
		c.removeItem(target)
		assertIndexConsistent(t, c)
		if target != "missing" {
			if c.findItemByID(target) != nil {
				t.Fatalf("item %q should be gone after removal", target)
			}
			if len(c.items) != 2 {
				t.Fatalf("expected 2 items after removing %q, got %d", target, len(c.items))
			}
		} else if len(c.items) != 3 {
			t.Fatalf("removing a missing item must not change items, got %d", len(c.items))
		}
	}
}

func TestRemoveLastItemDoesNotResurrect(t *testing.T) {
	c := newDstProjectContext("proj", nil)
	c.addItem(draftItem("a"))
	c.addItem(draftItem("b"))
	c.removeItem("b") // b is the last element: must not be re-added to itemByID
	if c.findItemByID("b") != nil {
		t.Fatalf("last item b must be removed, not resurrected")
	}
	assertIndexConsistent(t, c)
}

func TestFindItemByIDEmpty(t *testing.T) {
	c := newDstProjectContext("proj", nil)
	c.addItem(draftItem("a"))
	if c.findItemByID("") != nil {
		t.Fatalf("empty ID must not match any item")
	}
}

func selectOption(name, id string) gh.ProjectV2SelectOption {
	return gh.ProjectV2SelectOption{Name: name, ID: id}
}

func TestResolveMultiSelectOptionIDs(t *testing.T) {
	options := []gh.ProjectV2SelectOption{
		selectOption("Bug", "id-bug"),
		selectOption("Feature", "id-feat"),
		selectOption("Chore", "id-chore"),
	}
	// Multiple known names map to IDs in source-selection order.
	got := resolveMultiSelectOptionIDs(options, []string{"Feature", "Bug"})
	want := []string{"id-feat", "id-bug"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch: got %v, want %v", got, want)
		}
	}
	// Unknown names are dropped, known ones kept.
	got = resolveMultiSelectOptionIDs(options, []string{"Unknown", "Chore"})
	if len(got) != 1 || got[0] != "id-chore" {
		t.Fatalf("unknown names must be dropped: got %v", got)
	}
	// No matches yields an empty slice.
	if got = resolveMultiSelectOptionIDs(options, []string{"Nope"}); len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}

// TestSetFieldValueSkipsIncompatibleSelectType verifies that a source select value whose
// same-name destination field has an incompatible select type is skipped before any API call,
// preventing mutation requests that the API would reject.
func TestSetFieldValueSkipsIncompatibleSelectType(t *testing.T) {
	cases := []struct {
		name    string
		dstType string
		fv      gh.ProjectV2FieldValue
	}{
		{
			name:    "multi-select value into single-select field",
			dstType: "SINGLE_SELECT",
			fv: gh.ProjectV2FieldValue{
				FieldName:   "Labels",
				ValueType:   "MULTI_SELECT",
				SelectNames: []string{"Bug"},
			},
		},
		{
			name:    "single-select value into multi-select field",
			dstType: "MULTI_SELECT",
			fv: gh.ProjectV2FieldValue{
				FieldName:  "Labels",
				ValueType:  "SINGLE_SELECT",
				SelectName: "Bug",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dstCtx := newDstProjectContext("proj", map[string]*gh.ProjectV2Field{
				"Labels": {Name: "Labels", DataType: tc.dstType},
			})
			// dst is nil: the skip path must return before dereferencing the client.
			if err := setFieldValue(context.Background(), nil, dstCtx, "item1", gh.ProjectV2ItemTypeDraftIssue, tc.fv); err != nil {
				t.Fatalf("expected nil error on skip path, got %v", err)
			}
		})
	}
}
