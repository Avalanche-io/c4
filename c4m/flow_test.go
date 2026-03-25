package c4m

import (
	"os"
	"strings"
	"testing"
)

func TestFlowLinkParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDir   FlowDirection
		wantTgt   string
		wantName  string
	}{
		{
			name:     "outbound flow to location root",
			input:    "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas: -\n",
			wantDir:  FlowOutbound,
			wantTgt:  "nas:",
			wantName: "footage/",
		},
		{
			name:     "outbound flow with path",
			input:    "drwxr-xr-x 2024-01-01T00:00:00Z 0 deliverables/ -> client:project_x/ -\n",
			wantDir:  FlowOutbound,
			wantTgt:  "client:project_x/",
			wantName: "deliverables/",
		},
		{
			name:     "inbound flow",
			input:    "dr-xr-xr-x 2024-01-01T00:00:00Z 0 plates/ <- studio:plates/ -\n",
			wantDir:  FlowInbound,
			wantTgt:  "studio:plates/",
			wantName: "plates/",
		},
		{
			name:     "bidirectional flow",
			input:    "drwxrwxrwx 2024-01-01T00:00:00Z 0 shared/ <> nas:project/shared/ -\n",
			wantDir:  FlowBidirectional,
			wantTgt:  "nas:project/shared/",
			wantName: "shared/",
		},
		{
			name:     "outbound flow on file",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 1048576 final.exr -> review: -\n",
			wantDir:  FlowOutbound,
			wantTgt:  "review:",
			wantName: "final.exr",
		},
		{
			name:     "inbound flow on file",
			input:    "-r--r--r-- 2024-01-01T00:00:00Z 524288 lut.cube <- color:standards/lut.cube -\n",
			wantDir:  FlowInbound,
			wantTgt:  "color:standards/lut.cube",
			wantName: "lut.cube",
		},
		{
			name:     "location with hyphens and underscores",
			input:    "drwxr-xr-x 2024-01-01T00:00:00Z 0 finals/ -> studio-a_2:project/finals/ -\n",
			wantDir:  FlowOutbound,
			wantTgt:  "studio-a_2:project/finals/",
			wantName: "finals/",
		},
		{
			name:     "no flow link",
			input:    "drwxr-xr-x 2024-01-01T00:00:00Z 0 normal/ -\n",
			wantDir:  FlowNone,
			wantTgt:  "",
			wantName: "normal/",
		},
		{
			name:     "inbound flow with null metadata",
			input:    "dr-xr-xr-x - - reference/ <- library:assets/ref/ -\n",
			wantDir:  FlowInbound,
			wantTgt:  "library:assets/ref/",
			wantName: "reference/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Unmarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			e := m.Entries[0]
			if e.FlowDirection != tt.wantDir {
				t.Errorf("FlowDirection = %d, want %d", e.FlowDirection, tt.wantDir)
			}
			if e.FlowTarget != tt.wantTgt {
				t.Errorf("FlowTarget = %q, want %q", e.FlowTarget, tt.wantTgt)
			}
			if e.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", e.Name, tt.wantName)
			}
		})
	}
}

func TestFlowLinkRoundTrip(t *testing.T) {
	inputs := []string{
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/ -\n",
		"dr-xr-xr-x 2024-01-01T00:00:00Z 0 plates/ <- studio:plates/ -\n",
		"drwxrwxrwx 2024-01-01T00:00:00Z 0 shared/ <> nas:project/shared/ -\n",
		"-rw-r--r-- 2024-01-01T00:00:00Z 1048576 final.exr -> review: -\n",
		"--w------- 2024-01-01T00:00:00Z 0 upload/ -> ingest:incoming/ -\n",
	}

	for _, input := range inputs {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			m, err := Unmarshal([]byte(input))
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			out, err := Marshal(m)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			if string(out) != input {
				t.Errorf("round-trip mismatch:\n  got:  %q\n  want: %q", string(out), input)
			}

			// Parse again and verify identical
			m2, err := Unmarshal(out)
			if err != nil {
				t.Fatalf("second Unmarshal failed: %v", err)
			}
			if len(m2.Entries) != 1 || len(m.Entries) != 1 {
				t.Fatal("entry count mismatch")
			}
			e1, e2 := m.Entries[0], m2.Entries[0]
			if e1.FlowDirection != e2.FlowDirection || e1.FlowTarget != e2.FlowTarget {
				t.Errorf("flow fields differ after round-trip")
			}
		})
	}
}

