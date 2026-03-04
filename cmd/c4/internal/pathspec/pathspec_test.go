package pathspec

import (
	"testing"
)

func knownLocation(name string) bool {
	return name == "studio" || name == "cloud" || name == "nas"
}

func TestParseLocal(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"file.txt"},
		{"renders/"},
		{"./file:with:colons"},
		{"/absolute/path"},
		{"path/to/file"},
		{"some/path:stuff"}, // left side has /, so local
		{"."},
		{".."},
	}
	for _, tt := range tests {
		p, err := Parse(tt.input, knownLocation)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if p.Type != Local {
			t.Errorf("Parse(%q).Type = %v, want Local", tt.input, p.Type)
		}
		if p.Source != tt.input {
			t.Errorf("Parse(%q).Source = %q, want %q", tt.input, p.Source, tt.input)
		}
	}
}

func TestParseCapsule(t *testing.T) {
	tests := []struct {
		input   string
		source  string
		subpath string
	}{
		{"project.c4m:", "project.c4m", ""},
		{"project.c4m:renders/", "project.c4m", "renders/"},
		{"project.c4m:renders/shot_010/", "project.c4m", "renders/shot_010/"},
		{"my.data.c4m:", "my.data.c4m", ""},
		{"scan.c4m:plate.[0001-0200].exr", "scan.c4m", "plate.[0001-0200].exr"},
	}
	for _, tt := range tests {
		p, err := Parse(tt.input, knownLocation)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if p.Type != Capsule {
			t.Errorf("Parse(%q).Type = %v, want Capsule", tt.input, p.Type)
		}
		if p.Source != tt.source {
			t.Errorf("Parse(%q).Source = %q, want %q", tt.input, p.Source, tt.source)
		}
		if p.SubPath != tt.subpath {
			t.Errorf("Parse(%q).SubPath = %q, want %q", tt.input, p.SubPath, tt.subpath)
		}
	}
}

func TestParseLocation(t *testing.T) {
	tests := []struct {
		input   string
		source  string
		subpath string
	}{
		{"studio:", "studio", ""},
		{"studio:project/renders/", "studio", "project/renders/"},
		{"cloud:backups/", "cloud", "backups/"},
		{"nas:", "nas", ""},
	}
	for _, tt := range tests {
		p, err := Parse(tt.input, knownLocation)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if p.Type != Location {
			t.Errorf("Parse(%q).Type = %v, want Location", tt.input, p.Type)
		}
		if p.Source != tt.source {
			t.Errorf("Parse(%q).Source = %q, want %q", tt.input, p.Source, tt.source)
		}
		if p.SubPath != tt.subpath {
			t.Errorf("Parse(%q).SubPath = %q, want %q", tt.input, p.SubPath, tt.subpath)
		}
	}
}

func TestParseUnknown(t *testing.T) {
	_, err := Parse("unknown:", knownLocation)
	if err == nil {
		t.Error("Parse(\"unknown:\") should error for unknown left side")
	}
}

func TestParseNilLocationFunc(t *testing.T) {
	// Without a location resolver, "studio:" should error
	_, err := Parse("studio:", nil)
	if err == nil {
		t.Error("Parse(\"studio:\", nil) should error without location resolver")
	}

	// But capsule paths still work
	p, err := Parse("project.c4m:", nil)
	if err != nil {
		t.Errorf("Parse(\"project.c4m:\", nil) error: %v", err)
	}
	if p.Type != Capsule {
		t.Errorf("Parse(\"project.c4m:\", nil).Type = %v, want Capsule", p.Type)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		spec PathSpec
		want string
	}{
		{PathSpec{Type: Local, Source: "file.txt"}, "file.txt"},
		{PathSpec{Type: Capsule, Source: "project.c4m"}, "project.c4m:"},
		{PathSpec{Type: Capsule, Source: "project.c4m", SubPath: "renders/"}, "project.c4m:renders/"},
		{PathSpec{Type: Location, Source: "studio"}, "studio:"},
		{PathSpec{Type: Location, Source: "studio", SubPath: "project/"}, "studio:project/"},
	}
	for _, tt := range tests {
		got := tt.spec.String()
		if got != tt.want {
			t.Errorf("PathSpec{%v, %q, %q}.String() = %q, want %q",
				tt.spec.Type, tt.spec.Source, tt.spec.SubPath, got, tt.want)
		}
	}
}

func TestIsRoot(t *testing.T) {
	root := PathSpec{Type: Capsule, Source: "project.c4m"}
	if !root.IsRoot() {
		t.Error("empty SubPath should be root")
	}
	nonRoot := PathSpec{Type: Capsule, Source: "project.c4m", SubPath: "renders/"}
	if nonRoot.IsRoot() {
		t.Error("non-empty SubPath should not be root")
	}
}

func TestRoundTrip(t *testing.T) {
	inputs := []string{
		"file.txt",
		"project.c4m:",
		"project.c4m:renders/",
		"studio:",
		"studio:project/renders/",
	}
	for _, input := range inputs {
		p, err := Parse(input, knownLocation)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
			continue
		}
		got := p.String()
		if got != input {
			t.Errorf("round-trip: Parse(%q).String() = %q", input, got)
		}
	}
}
