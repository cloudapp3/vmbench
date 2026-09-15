package share

import (
	"fmt"
	"io"

	"github.com/cloudapp3/vmbench/i18n"
)

// appendFooter writes the share provenance footer at the end of a text
// payload: who generated it and what redaction was (or was not) applied.
// The summary is derived from the actual inventory, so the footer can never
// claim a redaction that did not happen.
func appendFooter(w io.Writer, inventory Inventory) error {
	if _, err := fmt.Fprintf(w, "\n---\n%s · %s\n", i18n.T("cli.share.generatedBy"), inventory.Summary()); err != nil {
		return err
	}
	return nil
}