func TestFlowArrowDisambiguation(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFlow   FlowDirection
		wantTarget string // symlink target (not flow)
		wantHL     int    // hard link marker
		wantFlowT  string // flow target
	}{
		{
			name:     "hard link group ->1",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt ->1 -\n",
			wantHL:   1,
			wantFlow: FlowNone,
		},
		{
			name:     "ungrouped hard link -> with c4 ID",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt -> c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111\n",
			wantHL:   -1,
			wantFlow: FlowNone,
		},
		{
			name:     "ungrouped hard link -> with null ID",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt -> -\n",
			wantHL:   -1,
			wantFlow: FlowNone,
		},
		{
			name:       "symlink target (path, no colon)",
			input:      "lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> ../other/file -\n",
			wantTarget: "../other/file",
			wantFlow:   FlowNone,
		},
		{
			name:      "flow target (location:path)",
			input:     "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/ -\n",
			wantFlow:  FlowOutbound,
			wantFlowT: "nas:raw/",
		},
		{
			name:      "flow target (bare location:)",
			input:     "-rw-r--r-- 2024-01-01T00:00:00Z 100 file.txt -> backup: -\n",
			wantFlow:  FlowOutbound,
			wantFlowT: "backup:",
		},
		{
			name:      "flow target with c4-prefixed location name",
			input:     "drwxr-xr-x 2024-01-01T00:00:00Z 0 outbox/ -> c4studio:inbox/ -\n",
			wantFlow:  FlowOutbound,
			wantFlowT: "c4studio:inbox/",
		},
		{
			name:      "flow target with c4-only location name",
			input:     "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> c4:renders/ -\n",
			wantFlow:  FlowOutbound,
			wantFlowT: "c4:renders/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Unmarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			e := m.Entries[0]
			if e.FlowDirection != tt.wantFlow {
				t.Errorf("FlowDirection = %d, want %d", e.FlowDirection, tt.wantFlow)
			}
			if e.FlowTarget != tt.wantFlowT {
				t.Errorf("FlowTarget = %q, want %q", e.FlowTarget, tt.wantFlowT)
			}
			if e.Target != tt.wantTarget {
				t.Errorf("Target = %q, want %q", e.Target, tt.wantTarget)
			}
			if e.HardLink != tt.wantHL {
				t.Errorf("HardLink = %d, want %d", e.HardLink, tt.wantHL)
			}
		})
	}
}

func TestFlowEscapedNamesNotMisinterpreted(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantFlow FlowDirection
	}{
		{
			name:     "escaped name containing <- is not a flow operator",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 a\\ <-\\ b -\n",
			wantName: "a <- b",
			wantFlow: FlowNone,
		},
		{
			name:     "escaped name containing <> is not a flow operator",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 a\\ <>\\ b -\n",
			wantName: "a <> b",
			wantFlow: FlowNone,
		},
		{
			name:     "escaped name containing -> is not a flow operator",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 report\\ ->\\ draft -\n",
			wantName: "report -> draft",
			wantFlow: FlowNone,
		},
		{
			name:     "escaped name with <- followed by flow operator",
			input:    "-rw-r--r-- 2024-01-01T00:00:00Z 100 report\\ <-\\ draft <- edits: -\n",
			wantName: "report <- draft",
			wantFlow: FlowInbound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Unmarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			e := m.Entries[0]
			if e.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", e.Name, tt.wantName)
			}
			if e.FlowDirection != tt.wantFlow {
				t.Errorf("FlowDirection = %d, want %d", e.FlowDirection, tt.wantFlow)
			}
		})
	}
}

func TestFlowBuilderOptions(t *testing.T) {
	tests := []struct {
		name    string
		option  EntryOption
		wantDir FlowDirection
		wantTgt string
	}{
		{
			name:    "WithFlowOutbound",
			option:  WithFlowOutbound("nas:raw/"),
			wantDir: FlowOutbound,
			wantTgt: "nas:raw/",
		},
		{
			name:    "WithFlowInbound",
			option:  WithFlowInbound("studio:plates/"),
			wantDir: FlowInbound,
			wantTgt: "studio:plates/",
		},
		{
			name:    "WithFlowSync",
			option:  WithFlowSync("nas:project/shared/"),
			wantDir: FlowBidirectional,
			wantTgt: "nas:project/shared/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder()
			b.AddDir("test/", tt.option)
			m, err := b.Build()
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if len(m.Entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(m.Entries))
			}
			e := m.Entries[0]
			if e.FlowDirection != tt.wantDir {
				t.Errorf("FlowDirection = %d, want %d", e.FlowDirection, tt.wantDir)
			}
			if e.FlowTarget != tt.wantTgt {
				t.Errorf("FlowTarget = %q, want %q", e.FlowTarget, tt.wantTgt)
			}
		})
	}
}

