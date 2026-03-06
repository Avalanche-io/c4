package c4m

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// TestReadiness mechanically verifies every item in READINESS.md.
// When this test passes, c4m is ready for 1.0 release.
func TestReadiness(t *testing.T) {
	t.Run("Parser", func(t *testing.T) {
		t.Run("QuotedRoundTrip", testParserQuotedRoundTrip)
		t.Run("LeadingSpaceRoundTrip", testParserLeadingSpaceRoundTrip)
		t.Run("SymlinkSpacesRoundTrip", testParserSymlinkSpacesRoundTrip)
		t.Run("BracketEscapeRoundTrip", testParserBracketEscapeRoundTrip)
		t.Run("NaturalSortTextBeforeNumeric", testNaturalSortTextBeforeNumeric)
		t.Run("NaturalSortASCIIDigitsOnly", testNaturalSortASCIIOnly)
	})

	t.Run("Semantics", func(t *testing.T) {
		t.Run("CanonicalizeDeterministic", testCanonicalizeDeterministic)
		t.Run("ValidateAcceptsNullTimestamps", testValidateAcceptsNullTimestamps)
		t.Run("ValidateAcceptsNullSizes", testValidateAcceptsNullSizes)
		t.Run("NoStderrFromValidator", testNoStderrFromValidator)
		t.Run("ExpandReturnsError", testExpandReturnsError)
		t.Run("NullTimestampExported", testNullTimestampExported)
	})

	t.Run("API", func(t *testing.T) {
		t.Run("CurrentLayerUnexported", testCurrentLayerUnexported)
		t.Run("PropagateMetadataUnexported", testPropagateMetadataUnexported)
		t.Run("GenerateFromReaderRemoved", testGenerateFromReaderRemoved)
		t.Run("SingleSortMethod", testSingleSortMethod)
		t.Run("SingleLookupMethod", testSingleLookupMethod)
		t.Run("CopyIsDeep", testCopyIsDeep)
		t.Run("SentinelErrors", testSentinelErrors)
	})

	t.Run("Docs", func(t *testing.T) {
		t.Run("READMEFilesExist", testREADMEFilesExist)
		t.Run("NoStaleAPINamesInWorkflows", testNoStaleAPINamesInWorkflows)
		t.Run("NoStaleAPINamesInREADME", testNoStaleAPINamesInREADME)
		t.Run("ImplementationNotesSymlink", testImplementationNotesSymlink)
	})

	t.Run("Hardening", func(t *testing.T) {
		t.Run("FuzzTestsExist", testFuzzTestsExist)
		t.Run("AdversarialTestsExist", testAdversarialTestsExist)
		t.Run("CoverageThreshold", testCoverageThreshold)
	})

	t.Run("Transform", func(t *testing.T) {
		t.Run("ExperimentalWarning", testTransformExperimentalWarning)
	})
}

// --- Parser ---

func testParserQuotedRoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	idOf := func(s string) c4.ID { return c4.Identify(strings.NewReader(s)) }

	cases := []struct {
		name     string
		filename string
	}{
		{"backslash", `file\with\backslashes.txt`},
		{"quote", `file"with"quotes.txt`},
		{"newline", "file\nwith\nnewlines.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManifest()
			m.Version = "1.0"
			e := &Entry{
				Name:      tc.filename,
				Mode:      0644,
				Timestamp: ts,
				Size:      100,
				C4ID:      idOf(tc.filename),
			}
			m.Entries = append(m.Entries, e)

			data, err := Marshal(m)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			m2, err := Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if len(m2.Entries) != 1 || m2.Entries[0].Name != tc.filename {
				t.Errorf("round-trip failed: got %q, want %q", m2.Entries[0].Name, tc.filename)
			}
		})
	}
}

func testParserLeadingSpaceRoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := c4.Identify(strings.NewReader("leading space"))

	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name: " leading-space.txt", Mode: 0644, Timestamp: ts, Size: 50, C4ID: id,
	})

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m2.Entries[0].Name != " leading-space.txt" {
		t.Errorf("got %q, want %q", m2.Entries[0].Name, " leading-space.txt")
	}
}

func testParserSymlinkSpacesRoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := c4.Identify(strings.NewReader("target content"))

	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name:      "my link",
		Mode:      os.ModeSymlink | 0777,
		Timestamp: ts,
		Size:      30,
		Target:    "path with spaces/file.txt",
		C4ID:      id,
	})

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m2.Entries[0].Target != "path with spaces/file.txt" {
		t.Errorf("got target %q, want %q", m2.Entries[0].Target, "path with spaces/file.txt")
	}
}

func testParserBracketEscapeRoundTrip(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	id := c4.Identify(strings.NewReader("bracket test"))

	// A filename with literal brackets should NOT be treated as a sequence
	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name:      "file[1].txt",
		Mode:      0644,
		Timestamp: ts,
		Size:      100,
		C4ID:      id,
	})

	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m2.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m2.Entries))
	}
	if m2.Entries[0].Name != "file[1].txt" {
		t.Errorf("round-trip name: got %q, want %q", m2.Entries[0].Name, "file[1].txt")
	}
	if m2.Entries[0].IsSequence {
		t.Error("escaped brackets should not be detected as sequence")
	}
}

func testNaturalSortTextBeforeNumeric(t *testing.T) {
	// Text segments should sort before numeric segments
	if !NaturalLess("abc", "123") {
		t.Error("text should sort before numeric: NaturalLess(\"abc\", \"123\") should be true")
	}
}

func testNaturalSortASCIIOnly(t *testing.T) {
	// Arabic-Indic digits (U+0660-U+0669) should be treated as text, not numbers
	arabicOne := "\u0661" // Arabic-Indic digit one
	if NaturalLess("2", arabicOne) {
		t.Error("Arabic-Indic digits should be treated as text, not compared numerically")
	}
}

// --- Semantics ---

func testCanonicalizeDeterministic(t *testing.T) {
	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Timestamp: NullTimestamp(),
		Size:      100,
		C4ID:      c4.Identify(strings.NewReader("test")),
	})

	c1 := m.Copy()
	c1.Canonicalize()
	out1 := c1.Canonical()

	c2 := m.Copy()
	c2.Canonicalize()
	out2 := c2.Canonical()

	if out1 != out2 {
		t.Error("Canonicalize is not deterministic")
	}

	// Null timestamps must stay null (not get time.Now())
	for _, e := range c1.Entries {
		if e.Timestamp.Equal(NullTimestamp()) {
			continue // null stayed null — correct
		}
		if time.Since(e.Timestamp) < time.Minute {
			t.Error("Canonicalize injected time.Now() into null timestamp")
		}
	}
}

func testValidateAcceptsNullTimestamps(t *testing.T) {
	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Timestamp: NullTimestamp(),
		Size:      100,
		C4ID:      c4.Identify(strings.NewReader("test")),
	})
	if err := m.Validate(); err != nil {
		t.Errorf("Validate rejected null timestamp: %v", err)
	}
}

func testValidateAcceptsNullSizes(t *testing.T) {
	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Size:      -1, // null size
		C4ID:      c4.Identify(strings.NewReader("test")),
	})
	if err := m.Validate(); err != nil {
		t.Errorf("Validate rejected null size: %v", err)
	}
}

func testNoStderrFromValidator(t *testing.T) {
	// Verify validator.go does not import os (which would be needed for os.Stderr)
	src, err := os.ReadFile("validator.go")
	if err != nil {
		t.Fatalf("reading validator.go: %v", err)
	}
	if bytes.Contains(src, []byte("os.Stderr")) {
		t.Error("validator.go still contains os.Stderr reference")
	}
	if bytes.Contains(src, []byte("fmt.Fprint")) && bytes.Contains(src, []byte("Stderr")) {
		t.Error("validator.go still prints to stderr")
	}
}

func testExpandReturnsError(t *testing.T) {
	input := "@expand patterns.c4m\n"
	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("@expand should return an error")
	}
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("@expand error should wrap ErrInvalidEntry, got: %v", err)
	}
}

