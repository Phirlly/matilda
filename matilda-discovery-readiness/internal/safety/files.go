package safety

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"matilda-discovery-readiness/internal/ui"
)

var ErrSkip = errors.New("skip existing file")

func PrepareDestination(in io.Reader, out io.Writer, dest string) error {
	if _, err := os.Stat(dest); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	reader := bufio.NewReader(in)
	renderer := ui.New(out)
	renderer.Header("File Exists", dest)
	renderer.Section("Choose Action")
	renderer.KeyValues([]ui.KV{
		{Key: "1", Value: "keep existing file"},
		{Key: "2", Value: "back up existing file and create a new one"},
		{Key: "3", Value: "overwrite existing file without backup"},
	})
	answer := renderer.Prompt(reader, "Select", "1")
	switch strings.TrimSpace(answer) {
	case "", "1":
		return ErrSkip
	case "2":
		backup := fmt.Sprintf("%s.backup-%s", dest, time.Now().Format("20060102-150405"))
		content, err := os.ReadFile(dest)
		if err != nil {
			return err
		}
		if err := os.WriteFile(backup, content, 0600); err != nil {
			return err
		}
		renderer.Done(fmt.Sprintf("Backed up %s to %s.", dest, backup))
		return nil
	case "3":
		renderer.Warning(fmt.Sprintf("Overwriting %s without backup.", dest))
		return nil
	default:
		renderer.Warning("Invalid choice. Keeping existing file.")
		return ErrSkip
	}
}
