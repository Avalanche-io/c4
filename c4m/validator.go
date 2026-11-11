package c4m

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Line    int
	Column  int
	Field   string
	Message string
	Fatal   bool // If true, validation cannot continue
}

func (e ValidationError) Error() string {
	if e.Line > 0 {
		if e.Column > 0 {
			return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Column, e.Message)
		}
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// ValidationStats tracks statistics about the validated content
type ValidationStats struct {
	TotalEntries int64
	Files        int64
	Directories  int64
	Symlinks     int64
	SpecialFiles int64 // block/char devices, pipes, sockets
	TotalSize    int64
	OldestTime   time.Time
	NewestTime   time.Time
	NullTimes    int64 // Entries with null timestamps
	NullSizes    int64 // Entries with null sizes
	Chunks       int64 // Entries carrying a C4 ID
	MaxDepth     int   // Maximum directory depth
}

// Validator checks a c4m file. It reads through the Decoder, the one reader
// every verb and every shared vector is held to, so a line the decoder
// refuses is reported as one fatal error at the decoder's line, and a line
// it reads is never parsed again here (design/validator-reads-through-
// decoder.md). On the entries as written, the Validator adds what the
// decoder does not check:
//
//   - a name that is a path or a path component ("./", "../", "/"; the
//     spec says parsers MUST reject them, and the decoder does not);
//   - a path listed twice in one section (an error; the spec says parsers
//     MUST reject duplicates, and the decoder keeps only the last);
//   - an entry deeper than its parent directory allows (an error);
//   - with Strict, siblings out of canonical order, files before
//     directories then natural sort (the spec lets a parser accept
//     unsorted input, so only Strict reports it).
type Validator struct {
	Strict    bool // Also require canonical order
	MaxErrors int  // Stop after this many errors (0 = unlimited)

	errors      []ValidationError
	warnings    []ValidationError
	stats       ValidationStats
	currentPath string
}

// NewValidator creates a new validator
func NewValidator(strict bool) *Validator {
	return &Validator{Strict: strict, MaxErrors: 100000}
}

// ValidateManifest validates a c4m file read from r: a plain listing or a
// patch chain, in canonical or pretty form.
func (v *Validator) ValidateManifest(r io.Reader) error {
	v.errors, v.warnings = nil, nil
	v.stats = ValidationStats{}
	v.currentPath = ""

	var (
		section = -1
		dirs    []string          // open directory names by depth
		lineOf  map[string]int    // path -> line, this section
		lastKid map[string]*Entry // parent path -> previous sibling, this section
		stopped bool
	)
	report := func(line int, field, msg string) {
		if stopped {
			return
		}
		if v.MaxErrors > 0 && len(v.errors) >= v.MaxErrors {
			v.addError(line, 0, "", fmt.Sprintf("stopping after %d errors", v.MaxErrors), true)
			stopped = true
			return
		}
		v.addError(line, 0, field, msg, false)
	}

	d := NewDecoder(r)
	d.quiet = true
	d.onEntry = func(line, sec int, e *Entry) {
		if sec != section {
			section = sec
			dirs = dirs[:0]
			lineOf = make(map[string]int)
			lastKid = make(map[string]*Entry)
		}
		if e.Depth > len(dirs) {
			report(line, "depth", fmt.Sprintf("entry %q at depth %d has no parent directory at depth %d", e.Name, e.Depth, e.Depth-1))
			return
		}
		if isPathName(e.Name) {
			// The spec: parsers MUST reject path traversal ("../", "./",
			// names containing "/"). Manifest.Validate's check, one rule.
			report(line, "name", fmt.Sprintf("name %q is a path or a path component, not an entry name", e.Name))
			return
		}
		dirs = dirs[:e.Depth]
		parent := strings.Join(dirs, "")
		path := parent + e.Name
		v.currentPath = path
		v.count(e)

		if first, ok := lineOf[path]; ok {
			report(line, "duplicate", fmt.Sprintf("duplicate path %q (first at line %d)", path, first))
		} else {
			lineOf[path] = line
			if prev := lastKid[parent]; v.Strict && prev != nil && !siblingLess(prev, e) {
				report(line, "ordering", fmt.Sprintf("entries not in canonical order: %q should come before %q (files before directories, then natural order)", path, parent+prev.Name))
			}
			kid := *e
			lastKid[parent] = &kid
		}
		if e.FlowTarget != "" {
			if msg := flowTargetFault(e.FlowTarget); msg != "" {
				report(line, "flow", msg)
			}
		}
		if e.IsDir() {
			dirs = append(dirs, e.Name)
		}
	}
	if _, err := d.Decode(); err != nil {
		v.errors = append(v.errors, ValidationError{Line: d.lineNum, Field: "decode", Message: err.Error(), Fatal: true})
	}
	return v.getResult()
}

// flowLocationPattern is the spec's location name: [a-zA-Z][a-zA-Z0-9_-]*.
var flowLocationPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// flowTargetFault checks a decoded flow target, <location>:<path>. The
// decoder splits the target off its line but does not check it, so this is
// a check of the decoded value, not a second parse. It returns "" when the
// target is sound.
func flowTargetFault(target string) string {
	i := strings.Index(target, ":")
	if i < 0 {
		return fmt.Sprintf("flow target missing ':' separator: %q", target)
	}
	if !flowLocationPattern.MatchString(target[:i]) {
		return fmt.Sprintf("invalid flow location label: %q", target[:i])
	}
	path := target[i+1:]
	if strings.HasPrefix(path, "/") {
		return fmt.Sprintf("flow target path must not start with '/': %q", target)
	}
	if strings.Contains(path, "..") {
		return fmt.Sprintf("flow target path must not contain '..': %q", target)
	}
	return ""
}

// count adds a decoded entry to the statistics.
func (v *Validator) count(e *Entry) {
	s := &v.stats
	s.TotalEntries++
	if e.Depth > s.MaxDepth {
		s.MaxDepth = e.Depth
	}
	switch {
	case e.IsDir():
		s.Directories++
	case e.Mode&os.ModeSymlink != 0:
		s.Symlinks++
	case e.Mode&(os.ModeDevice|os.ModeCharDevice|os.ModeNamedPipe|os.ModeSocket) != 0:
		s.SpecialFiles++
	default:
		s.Files++
	}
	if e.Size < 0 {
		s.NullSizes++
	} else {
		s.TotalSize += e.Size
	}
	if e.Timestamp.Equal(NullTimestamp()) {
		s.NullTimes++
	} else {
		if s.OldestTime.IsZero() || e.Timestamp.Before(s.OldestTime) {
			s.OldestTime = e.Timestamp
		}
		if s.NewestTime.IsZero() || e.Timestamp.After(s.NewestTime) {
			s.NewestTime = e.Timestamp
		}
	}
	if !e.C4ID.IsNil() {
		s.Chunks++
	}
}

func (v *Validator) addError(line, col int, field, msg string, fatal bool) {
	v.errors = append(v.errors, ValidationError{Line: line, Column: col, Field: field, Message: msg, Fatal: fatal})
}

func (v *Validator) getResult() error {
	if len(v.errors) == 0 {
		return nil
	}
	var msgs []string
	for _, e := range v.errors {
		msgs = append(msgs, e.Error())
	}
	summary := fmt.Sprintf("validation failed with %d errors", len(v.errors))
	if len(v.warnings) > 0 {
		summary += fmt.Sprintf(" and %d warnings", len(v.warnings))
	}
	return fmt.Errorf("%s:\n%s", summary, strings.Join(msgs, "\n"))
}

// GetErrors returns all validation errors
func (v *Validator) GetErrors() []ValidationError {
	return v.errors
}

// GetWarnings returns all validation warnings
func (v *Validator) GetWarnings() []ValidationError {
	return v.warnings
}

// GetStats returns validation statistics
func (v *Validator) GetStats() ValidationStats {
	return v.stats
}

// GetCurrentPath returns the full path of the last entry read
func (v *Validator) GetCurrentPath() string {
	return v.currentPath
}

// ValidateFile validates a c4m file
func ValidateFile(path string, strict bool) error {
	validator := NewValidator(strict)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return validator.ValidateManifest(file)
}
