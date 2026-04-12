// Package projects provides GitHub Project v2 diff logic for gh-pm-kit.
package projects

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/srz-zumix/gh-pm-kit/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// DiffProjects compares a source and destination GitHub Project v2.
// Items are matched using the migration markers embedded during migration.
// Returns a ProjectDiffReport suitable for rendering.
func DiffProjects(ctx context.Context, src, dst *gh.GitHubClient, srcHost, srcOwner, dstOwner string, srcNumber, dstNumber int) (*render.ProjectDiffReport, error) {
	srcProject, err := gh.GetProjectV2ByNumber(ctx, src, srcOwner, srcNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get source project #%d for '%s': %w", srcNumber, srcOwner, err)
	}
	srcFields, err := gh.ListProjectV2Fields(ctx, src, srcOwner, srcNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for source project #%d of '%s': %w", srcNumber, srcOwner, err)
	}
	srcItems, err := gh.ListProjectV2Items(ctx, src, srcOwner, srcNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for source project #%d of '%s': %w", srcNumber, srcOwner, err)
	}

	dstProject, err := gh.GetProjectV2ByNumber(ctx, dst, dstOwner, dstNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get destination project #%d for '%s': %w", dstNumber, dstOwner, err)
	}
	dstFields, err := gh.ListProjectV2Fields(ctx, dst, dstOwner, dstNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list fields for destination project #%d of '%s': %w", dstNumber, dstOwner, err)
	}
	dstItems, err := gh.ListProjectV2Items(ctx, dst, dstOwner, dstNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list items for destination project #%d of '%s': %w", dstNumber, dstOwner, err)
	}

	return &render.ProjectDiffReport{
		SrcLabel: fmt.Sprintf("#%d %s", srcProject.Number, srcOwner),
		DstLabel: fmt.Sprintf("#%d %s", dstProject.Number, dstOwner),
		Fields:   diffProjectFields(srcFields, dstFields),
		Items:    diffProjectItems(srcHost, srcOwner, srcNumber, srcItems, dstItems),
	}, nil
}

// diffProjectFields returns diffs for all migratable fields between src and dst.
func diffProjectFields(srcFields, dstFields []gh.ProjectV2Field) []render.ProjectFieldDiffEntry {
	dstByName := make(map[string]*gh.ProjectV2Field, len(dstFields))
	for i := range dstFields {
		dstByName[dstFields[i].Name] = &dstFields[i]
	}

	var diffs []render.ProjectFieldDiffEntry
	matched := make(map[string]bool)

	for i := range srcFields {
		sf := &srcFields[i]
		if !migratableDataTypes[sf.DataType] {
			continue
		}
		if df, ok := dstByName[sf.Name]; ok {
			matched[sf.Name] = true
			status := render.ProjectDiffStatusEqual
			if !projectFieldsEqual(sf, df) {
				status = render.ProjectDiffStatusModified
			}
			diffs = append(diffs, render.ProjectFieldDiffEntry{
				Status:   status,
				Name:     sf.Name,
				DataType: sf.DataType,
			})
		} else {
			diffs = append(diffs, render.ProjectFieldDiffEntry{
				Status:   render.ProjectDiffStatusSrcOnly,
				Name:     sf.Name,
				DataType: sf.DataType,
			})
		}
	}

	// Collect dst-only fields.
	for i := range dstFields {
		df := &dstFields[i]
		if !migratableDataTypes[df.DataType] || matched[df.Name] {
			continue
		}
		diffs = append(diffs, render.ProjectFieldDiffEntry{
			Status:   render.ProjectDiffStatusDstOnly,
			Name:     df.Name,
			DataType: df.DataType,
		})
	}

	return diffs
}

// projectFieldsEqual reports whether two project fields have the same type and option names.
func projectFieldsEqual(a, b *gh.ProjectV2Field) bool {
	if a.DataType != b.DataType {
		return false
	}
	if a.DataType == "SINGLE_SELECT" {
		if len(a.Options) != len(b.Options) {
			return false
		}
		for i := range a.Options {
			if a.Options[i].Name != b.Options[i].Name {
				return false
			}
		}
	}
	return true
}

