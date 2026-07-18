package report

import (
	"encoding/json"
	"io"
)

// WriteJSON writes a formatted JSON report to w.
func WriteJSON(w io.Writer, doc Document) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(doc)
}
