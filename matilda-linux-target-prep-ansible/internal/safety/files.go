package safety

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	fmt.Fprintf(out, "\n%s already exists. Choose an action:\n", dest)
	fmt.Fprintln(out, "  1) Keep existing file")
	fmt.Fprintln(out, "  2) Back up existing file and create a new one")
	fmt.Fprintln(out, "  3) Overwrite existing file without backup")
	fmt.Fprint(out, "Select [1]: ")
	answer, _ := reader.ReadString('\n')
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
		fmt.Fprintf(out, "Backed up %s to %s\n", dest, backup)
		return nil
	case "3":
		fmt.Fprintf(out, "Overwriting %s without backup.\n", dest)
		return nil
	default:
		fmt.Fprintln(out, "Invalid choice. Keeping existing file.")
		return ErrSkip
	}
}
