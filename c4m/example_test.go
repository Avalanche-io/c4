package c4m_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func ExampleNewDecoder() {
	input := `@c4m 1.0
-rw-r--r-- 2025-01-01T12:00:00Z 1024 README.md c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111
drwxr-xr-x 2025-01-01T12:00:00Z 4096 src/
  -rw-r--r-- 2025-01-01T12:00:00Z 2048 main.go c42222222222222222222222222222222222222222222222222222222222222222222222222222222222222222
`
	decoder := c4m.NewDecoder(strings.NewReader(input))
	manifest, err := decoder.Decode()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Version: %s\n", manifest.Version)
	fmt.Printf("Entries: %d\n", len(manifest.Entries))
	// Output:
	// Version: 1.0
	// Entries: 3
}

func ExampleNewEncoder() {
	manifest := c4m.NewManifest()
	manifest.AddEntry(&c4m.Entry{
		Name:      "hello.txt",
		Mode:      0644,
		Size:      13,
		Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader("Hello, World!")),
	})

	encoder := c4m.NewEncoder(os.Stdout)
	if err := encoder.Encode(manifest); err != nil {
		fmt.Println("Error:", err)
	}
	// Output:
	// @c4m 1.0
	// -rw-r--r-- 2025-01-01T12:00:00Z 13 hello.txt c4278VoUM5dXnzULoTV6JqiyoeyFaL4DZo2oDPTsmDAE4Ki4Uwe8PZyENUh9uBHhWQ5HCvgb72Emg4nSazsTRophmx
}

func ExampleEncoder_SetPretty() {
	manifest := c4m.NewManifest()
	manifest.AddEntry(&c4m.Entry{
		Name:      "document.pdf",
		Mode:      0644,
		Size:      1234567,
		Timestamp: time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader("test content")),
	})

	var buf strings.Builder
	encoder := c4m.NewEncoder(&buf).SetPretty(true)
	if err := encoder.Encode(manifest); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Pretty output has formatted sizes and local timestamps
	output := buf.String()
	fmt.Println(strings.Contains(output, "1,234,567"))
	// Output:
	// true
}

func ExampleMarshal() {
	manifest := c4m.NewManifest()
	manifest.AddEntry(&c4m.Entry{
		Name:      "test.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	data, err := c4m.Marshal(manifest)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Print(string(data))
	// Output:
	// @c4m 1.0
	// -rw-r--r-- 2025-01-01T00:00:00Z 100 test.txt
}

func ExampleUnmarshal() {
	data := []byte(`@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt
`)
	manifest, err := c4m.Unmarshal(data)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Files: %d\n", len(manifest.Entries))
	fmt.Printf("Name: %s\n", manifest.Entries[0].Name)
	// Output:
	// Files: 1
	// Name: file.txt
}

func ExampleFormat() {
	// Parse and re-format a manifest
	input := []byte(`@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt
`)
	formatted, err := c4m.Format(input)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Print(string(formatted))
	// Output:
	// @c4m 1.0
	// -rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt
}

func ExampleValidator() {
	input := `@c4m 1.0
-rw-r--r-- 2025-01-01T12:00:00Z 1024 valid.txt c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111
`
	validator := c4m.NewValidator(true)
	err := validator.ValidateManifest(strings.NewReader(input))
	if err != nil {
		fmt.Println("Invalid:", err)
		return
	}

	stats := validator.GetStats()
	fmt.Printf("Valid manifest with %d entries\n", stats.TotalEntries)
	// Output:
	// Valid manifest with 1 entries
}

func ExampleNewBuilder() {
	// Build a manifest with the fluent builder API
	// Depth is computed automatically - no manual tracking needed
	m := c4m.NewBuilder().
		AddFile("readme.txt", c4m.WithSize(100)).
		AddDir("src").
			AddFile("main.go", c4m.WithSize(500)).
			AddDir("internal").
				AddFile("helper.go", c4m.WithSize(200)).
			EndDir().
			AddFile("util.go", c4m.WithSize(150)).
		End().
		AddFile("go.mod", c4m.WithSize(50)).
		MustBuild()

	fmt.Printf("Total entries: %d\n", len(m.Entries))

	// Verify depth was computed correctly
	helper := m.GetEntry("helper.go")
	fmt.Printf("helper.go depth: %d\n", helper.Depth)

	// Output:
	// Total entries: 7
	// helper.go depth: 2
}

func ExampleManifest_Builder() {
	// Add to an existing manifest using the builder
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{Name: "existing.txt", Depth: 0, Size: 100})

	// Continue building with the fluent API
	m.Builder().
		AddFile("new.txt", c4m.WithSize(50)).
		AddDir("newdir").
			AddFile("nested.txt", c4m.WithSize(25)).
		End()

	fmt.Printf("Total entries: %d\n", len(m.Entries))
	// Output:
	// Total entries: 4
}

func ExampleManifest_Children() {
	// Navigate manifest hierarchy using tree methods
	m := c4m.NewBuilder().
		AddDir("project").
			AddFile("README.md", c4m.WithSize(1000)).
			AddFile("main.go", c4m.WithSize(2000)).
			AddDir("pkg").
				AddFile("lib.go", c4m.WithSize(500)).
			EndDir().
		End().
		MustBuild()

	// Get direct children of project/
	project := m.GetEntry("project/")
	children := m.Children(project)

	fmt.Printf("project/ has %d direct children\n", len(children))
	for _, c := range children {
		fmt.Printf("  - %s\n", c.Name)
	}
	// Output:
	// project/ has 3 direct children
	//   - README.md
	//   - main.go
	//   - pkg/
}

func ExampleManifest_Ancestors() {
	m := c4m.NewBuilder().
		AddDir("a").
			AddDir("b").
				AddDir("c").
					AddFile("deep.txt").
				EndDir().
			EndDir().
		End().
		MustBuild()

	deep := m.GetEntry("deep.txt")
	ancestors := m.Ancestors(deep)

	fmt.Printf("deep.txt has %d ancestors:\n", len(ancestors))
	for _, a := range ancestors {
		fmt.Printf("  %s (depth %d)\n", a.Name, a.Depth)
	}
	// Output:
	// deep.txt has 3 ancestors:
	//   c/ (depth 2)
	//   b/ (depth 1)
	//   a/ (depth 0)
}

func ExampleManifest_Root() {
	m := c4m.NewBuilder().
		AddFile("root1.txt").
		AddDir("dir").
			AddFile("nested.txt").
		End().
		AddFile("root2.txt").
		MustBuild()

	roots := m.Root()
	fmt.Printf("Root entries: %d\n", len(roots))
	for _, r := range roots {
		fmt.Printf("  %s\n", r.Name)
	}
	// Output:
	// Root entries: 3
	//   root1.txt
	//   dir/
	//   root2.txt
}