func TestFlowHelperMethods(t *testing.T) {
	e := &Entry{}
	if e.IsFlowLinked() {
		t.Error("zero-value Entry should not be flow-linked")
	}
	if e.FlowOperator() != "" {
		t.Errorf("FlowOperator() = %q, want empty", e.FlowOperator())
	}

	e.FlowDirection = FlowOutbound
	if !e.IsFlowLinked() {
		t.Error("outbound entry should be flow-linked")
	}
	if e.FlowOperator() != "->" {
		t.Errorf("FlowOperator() = %q, want ->", e.FlowOperator())
	}

	e.FlowDirection = FlowInbound
	if e.FlowOperator() != "<-" {
		t.Errorf("FlowOperator() = %q, want <-", e.FlowOperator())
	}

	e.FlowDirection = FlowBidirectional
	if e.FlowOperator() != "<>" {
		t.Errorf("FlowOperator() = %q, want <>", e.FlowOperator())
	}
}

func TestFlowCanonicalAffectsID(t *testing.T) {
	// Two manifests: same content, one with flow link and one without.
	// They should produce different canonical output and thus different C4 IDs.
	withoutFlow := "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -\n"
	withFlow := "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/ -\n"

	m1, err := Unmarshal([]byte(withoutFlow))
	if err != nil {
		t.Fatalf("Unmarshal withoutFlow: %v", err)
	}
	m2, err := Unmarshal([]byte(withFlow))
	if err != nil {
		t.Fatalf("Unmarshal withFlow: %v", err)
	}

	c1 := m1.Entries[0].Canonical()
	c2 := m2.Entries[0].Canonical()

	if c1 == c2 {
		t.Errorf("canonical output should differ:\n  without: %q\n  with:    %q", c1, c2)
	}

	// Verify the flow entry's canonical includes the operator
	if !strings.Contains(c2, "-> nas:raw/") {
		t.Errorf("canonical should contain flow operator: %q", c2)
	}
}

func TestFlowValidatorAcceptsValidTargets(t *testing.T) {
	valid := []string{
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas: -\n",
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/plates/ -\n",
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ <- studio-a:plates/ -\n",
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ <> Backup_2:data/ -\n",
	}

	for _, input := range valid {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			v := NewValidator(false)
			err := v.ValidateManifest(strings.NewReader(input))
			if err != nil {
				t.Errorf("should be valid: %v", err)
			}
		})
	}
}

func TestFlowValidatorRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		errIn string
	}{
		{
			name:  "path traversal in flow target",
			input: "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:../escape/ -\n",
			errIn: "..",
		},
		{
			name:  "leading slash in flow target path",
			input: "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:/absolute -\n",
			errIn: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(false)
			err := v.ValidateManifest(strings.NewReader(tt.input))
			if err == nil {
				t.Error("expected validation error")
				return
			}
			if !strings.Contains(err.Error(), tt.errIn) {
				t.Errorf("error %q should contain %q", err, tt.errIn)
			}
		})
	}
}

