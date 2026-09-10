package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/score"
)

// runScore implements `vmbench score <report.json|->`: it evaluates a report
// deterministically against the baseline set and renders the assessment.
func runScore(args []string) int {
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	asJSON := fs.Bool("json", false, "print the assessment as JSON")
	outPath := fs.String("out", "", "write the assessment JSON to a file")
	baselinePath := fs.String("baseline", "", "use a baseline JSON file instead of the embedded set")
	requireRevision := fs.String("baseline-rev", "", "fail unless the baseline revision matches")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench score <report.json|-> [flags]\n\n"+i18n.T("cli.usage.scoreDetail"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.scoreNeedsReport"))
		return 2
	}

	data, err := readScoreInput(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.readingReport", map[string]any{"Path": fs.Arg(0), "Err": err.Error()}))
		return 1
	}

	opts := score.Options{
		RequireRevision: *requireRevision,
		Generator:       score.DefaultGenerator,
	}
	if *baselinePath != "" {
		baseline, err := score.LoadBaseline(*baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		opts.Baseline = baseline
		opts.BaselineSource = score.SourcePath
	}
	assessment, err := score.Evaluate(data, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if *outPath != "" {
		if err := writeFile(*outPath, func(w io.Writer) error {
			return writeAssessmentJSON(w, assessment)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	if *asJSON {
		if err := writeAssessmentJSON(os.Stdout, assessment); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0
	}
	renderAssessment(os.Stdout, assessment)
	return 0
}

func readScoreInput(path string) ([]byte, error) {
	if path != "-" {
		return os.ReadFile(path)
	}
	return io.ReadAll(os.Stdin)
}

func writeAssessmentJSON(w io.Writer, assessment score.Assessment) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(assessment)
}

// renderAssessment prints the human-readable console view. Labels are
// localized; metric ids, workload names, and state values stay English.
func renderAssessment(w io.Writer, assessment score.Assessment) {
	fmt.Fprintf(w, "%s: %s (%s)\n", i18n.T("cli.score.baseline"), assessment.Baseline.Revision, assessment.Baseline.Source)
	if assessment.Composite != nil {
		composite := assessment.Composite
		line := fmt.Sprintf("%s: %.1f (%s) [%s]", i18n.T("cli.score.composite"), composite.Index, composite.Rating, composite.Status)
		if len(composite.Basis) > 0 {
			line += "  " + i18n.T("cli.score.basis") + ": " + strings.Join(composite.Basis, ", ")
		}
		if len(composite.Excluded) > 0 {
			line += "  " + i18n.T("cli.score.excluded") + ": " + strings.Join(composite.Excluded, ", ")
		}
		fmt.Fprintln(w, line)
	} else {
		fmt.Fprintf(w, "%s: %s\n", i18n.T("cli.score.composite"), i18n.T("cli.score.compositeWithheld"))
	}
	fmt.Fprintf(w, "%s: %.1f%%  %s: %.2f\n\n", i18n.T("cli.score.coverage"), assessment.Coverage.MetricPct, i18n.T("cli.score.confidence"), assessment.Coverage.Confidence)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
		i18n.T("cli.score.dimension"), i18n.T("cli.score.index"), i18n.T("cli.score.rating"),
		i18n.T("cli.score.coverage"), i18n.T("cli.score.confidence"))
	for _, dimension := range assessment.Dimensions {
		if dimension.Status != score.DimensionScored {
			fmt.Fprintf(tw, "%s\t-\t-\t-\t%s\n", dimension.ID, dimension.Status)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			dimension.ID,
			formatIndex(dimension.Index),
			dimension.Rating,
			strconv.FormatFloat(dimension.CoveragePct, 'f', 0, 64)+"%",
			strconv.FormatFloat(dimension.Confidence, 'f', 2, 64))
	}
	fmt.Fprintf(tw, "\n%s\t%s\t%s\t%s\t%s\t%s\n",
		i18n.T("cli.score.dimension"), i18n.T("cli.score.metric"), i18n.T("cli.score.value"),
		i18n.T("cli.score.normalized"), i18n.T("cli.score.status"), i18n.T("cli.score.note"))
	for _, dimension := range assessment.Dimensions {
		for _, metric := range dimension.Metrics {
			value := "-"
			if metric.Status == score.MetricOK {
				value = formatScoreValue(metric.Value) + " " + metric.Unit
			}
			normalized := "-"
			if metric.Status == score.MetricOK {
				normalized = formatIndex(metric.Normalized)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				dimension.ID, metric.ID, value, normalized, metric.Status, metric.Note)
		}
		if len(dimension.Missing) > 0 {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				dimension.ID, i18n.T("cli.score.missing"), strings.Join(dimension.Missing, ", "), "", "", "")
		}
	}
	fmt.Fprintf(tw, "\n%s\t%s\t%s\t%s\t%s\n",
		i18n.T("cli.score.profile"), i18n.T("cli.score.index"), i18n.T("cli.score.rating"),
		i18n.T("cli.score.fit"), i18n.T("cli.score.veto"))
	for _, profile := range assessment.Profiles {
		index := "-"
		rating := "-"
		if profile.Index > 0 || profile.Rating != "" {
			index = formatIndex(profile.Index)
			rating = profile.Rating
		}
		veto := ""
		if profile.Veto != nil {
			veto = fmt.Sprintf("%s %s %s (cap %s)", profile.Veto.Metric, profile.Veto.Op, formatScoreValue(profile.Veto.Value), profile.Veto.Cap)
		}
		fit := profile.Fit
		if len(profile.Missing) > 0 {
			fit += " [" + i18n.T("cli.score.missing") + ": " + strings.Join(profile.Missing, ", ") + "]"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", profile.ID, index, rating, fit, veto)
	}
	tw.Flush()

	if len(assessment.Warnings) > 0 {
		fmt.Fprintf(w, "\n%s:\n", i18n.T("cli.score.warnings"))
		for _, warning := range assessment.Warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
}

func formatIndex(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}

// formatScoreValue trims float noise from metric readings. JSON keeps full
// precision; this is only the console view.
func formatScoreValue(value float64) string {
	return strconv.FormatFloat(value, 'g', 6, 64)
}
