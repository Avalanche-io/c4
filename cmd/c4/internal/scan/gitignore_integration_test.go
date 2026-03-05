package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratorRespectsGitignore(t *testing.T) {
	tmpdir := t.TempDir()

	// Create .gitignore excluding *.log files
	os.WriteFile(filepath.Join(tmpdir, ".gitignore"), []byte("*.log\n"), 0644)

	// Create files
	os.WriteFile(filepath.Join(tmpdir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(tmpdir, "error.log"), []byte("error"), 0644)
	os.WriteFile(filepath.Join(tmpdir, "debug.log"), []byte("debug"), 0644)

	g := NewGeneratorWithOptions(WithC4IDs(false))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range manifest.Entries {
		names = append(names, e.Name)
	}

	if !containsName(names, "keep.txt") {
		t.Error("keep.txt should be present")
	}
	if containsName(names, "error.log") {
		t.Error("error.log should be excluded by .gitignore")
	}
	if containsName(names, "debug.log") {
		t.Error("debug.log should be excluded by .gitignore")
	}
}

func TestGeneratorNestedGitignore(t *testing.T) {
	tmpdir := t.TempDir()

	// Root .gitignore excludes *.log
	os.WriteFile(filepath.Join(tmpdir, ".gitignore"), []byte("*.log\n"), 0644)

	// Create subdir with its own .gitignore that negates debug.log
	subdir := filepath.Join(tmpdir, "src")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, ".gitignore"), []byte("!debug.log\n"), 0644)

	// Create files
	os.WriteFile(filepath.Join(tmpdir, "root.log"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(subdir, "error.log"), []byte("error"), 0644)
	os.WriteFile(filepath.Join(subdir, "debug.log"), []byte("debug"), 0644)
	os.WriteFile(filepath.Join(subdir, "main.go"), []byte("main"), 0644)

	g := NewGeneratorWithOptions(WithC4IDs(false))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range manifest.Entries {
		names = append(names, e.Name)
	}

	if containsName(names, "root.log") {
		t.Error("root.log should be excluded by root .gitignore")
	}
	if containsName(names, "error.log") {
		t.Error("src/error.log should be excluded by root *.log pattern")
	}
	if !containsName(names, "debug.log") {
		t.Error("src/debug.log should be present (negated by src/.gitignore)")
	}
	if !containsName(names, "main.go") {
		t.Error("src/main.go should be present")
	}
}

func TestGeneratorNoGitignore(t *testing.T) {
	tmpdir := t.TempDir()

	// Create .gitignore excluding *.log files
	os.WriteFile(filepath.Join(tmpdir, ".gitignore"), []byte("*.log\n"), 0644)

	os.WriteFile(filepath.Join(tmpdir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(tmpdir, "error.log"), []byte("error"), 0644)

	// With gitignore disabled, log files should appear
	g := NewGeneratorWithOptions(WithC4IDs(false), WithGitignore(false))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range manifest.Entries {
		names = append(names, e.Name)
	}

	if !containsName(names, "keep.txt") {
		t.Error("keep.txt should be present")
	}
	if !containsName(names, "error.log") {
		t.Error("error.log should be present when gitignore is disabled")
	}
}

func TestGeneratorNegationPattern(t *testing.T) {
	tmpdir := t.TempDir()

	// Exclude all .log files except important.log
	os.WriteFile(filepath.Join(tmpdir, ".gitignore"), []byte("*.log\n!important.log\n"), 0644)

	os.WriteFile(filepath.Join(tmpdir, "error.log"), []byte("error"), 0644)
	os.WriteFile(filepath.Join(tmpdir, "important.log"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(tmpdir, "readme.md"), []byte("readme"), 0644)

	g := NewGeneratorWithOptions(WithC4IDs(false))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range manifest.Entries {
		names = append(names, e.Name)
	}

	if containsName(names, "error.log") {
		t.Error("error.log should be excluded")
	}
	if !containsName(names, "important.log") {
		t.Error("important.log should be present (negation pattern)")
	}
	if !containsName(names, "readme.md") {
		t.Error("readme.md should be present")
	}
}

func TestGeneratorAlwaysSkipsGitDir(t *testing.T) {
	tmpdir := t.TempDir()

	// Create .git directory (simulating a git repo)
	gitdir := filepath.Join(tmpdir, ".git")
	os.Mkdir(gitdir, 0755)
	os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/main"), 0644)

	// Create regular files
	os.WriteFile(filepath.Join(tmpdir, "main.go"), []byte("main"), 0644)

	// Even with --hidden flag, .git should be skipped
	g := NewGeneratorWithOptions(WithC4IDs(false), WithHidden(true))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range manifest.Entries {
		if e.Name == ".git/" || e.Name == ".git" {
			t.Error(".git directory should always be skipped")
		}
		if e.Name == "HEAD" {
			t.Error("files inside .git should not appear")
		}
	}

	if !containsEntry(manifest, "main.go") {
		t.Error("main.go should be present")
	}
}

func TestGeneratorDirectoryOnlyPattern(t *testing.T) {
	tmpdir := t.TempDir()

	// Exclude node_modules/ (directory-only pattern)
	os.WriteFile(filepath.Join(tmpdir, ".gitignore"), []byte("node_modules/\n"), 0644)

	// Create node_modules directory with files
	nmdir := filepath.Join(tmpdir, "node_modules")
	os.Mkdir(nmdir, 0755)
	os.WriteFile(filepath.Join(nmdir, "pkg.json"), []byte("{}"), 0644)

	// Create a file named node_modules (not a directory)
	os.WriteFile(filepath.Join(tmpdir, "src"), []byte("src"), 0644)

	g := NewGeneratorWithOptions(WithC4IDs(false))
	manifest, err := g.GenerateFromPath(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range manifest.Entries {
		if e.Name == "node_modules/" {
			t.Error("node_modules/ directory should be excluded")
		}
		if e.Name == "pkg.json" {
			t.Error("files inside node_modules should not appear")
		}
	}
}

func containsName(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

func containsEntry(manifest *Manifest, name string) bool {
	for _, e := range manifest.Entries {
		if e.Name == name {
			return true
		}
	}
	return false
}