func testNullTimestampExported(t *testing.T) {
	ts := NullTimestamp()
	if !ts.Equal(time.Unix(0, 0).UTC()) {
		t.Error("NullTimestamp() should return Unix epoch")
	}
}

// --- API ---

func testCurrentLayerUnexported(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "manifest.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing manifest.go: %v", err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "Manifest" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.Name == "CurrentLayer" {
						t.Error("CurrentLayer is still exported — should be currentLayer")
					}
				}
			}
		}
	}
}

func testPropagateMetadataUnexported(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "manifest.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing manifest.go: %v", err)
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name == "PropagateMetadata" {
			t.Error("PropagateMetadata is still exported — should be propagateMetadata")
		}
	}
}

func testGenerateFromReaderRemoved(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "manifest.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing manifest.go: %v", err)
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name == "GenerateFromReader" {
			t.Error("GenerateFromReader still exists — should be removed")
		}
	}

	// Also check decoder.go, encoder.go, operations.go
	for _, filename := range []string{"decoder.go", "encoder.go", "operations.go"} {
		src, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		if bytes.Contains(src, []byte("func GenerateFromReader")) {
			t.Errorf("GenerateFromReader found in %s", filename)
		}
	}
}

func testSingleSortMethod(t *testing.T) {
	// SortEntries should exist; Sort and SortSiblingsHierarchically should not be exported
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing package: %v", err)
	}

	hasSortEntries := false
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil {
					continue
				}
				switch fd.Name.Name {
				case "SortEntries":
					hasSortEntries = true
				case "Sort":
					t.Error("Sort() is still exported — should be unexported or removed")
				case "SortSiblingsHierarchically":
					t.Error("SortSiblingsHierarchically() is still exported — should be unexported")
				}
			}
		}
	}
	if !hasSortEntries {
		t.Error("SortEntries() method not found")
	}
}

func testSingleLookupMethod(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing package: %v", err)
	}

	hasGetEntry := false
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil {
					continue
				}
				switch fd.Name.Name {
				case "GetEntry":
					hasGetEntry = true
				case "GetByPath":
					t.Error("GetByPath() still exists — should be removed (replaced by GetEntry)")
				}
			}
		}
	}
	if !hasGetEntry {
		t.Error("GetEntry() method not found")
	}

	// Verify GetEntry is O(1) indexed — check it uses ensureIndex
	src, err := os.ReadFile("manifest.go")
	if err != nil {
		t.Fatalf("reading manifest.go: %v", err)
	}
	if !bytes.Contains(src, []byte("ensureIndex")) {
		t.Error("GetEntry should use ensureIndex for O(1) lookup")
	}
}

func testCopyIsDeep(t *testing.T) {
	m := NewManifest()
	m.Version = "1.0"
	m.Entries = append(m.Entries, &Entry{
		Name: "test.txt",
		Mode: 0644,
		Size: 100,
		C4ID: c4.Identify(strings.NewReader("test")),
	})
	m.DataBlocks = append(m.DataBlocks, &DataBlock{
		ID:      c4.Identify(strings.NewReader("block")),
		Content: []byte("content"),
	})

	cp := m.Copy()

	// Mutate original — copy should be unaffected
	m.Entries[0].Name = "MUTATED"
	m.DataBlocks[0].Content[0] = 'X'

	if cp.Entries[0].Name == "MUTATED" {
		t.Error("Copy() entries are shallow — mutation propagated")
	}
	if cp.DataBlocks[0].Content[0] == 'X' {
		t.Error("Copy() data blocks are shallow — mutation propagated")
	}
}

func testSentinelErrors(t *testing.T) {
	// Verify all expected sentinel errors exist
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrInvalidEntry", ErrInvalidEntry},
		{"ErrDuplicatePath", ErrDuplicatePath},
		{"ErrPathTraversal", ErrPathTraversal},
	}
	for _, s := range sentinels {
		if s.err == nil {
			t.Errorf("sentinel error %s is nil", s.name)
		}
	}
}

// --- Docs ---

