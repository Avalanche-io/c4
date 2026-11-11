package reconcile

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The shared platform-name vector (c4m/testdata/name-refusal-vector): one
// case per line, `<goos> <hold|refuse> <class> <Go-quoted name>`. Core's
// materialisers and c4-operator's fsname both read it, so what one refuses
// the other refuses too. Every line must agree with nameRefusal, class
// included.
func TestNameRefusalVector(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "c4m", "testdata", "name-refusal-vector", "name-refusals.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) != 4 {
			t.Fatalf("malformed line %q", line)
		}
		goos, verdict, class := parts[0], parts[1], parts[2]
		name, err := strconv.Unquote(parts[3])
		if err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		gotClass, reason := nameRefusal(goos, name)
		switch verdict {
		case "hold":
			if reason != "" {
				t.Errorf("%s %q: refused (%s: %s), vector holds it", goos, name, gotClass, reason)
			}
		case "refuse":
			if gotClass != class {
				t.Errorf("%s %q: class %q, vector says %q", goos, name, gotClass, class)
			}
		default:
			t.Fatalf("line %q: verdict %q", line, verdict)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 1941 {
		t.Errorf("read %d cases, want 1941", n)
	}
}