// diffProjectItems returns diffs for all items, matched via migration markers.
// dstItems is indexed by marker string once upfront to avoid O(|src|*|dst|) scanning.
func diffProjectItems(srcHost, srcOwner string, srcProjectNumber int, srcItems, dstItems []gh.ProjectV2Item) []render.ProjectItemDiffEntry {
	// Build a map from expected migration marker -> *ProjectV2Item for dst items.
	// Each dst item's body may contain at most one marker for a given source project,
	// so we scan dstItems once and store pointers for O(1) lookup during the src loop.
	prefix := projectMarkerPrefix(srcHost, srcOwner, srcProjectNumber)
	dstByMarker := make(map[string]*gh.ProjectV2Item, len(dstItems))
	for i := range dstItems {
		di := &dstItems[i]
		t := di.Content.Type
		if t != gh.ProjectV2ItemTypeDraftIssue && t != gh.ProjectV2ItemTypeIssue {
			continue
		}
		// Extract the exact marker token from the body so the map key matches
		// what migratedItemMarker() produces for a given source item ID.
		start := strings.Index(di.Content.Body, prefix)
		if start == -1 {
			continue
		}
		end := strings.Index(di.Content.Body[start:], " -->")
		if end == -1 {
			continue
		}
		marker := di.Content.Body[start : start+end+4] // includes " -->"
		dstByMarker[marker] = di
	}

	var diffs []render.ProjectItemDiffEntry
	matchedDstIDs := make(map[string]bool)

	for i := range srcItems {
		si := &srcItems[i]
		marker := migratedItemMarker(srcHost, srcOwner, srcProjectNumber, si.ID)
		di := dstByMarker[marker]
		if di == nil {
			diffs = append(diffs, render.ProjectItemDiffEntry{
				Status:   render.ProjectDiffStatusSrcOnly,
				SrcTitle: si.Content.Title,
			})
		} else {
			matchedDstIDs[di.ID] = true
			fvDiffs := diffItemFieldValues(si, di)
			status := render.ProjectDiffStatusEqual
			if si.Content.Title != di.Content.Title || si.IsArchived != di.IsArchived || len(fvDiffs) > 0 {
				status = render.ProjectDiffStatusModified
			}
			diffs = append(diffs, render.ProjectItemDiffEntry{
				Status:      status,
				SrcTitle:    si.Content.Title,
				DstTitle:    di.Content.Title,
				SrcArchived: si.IsArchived,
				DstArchived: di.IsArchived,
				FieldDiffs:  fvDiffs,
			})
		}
	}

	// Collect dst-only items: those that do not carry any migration marker for this source project.
	for i := range dstItems {
		di := &dstItems[i]
		if matchedDstIDs[di.ID] || strings.Contains(di.Content.Body, prefix) {
			continue
		}
		diffs = append(diffs, render.ProjectItemDiffEntry{
			Status:   render.ProjectDiffStatusDstOnly,
			DstTitle: di.Content.Title,
		})
	}

	return diffs
}

// diffItemFieldValues compares migratable field values between two project items.
// It builds value maps for both src and dst, then iterates over the union of their
// migratable field names so that fields present only in dst are also reported.
func diffItemFieldValues(src, dst *gh.ProjectV2Item) []render.ProjectFieldValueDiff {
	srcFVMap := make(map[string]string, len(src.FieldValues))
	for _, fv := range src.FieldValues {
		if migratableDataTypes[fv.ValueType] {
			srcFVMap[fv.FieldName] = projectFieldValueString(fv)
		}
	}

	dstFVMap := make(map[string]string, len(dst.FieldValues))
	for _, fv := range dst.FieldValues {
		if migratableDataTypes[fv.ValueType] {
			dstFVMap[fv.FieldName] = projectFieldValueString(fv)
		}
	}

	// Build the union of migratable field names for a complete comparison.
	fieldNames := make(map[string]struct{}, len(srcFVMap)+len(dstFVMap))
	for name := range srcFVMap {
		fieldNames[name] = struct{}{}
	}
	for name := range dstFVMap {
		fieldNames[name] = struct{}{}
	}

	// Sort names to produce deterministic output.
	sorted := make([]string, 0, len(fieldNames))
	for name := range fieldNames {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var diffs []render.ProjectFieldValueDiff
	for _, name := range sorted {
		srcVal := srcFVMap[name]
		dstVal := dstFVMap[name]
		if srcVal != dstVal {
			diffs = append(diffs, render.ProjectFieldValueDiff{
				FieldName: name,
				SrcValue:  srcVal,
				DstValue:  dstVal,
			})
		}
	}
	return diffs
}

// projectFieldValueString formats a ProjectV2FieldValue as a comparable string.
func projectFieldValueString(fv gh.ProjectV2FieldValue) string {
	switch fv.ValueType {
	case "TEXT":
		return fv.Text
	case "NUMBER":
		if fv.Number != nil {
			return fmt.Sprintf("%g", *fv.Number)
		}
		return ""
	case "DATE":
		return fv.Date
	case "SINGLE_SELECT":
		return fv.SelectName
	case "ITERATION":
		return fv.IterationTitle
	}
	return ""
}
