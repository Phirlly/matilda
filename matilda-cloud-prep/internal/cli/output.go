package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/audit"
	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

func writeJSON(stdout io.Writer, result workflow.Result) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, audit.RedactString(string(encoded)))
	return err
}

func writeError(stderr io.Writer, message string) {
	fmt.Fprintln(stderr, audit.RedactString(message))
}
