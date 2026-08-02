package sourcehandlegate

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Phirlly/matilda/matilda-cloud-prep/internal/workflow"
)

const (
	ExitOK = iota
	ExitGateFailed
	ExitUsage
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("sourcehandlegate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	root := "."
	scans := scanFlags{}
	flags.StringVar(&root, "root", ".", "project root")
	flags.Var(&scans, "scan", "production Go file or directory to scan; repeatable")

	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "invalid source handle gate arguments: %v\n", err)
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected source handle gate argument %q\n", flags.Arg(0))
		return ExitUsage
	}
	if len(scans) == 0 {
		scans = scanFlags{"cmd", "internal"}
	}

	handles, err := discoverSourceHandles(root, scans)
	if err != nil {
		fmt.Fprintf(stderr, "source handle gate failed: %v\n", err)
		return ExitGateFailed
	}
	if len(handles) == 0 {
		fmt.Fprintln(stderr, "source handle gate failed: no cached source handles found in production Go files")
		return ExitGateFailed
	}
	if err := workflow.ValidateCachedSourceHandleFiles(root, handles); err != nil {
		fmt.Fprintf(stderr, "source handle gate failed: %v\n", err)
		return ExitGateFailed
	}

	fmt.Fprintf(stdout, "source handle gate passed: %d cached source handles verified\n", len(handles))
	return ExitOK
}

type scanFlags []string

func (flags *scanFlags) String() string {
	return strings.Join(*flags, ",")
}

func (flags *scanFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("scan path must not be empty")
	}
	*flags = append(*flags, value)
	return nil
}

func discoverSourceHandles(root string, scans []string) ([]workflow.SourceHandle, error) {
	seen := map[string]struct{}{}
	for _, scan := range scans {
		path := filepath.Join(root, filepath.FromSlash(scan))
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("scan path %q cannot be accessed", scan)
		}
		if info.IsDir() {
			if err := filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					if shouldSkipDir(entry.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				if productionGoFile(path) {
					return collectSourceHandlesFromFile(path, seen)
				}
				return nil
			}); err != nil {
				return nil, fmt.Errorf("scan path %q failed: %w", scan, err)
			}
			continue
		}
		if productionGoFile(path) {
			if err := collectSourceHandlesFromFile(path, seen); err != nil {
				return nil, err
			}
		}
	}

	uris := make([]string, 0, len(seen))
	for uri := range seen {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	handles := make([]workflow.SourceHandle, 0, len(uris))
	for _, uri := range uris {
		handles = append(handles, workflow.SourceHandle{Label: "Cached source reference", URI: uri})
	}
	return handles, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "vendor":
		return true
	default:
		return false
	}
}

func productionGoFile(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func collectSourceHandlesFromFile(path string, seen map[string]struct{}) error {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}

	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if cachedReferenceURI(value) {
			seen[value] = struct{}{}
		}
		return true
	})
	return nil
}

func cachedReferenceURI(value string) bool {
	return strings.HasPrefix(value, "docs/references/") &&
		strings.HasSuffix(value, ".md") &&
		value != "docs/references/README.md" &&
		!strings.ContainsAny(value, "\n\r\t ")
}
