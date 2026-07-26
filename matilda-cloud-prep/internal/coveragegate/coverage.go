package coveragegate

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

const DefaultMinimumPercent = 88.0

type Summary struct {
	Mode              string
	CoveredStatements int64
	TotalStatements   int64
	Percent           float64
}

type blockKey struct {
	fileName    string
	startLine   int
	startColumn int
	endLine     int
	endColumn   int
}

type profileBlock struct {
	key        blockKey
	statements int
	count      int
}

type BelowMinimumError struct {
	Summary        Summary
	MinimumPercent float64
}

func (e BelowMinimumError) Error() string {
	return fmt.Sprintf("coverage %.2f%% is below required %.2f%%", e.Summary.Percent, e.MinimumPercent)
}

func EvaluateFile(path string, minimumPercent float64) (Summary, error) {
	if err := validateMinimum(minimumPercent); err != nil {
		return Summary{}, err
	}

	profile, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("read coverage profile: %w", err)
	}
	defer profile.Close()

	summary, err := Summarize(profile)
	if err != nil {
		return Summary{}, err
	}
	if summary.Percent < minimumPercent {
		return summary, BelowMinimumError{
			Summary:        summary,
			MinimumPercent: minimumPercent,
		}
	}
	return summary, nil
}

func Evaluate(profile io.Reader, minimumPercent float64) (Summary, error) {
	if err := validateMinimum(minimumPercent); err != nil {
		return Summary{}, err
	}

	summary, err := Summarize(profile)
	if err != nil {
		return Summary{}, err
	}
	if summary.Percent < minimumPercent {
		return summary, BelowMinimumError{
			Summary:        summary,
			MinimumPercent: minimumPercent,
		}
	}
	return summary, nil
}

func Summarize(profile io.Reader) (Summary, error) {
	scanner := bufio.NewScanner(profile)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var mode string
	var lineNumber int
	blocks := map[blockKey]profileBlock{}

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if lineNumber == 1 {
			parsedMode, err := parseMode(line)
			if err != nil {
				return Summary{}, err
			}
			mode = parsedMode
			continue
		}

		block, err := parseBlock(line)
		if err != nil {
			return Summary{}, fmt.Errorf("coverage profile line %d: %w", lineNumber, err)
		}
		if existing, ok := blocks[block.key]; ok {
			if existing.statements != block.statements {
				return Summary{}, fmt.Errorf("coverage profile line %d: inconsistent numberOfStatements for %s", lineNumber, block.key.fileName)
			}
			if mode == "set" {
				existing.count |= block.count
			} else {
				existing.count += block.count
			}
			blocks[block.key] = existing
			continue
		}
		blocks[block.key] = block
	}
	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("read coverage profile: %w", err)
	}
	if lineNumber == 0 {
		return Summary{}, errors.New("coverage profile is empty")
	}

	var coveredStatements int64
	var totalStatements int64
	for _, block := range blocks {
		totalStatements += int64(block.statements)
		if block.count > 0 {
			coveredStatements += int64(block.statements)
		}
	}
	if totalStatements == 0 {
		return Summary{}, errors.New("coverage profile contains no statements")
	}

	return Summary{
		Mode:              mode,
		CoveredStatements: coveredStatements,
		TotalStatements:   totalStatements,
		Percent:           100 * float64(coveredStatements) / float64(totalStatements),
	}, nil
}

func parseMode(line string) (string, error) {
	const prefix = "mode: "
	if !strings.HasPrefix(line, prefix) || line == prefix {
		return "", fmt.Errorf("bad coverage profile mode line: %q", line)
	}

	mode := line[len(prefix):]
	switch mode {
	case "set", "count", "atomic":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported coverage mode %q", mode)
	}
}

func parseBlock(line string) (profileBlock, error) {
	end := len(line)

	count, end, err := seekBack(line, ' ', end, "count")
	if err != nil {
		return profileBlock{}, err
	}
	statements, end, err := seekBack(line, ' ', end, "numberOfStatements")
	if err != nil {
		return profileBlock{}, err
	}
	endColumn, end, err := seekBack(line, '.', end, "end column")
	if err != nil {
		return profileBlock{}, err
	}
	endLine, end, err := seekBack(line, ',', end, "end line")
	if err != nil {
		return profileBlock{}, err
	}
	startColumn, end, err := seekBack(line, '.', end, "start column")
	if err != nil {
		return profileBlock{}, err
	}
	startLine, end, err := seekBack(line, ':', end, "start line")
	if err != nil {
		return profileBlock{}, err
	}
	fileName := line[:end]
	if fileName == "" {
		return profileBlock{}, errors.New("filename cannot be blank")
	}

	return profileBlock{
		key: blockKey{
			fileName:    fileName,
			startLine:   startLine,
			startColumn: startColumn,
			endLine:     endLine,
			endColumn:   endColumn,
		},
		statements: statements,
		count:      count,
	}, nil
}

func seekBack(line string, separator byte, end int, field string) (value int, nextEnd int, err error) {
	for start := end - 1; start >= 0; start-- {
		if line[start] != separator {
			continue
		}

		raw := line[start+1 : end]
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("parse %s %q: %w", field, raw, err)
		}
		if value < 0 {
			return 0, 0, fmt.Errorf("negative values are not allowed for %s", field)
		}
		return value, start, nil
	}

	return 0, 0, fmt.Errorf("could not find %q before %s", string(separator), field)
}

func validateMinimum(minimumPercent float64) error {
	if math.IsNaN(minimumPercent) || math.IsInf(minimumPercent, 0) || minimumPercent < 0 || minimumPercent > 100 {
		return fmt.Errorf("invalid minimum coverage %.2f: must be between 0 and 100", minimumPercent)
	}
	return nil
}