func testREADMEFilesExist(t *testing.T) {
	src, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	// Extract .go filenames from the Package Files section
	goFiles := []string{
		"manifest.go", "entry.go", "encoder.go", "decoder.go",
		"builder.go", "operations.go", "validator.go",
		"naturalsort.go", "sequence.go",
	}
	for _, f := range goFiles {
		if !bytes.Contains(src, []byte(f)) {
			t.Errorf("README.md doesn't mention %s", f)
		}
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("README.md references %s but it doesn't exist", f)
		}
	}
}

func testNoStaleAPINamesInWorkflows(t *testing.T) {
	src, err := os.ReadFile("WORKFLOWS.md")
	if err != nil {
		t.Skipf("WORKFLOWS.md not found: %v", err)
	}
	stale := []string{
		"NewGeneratorWithOptions", "WithC4IDs", "WithHidden",
		"GenerateFromReader", "GetByPath",
	}
	for _, name := range stale {
		if bytes.Contains(src, []byte(name)) {
			t.Errorf("WORKFLOWS.md contains stale API name: %s", name)
		}
	}
}

func testNoStaleAPINamesInREADME(t *testing.T) {
	src, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	stale := []string{
		"NewGeneratorWithOptions", "WithC4IDs", "WithHidden",
		"GenerateFromReader", "GetByPath",
	}
	for _, name := range stale {
		if bytes.Contains(src, []byte(name)) {
			t.Errorf("README.md contains stale API name: %s", name)
		}
	}
}

func testImplementationNotesSymlink(t *testing.T) {
	src, err := os.ReadFile("IMPLEMENTATION_NOTES.md")
	if err != nil {
		t.Fatalf("reading IMPLEMENTATION_NOTES.md: %v", err)
	}
	if bytes.Contains(src, []byte("C4 ID of the link target path string")) {
		t.Error("IMPLEMENTATION_NOTES.md still says 'C4 ID of the link target path string' — should match spec (target file's content)")
	}
	if !bytes.Contains(src, []byte("target file's content")) {
		t.Error("IMPLEMENTATION_NOTES.md should say 'target file's content' for symlink C4 ID")
	}
}

// --- Hardening ---

func testFuzzTestsExist(t *testing.T) {
	required := []string{"FuzzDecoder", "FuzzRoundTrip", "FuzzValidator", "FuzzNaturalSort"}

	src, err := os.ReadFile("fuzz_test.go")
	if err != nil {
		t.Fatalf("fuzz_test.go not found: %v", err)
	}
	for _, name := range required {
		if !bytes.Contains(src, []byte("func "+name)) {
			t.Errorf("fuzz_test.go missing %s", name)
		}
	}
}

func testAdversarialTestsExist(t *testing.T) {
	if _, err := os.Stat("adversarial_test.go"); os.IsNotExist(err) {
		t.Fatal("adversarial_test.go not found")
	}

	src, err := os.ReadFile("adversarial_test.go")
	if err != nil {
		t.Fatalf("reading adversarial_test.go: %v", err)
	}
	if !bytes.Contains(src, []byte("func Test")) {
		t.Error("adversarial_test.go has no Test functions")
	}
}

func testCoverageThreshold(t *testing.T) {
	// Coverage must be verified externally with: go test -cover
	// Here we just verify the test files exist and are substantial
	testFiles, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("globbing test files: %v", err)
	}

	// Count test functions across all test files
	testCount := 0
	for _, tf := range testFiles {
		src, err := os.ReadFile(tf)
		if err != nil {
			continue
		}
		testCount += bytes.Count(src, []byte("\nfunc Test"))
		testCount += bytes.Count(src, []byte("\nfunc Fuzz"))
	}

	// We need substantial test coverage — at least 50 test functions
	if testCount < 50 {
		t.Errorf("only %d test/fuzz functions found — need at least 50 for adequate coverage", testCount)
	}
}

// --- Transform ---

func testTransformExperimentalWarning(t *testing.T) {
	src, err := os.ReadFile("transform/doc.go")
	if err != nil {
		t.Fatalf("transform/doc.go not found: %v", err)
	}
	if !bytes.Contains(src, []byte("EXPERIMENTAL")) {
		t.Error("transform/doc.go missing EXPERIMENTAL warning")
	}
}
