package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/i18n"
)

// WriteMarkdown writes a forum-pasteable markdown summary of a benchmark run:
// markdown headings outside, textgrid-aligned tables inside fenced code
// blocks so the layout survives any markdown renderer.
func WriteMarkdown(w io.Writer, doc Document) error {
	if w == nil {
		w = io.Discard
	}
	if _, err := fmt.Fprintf(w, "# %s\n\n", i18n.Tf("report.markdown.title", map[string]any{"Version": doc.Version})); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "> %s\n\n", markdownMetaLine(doc)); err != nil {
		return err
	}
	return WriteMarkdownBody(w, doc)
}

// WriteMarkdownBody writes the markdown sections without the document title
// and provenance line, for embedding inside larger documents such as the
// checkup hardware section.
func WriteMarkdownBody(w io.Writer, doc Document) error {
	if _, err := fmt.Fprintf(w, "## %s\n\n", i18n.T("report.markdown.system")); err != nil {
		return err
	}
	if err := writeFenced(w, func(w io.Writer) error {
		return writeSystemLines(w, doc)
	}); err != nil {
		return err
	}
	for _, table := range []struct {
		title   string
		entries []WorkloadEntry
	}{
		{i18n.T("report.console.measured"), doc.Results.Workloads},
		{i18n.T("report.console.extensions"), doc.Extensions.Workloads},
	} {
		if len(table.entries) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "\n## %s\n\n", table.title); err != nil {
			return err
		}
		if err := writeFenced(w, func(w io.Writer) error {
			_, err := fmt.Fprint(w, renderWorkloadGrid(table.entries))
			return err
		}); err != nil {
			return err
		}
	}
	if len(doc.Warnings) > 0 {
		if _, err := fmt.Fprintf(w, "\n## %s\n\n", i18n.T("report.console.warnings")); err != nil {
			return err
		}
		for _, warning := range doc.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", strings.TrimSpace(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

// markdownMetaLine renders the provenance quote: binary version, UTC
// timestamp, and the resolved node catalog when the run recorded one.
func markdownMetaLine(doc Document) string {
	parts := []string{
		"vmbench " + doc.Version,
		doc.Timestamp.UTC().Format(time.RFC3339),
	}
	if source := strings.TrimSpace(doc.Config.CatalogSource); source != "" {
		catalog := source
		if revision := strings.TrimSpace(doc.Config.CatalogRevision); revision != "" {
			catalog += "@" + revision
		}
		parts = append(parts, "catalog "+catalog)
	}
	return strings.Join(parts, " · ")
}

// writeFenced wraps body output in a fenced code block.
func writeFenced(w io.Writer, body func(io.Writer) error) error {
	if _, err := fmt.Fprintln(w, "```"); err != nil {
		return err
	}
	if err := body(w); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "```")
	return err
}
