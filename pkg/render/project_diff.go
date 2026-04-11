// Package render provides pm-kit-specific rendering helpers.
package render

import (
	"fmt"

	"github.com/fatih/color"
	ghrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

// ProjectDiffStatus represents the diff status of a project element.
type ProjectDiffStatus string

const (
	ProjectDiffStatusSrcOnly  ProjectDiffStatus = "src-only"
	ProjectDiffStatusDstOnly  ProjectDiffStatus = "dst-only"
	ProjectDiffStatusModified ProjectDiffStatus = "modified"
	ProjectDiffStatusEqual    ProjectDiffStatus = "equal"
)

// ProjectFieldValueDiff represents a field value difference for a single field within an item.
type ProjectFieldValueDiff struct {
	FieldName string `json:"fieldName"`
	SrcValue  string `json:"srcValue"`
	DstValue  string `json:"dstValue"`
}

// ProjectItemDiffEntry represents the diff of a single project item.
type ProjectItemDiffEntry struct {
	Status     ProjectDiffStatus       `json:"status"`
	SrcTitle   string                  `json:"srcTitle,omitempty"`
	DstTitle   string                  `json:"dstTitle,omitempty"`
	FieldDiffs []ProjectFieldValueDiff `json:"fieldDiffs,omitempty"`
}

// ProjectFieldDiffEntry represents the diff of a single custom field definition.
type ProjectFieldDiffEntry struct {
	Status   ProjectDiffStatus `json:"status"`
	Name     string            `json:"name"`
	DataType string            `json:"dataType"`
}

// ProjectDiffReport is the top-level report for rendering a project diff.
type ProjectDiffReport struct {
	SrcLabel string                  `json:"srcLabel"`
	DstLabel string                  `json:"dstLabel"`
	Fields   []ProjectFieldDiffEntry `json:"fields"`
	Items    []ProjectItemDiffEntry  `json:"items"`
}

// HasDiff reports whether any differences exist in the report.
func (r *ProjectDiffReport) HasDiff() bool {
	for _, f := range r.Fields {
		if f.Status != ProjectDiffStatusEqual {
			return true
		}
	}
	for _, i := range r.Items {
		if i.Status != ProjectDiffStatusEqual {
			return true
		}
	}
	return false
}

// RenderProjectDiff renders a ProjectDiffReport using the provided Renderer.
func RenderProjectDiff(r *ghrender.Renderer, report *ProjectDiffReport) error {
	if r.HasExporter() {
		return r.RenderExportedData(report)
	}

	if !report.HasDiff() {
		r.WriteLine("No differences.")
		return nil
	}

	minus := "---"
	plus := "+++"
	if r.Color {
		minus = color.RedString("---")
		plus = color.GreenString("+++")
	}
	r.WriteLine(fmt.Sprintf("%s %s", minus, report.SrcLabel))
	r.WriteLine(fmt.Sprintf("%s %s", plus, report.DstLabel))

	// Fields section
	var fieldDiffs []ProjectFieldDiffEntry
	for _, f := range report.Fields {
		if f.Status != ProjectDiffStatusEqual {
			fieldDiffs = append(fieldDiffs, f)
		}
	}
	if len(fieldDiffs) > 0 {
		r.WriteLine("")
		r.WriteLine("@@ Fields @@")
		for _, f := range fieldDiffs {
			r.WriteLine(renderProjectFieldDiff(f, r.Color))
		}
	}

	// Items section
	var itemDiffs []ProjectItemDiffEntry
	for _, i := range report.Items {
		if i.Status != ProjectDiffStatusEqual {
			itemDiffs = append(itemDiffs, i)
		}
	}
	if len(itemDiffs) > 0 {
		r.WriteLine("")
		r.WriteLine("@@ Items @@")
		for _, i := range itemDiffs {
			if line := renderProjectItemDiff(i, r.Color); line != "" {
				r.WriteLine(line)
			}
		}
	}

	return nil
}

func renderProjectFieldDiff(f ProjectFieldDiffEntry, colorEnabled bool) string {
	switch f.Status {
	case ProjectDiffStatusSrcOnly:
		line := fmt.Sprintf("- %s (%s)", f.Name, f.DataType)
		if colorEnabled {
			return color.RedString(line)
		}
		return line
	case ProjectDiffStatusDstOnly:
		line := fmt.Sprintf("+ %s (%s)", f.Name, f.DataType)
		if colorEnabled {
			return color.GreenString(line)
		}
		return line
	case ProjectDiffStatusModified:
		return fmt.Sprintf("~ %s (%s)", f.Name, f.DataType)
	}
	return fmt.Sprintf("  %s (%s)", f.Name, f.DataType)
}

func renderProjectItemDiff(item ProjectItemDiffEntry, colorEnabled bool) string {
	title := item.SrcTitle
	if title == "" {
		title = item.DstTitle
	}

	var result string
	switch item.Status {
	case ProjectDiffStatusSrcOnly:
		line := fmt.Sprintf("- %s", title)
		if colorEnabled {
			result = color.RedString(line)
		} else {
			result = line
		}
	case ProjectDiffStatusDstOnly:
		line := fmt.Sprintf("+ %s", title)
		if colorEnabled {
			result = color.GreenString(line)
		} else {
			result = line
		}
	case ProjectDiffStatusModified:
		result = fmt.Sprintf("~ %s", title)
		if item.SrcTitle != item.DstTitle && item.SrcTitle != "" && item.DstTitle != "" {
			result += fmt.Sprintf(" (title: %q -> %q)", item.SrcTitle, item.DstTitle)
		}
		for _, fv := range item.FieldDiffs {
			srcLine := fmt.Sprintf("  - %s: %s", fv.FieldName, fv.SrcValue)
			dstLine := fmt.Sprintf("  + %s: %s", fv.FieldName, fv.DstValue)
			if colorEnabled {
				result += "\n" + color.RedString(srcLine)
				result += "\n" + color.GreenString(dstLine)
			} else {
				result += "\n" + srcLine
				result += "\n" + dstLine
			}
		}
	}
	return result
}