func TestFlowIsFlowTargetFunction(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"nas:", true},
		{"nas:raw/", true},
		{"studio-a:plates/", true},
		{"A:", true},
		{"a1_b-c:", true},
		{"../other", false},
		{"-", false},
		{"c4abc", false},
		{"", false},
		{"123:", false},       // must start with letter
		{"_bad:", false},      // must start with letter
		{"/absolute", false},  // starts with /
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isFlowTarget(tt.input)
			if got != tt.want {
				t.Errorf("isFlowTarget(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFlowPrettyPrint(t *testing.T) {
	input := "drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/ -\n"
	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	pretty, err := MarshalPretty(m)
	if err != nil {
		t.Fatalf("MarshalPretty failed: %v", err)
	}

	prettyStr := string(pretty)
	if !strings.Contains(prettyStr, "-> nas:raw/") {
		t.Errorf("pretty output should contain flow operator: %q", prettyStr)
	}
}

func TestFlowMixedManifest(t *testing.T) {
	// Manifest with regular entries, flow entries, and symlinks
	input := strings.Join([]string{
		"-rw-r--r-- 2024-01-01T00:00:00Z 100 readme.txt -",
		"lrwxrwxrwx 2024-01-01T00:00:00Z 0 link -> ../target -",
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 incoming/ <- dailies: -",
		"drwxr-xr-x 2024-01-01T00:00:00Z 0 outgoing/ -> review:finals/ -",
		"drwxrwxrwx 2024-01-01T00:00:00Z 0 shared/ <> nas:project/ -",
		"",
	}, "\n")

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(m.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(m.Entries))
	}

	// Look up entries by name (auto-sort may reorder).
	byName := make(map[string]*Entry)
	for _, e := range m.Entries {
		byName[e.Name] = e
	}

	// Regular file
	if byName["readme.txt"].IsFlowLinked() {
		t.Error("readme.txt should not be flow-linked")
	}

	// Symlink
	if byName["link"].Target != "../target" {
		t.Errorf("link target = %q, want ../target", byName["link"].Target)
	}
	if byName["link"].IsFlowLinked() {
		t.Error("symlink should not be flow-linked")
	}

	// Inbound flow
	if byName["incoming/"].FlowDirection != FlowInbound {
		t.Errorf("incoming/ FlowDirection = %d, want %d", byName["incoming/"].FlowDirection, FlowInbound)
	}

	// Outbound flow
	if byName["outgoing/"].FlowDirection != FlowOutbound {
		t.Errorf("outgoing/ FlowDirection = %d, want %d", byName["outgoing/"].FlowDirection, FlowOutbound)
	}

	// Bidirectional flow
	if byName["shared/"].FlowDirection != FlowBidirectional {
		t.Errorf("shared/ FlowDirection = %d, want %d", byName["shared/"].FlowDirection, FlowBidirectional)
	}

	// Round-trip
	out, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	m2, err := Unmarshal(out)
	if err != nil {
		t.Fatalf("second Unmarshal failed: %v", err)
	}
	if len(m2.Entries) != 5 {
		t.Fatalf("round-trip: expected 5 entries, got %d", len(m2.Entries))
	}
	for i := range m.Entries {
		if m.Entries[i].FlowDirection != m2.Entries[i].FlowDirection {
			t.Errorf("entry %d FlowDirection mismatch after round-trip", i)
		}
		if m.Entries[i].FlowTarget != m2.Entries[i].FlowTarget {
			t.Errorf("entry %d FlowTarget mismatch after round-trip", i)
		}
	}
}

func TestFlowEntryFormat(t *testing.T) {
	e := &Entry{
		Mode:          os.ModeDir | 0755,
		Timestamp:     NullTimestamp(),
		Size:          0,
		Name:          "footage/",
		FlowDirection: FlowOutbound,
		FlowTarget:    "nas:raw/",
	}

	got := e.Format(0, false)
	want := "drwxr-xr-x - 0 footage/ -> nas:raw/ -"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFlowEntryCanonical(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		contains string
	}{
		{
			name: "outbound canonical",
			entry: Entry{
				Mode: os.ModeDir | 0755, Name: "footage/", Size: 0,
				Timestamp: NullTimestamp(),
				FlowDirection: FlowOutbound, FlowTarget: "nas:raw/",
			},
			contains: "-> nas:raw/",
		},
		{
			name: "inbound canonical",
			entry: Entry{
				Mode: os.ModeDir | 0555, Name: "plates/", Size: 0,
				Timestamp: NullTimestamp(),
				FlowDirection: FlowInbound, FlowTarget: "studio:plates/",
			},
			contains: "<- studio:plates/",
		},
		{
			name: "bidirectional canonical",
			entry: Entry{
				Mode: os.ModeDir | 0777, Name: "shared/", Size: 0,
				Timestamp: NullTimestamp(),
				FlowDirection: FlowBidirectional, FlowTarget: "nas:project/shared/",
			},
			contains: "<> nas:project/shared/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.Canonical()
			if !strings.Contains(got, tt.contains) {
				t.Errorf("Canonical() = %q, should contain %q", got, tt.contains)
			}
		})
	}
}
