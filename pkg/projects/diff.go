// Package projects provides GitHub Project v2 diff logic for gh-pm-kit.
package projects

import (
	"context"
	"fmt"
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
func diffProjectItems(srcHost, srcOwner string, srcProjectNumber int, srcItems, dstItems []gh.ProjectV2Item) []render.ProjectItemDiffEntry {
	var diffs []render.ProjectItemDiffEntry
	matchedDstIDs := make(map[string]bool)

	for i := range srcItems {
		si := &srcItems[i]
		marker := migratedItemMarker(srcHost, srcOwner, srcProjectNumber, si.ID)
		di := findItemByMarker(dstItems, marker)
		if di == nil {
			diffs = append(diffs, render.ProjectItemDiffEntry{
				Status:   render.ProjectDiffStatusSrcOnly,
				SrcTitle: si.Content.Title,
			})
		} else {
			matchedDstIDs[di.ID] = true
			fvDiffs := diffItemFieldValues(si, di)
			status := render.ProjectDiffStatusEqual
			if si.Content.Title != di.Content.Title || len(fvDiffs) > 0 {
				status = render.ProjectDiffStatusModified
			}
			diffs = append(diffs, render.ProjectItemDiffEntry{
				Status:     status,
				SrcTitle:   si.Content.Title,
				DstTitle:   di.Content.Title,
				FieldDiffs: fvDiffs,
			})
		}
	}

	// Collect dst-only items: those that do not carry any migration marker for this source project.
	prefix := projectMarkerPrefix(srcHost, srcOwner, srcProjectNumber)
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
func diffItemFieldValues(src, dst *gh.ProjectV2Item) []render.ProjectFieldValueDiff {
	dstFVMap := make(map[string]string, len(dst.FieldValues))
	for _, fv := range dst.FieldValues {
		dstFVMap[fv.FieldName] = projectFieldValueString(fv)
	}

	var diffs []render.ProjectFieldValueDiff
	for _, fv := range src.FieldValues {
		if !migratableDataTypes[fv.ValueType] {
			continue
		}
		srcVal := projectFieldValueString(fv)
		dstVal := dstFVMap[fv.FieldName]
		if srcVal != dstVal {
			diffs = append(diffs, render.ProjectFieldValueDiff{
				FieldName: fv.FieldName,
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
