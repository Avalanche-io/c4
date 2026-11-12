package main

import (
	"crypto/sha512"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// independentC4ID computes a C4 ID from first principles: SHA-512 of the
// bytes, base58 with the C4 alphabet, "c4" prefix, padded to 90 characters.
// It shares no code with the c4 package, so a wrong ID there cannot hide
// here.
func independentC4ID(data []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	sum := sha512.Sum512(data)
	n := new(big.Int).SetBytes(sum[:])
	base, mod := big.NewInt(58), new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append([]byte{alphabet[mod.Int64()]}, out...)
	}
	s := string(out)
	for len(s) < 88 {
		s = "1" + s
	}
	return "c4" + s
}

// A file's C4 ID summons exactly that file, on every path (Joshua,
// 2026-11-03; design/no-self-id.md on master). The file here holds a
// pretty (-e) c4m: it parses as a description, but it is not canonical, so
// re-rendering it before hashing changes the bytes. Through 1.0.26 only a
// file inside a scanned folder got its byte ID; `c4 id FILE` (any name),
// stdin and `c4 id -s` used the re-rendered text, the store kept that text,
// and `c4 cat FILE` wrote it out. A .c4m file given to `c4 id` as an
// argument is still read as a description, as on master.
func TestFileIDIsItsBytesOnEveryPath(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}

	if got := independentC4ID(nil); got != "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT" {
		t.Fatalf("the independent encoder is wrong: %s", got)
	}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"a.txt": "a\n", "d/b": "b\n"})
	pretty, stderr, code := runC4WithEnv(t, bin, env, "id", "-e", src)
	if code != 0 {
		t.Fatalf("c4 id -e: exit %d: %s", code, stderr)
	}
	canonical, _, code := runC4WithEnv(t, bin, env, "id", src)
	if code != 0 || canonical == pretty {
		t.Fatalf("need a pretty form that differs from the canonical one (exit %d)", code)
	}
	want := independentC4ID([]byte(pretty))

	files := filepath.Join(dir, "files")
	bmust(t, os.MkdirAll(files, 0o755))
	for _, name := range []string{"tree.c4m", "tree.bin", "tree.txt"} {
		bmust(t, os.WriteFile(filepath.Join(files, name), []byte(pretty), 0o644))
	}
	entryID := func(listing, name string) string {
		t.Helper()
		for _, line := range strings.Split(listing, "\n") {
			f := strings.Fields(line)
			if len(f) >= 5 && f[len(f)-2] == name {
				return f[len(f)-1]
			}
		}
		t.Fatalf("no entry %q in:\n%s", name, listing)
		return ""
	}
	cat := func(id string) string {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
		if code != 0 {
			t.Fatalf("c4 cat %s: exit %d: %s", id, code, stderr)
		}
		return out
	}

	// 1. A file inside a scanned folder (fixed before 1.0.27; pinned here too).
	out, _, _ := runC4WithEnv(t, bin, env, "id", files)
	for _, name := range []string{"tree.c4m", "tree.bin", "tree.txt"} {
		if got := entryID(out, name); got != want {
			t.Errorf("scan: %s got %s, want the byte ID %s", name, got, want)
		}
	}

	// 2. `c4 id FILE`, any name but .c4m.
	for _, name := range []string{"tree.bin", "tree.txt"} {
		out, stderr, code := runC4WithEnv(t, bin, env, "id", filepath.Join(files, name))
		if code != 0 {
			t.Fatalf("c4 id %s: exit %d: %s", name, code, stderr)
		}
		if got := entryID(out, name); got != want {
			t.Errorf("c4 id %s: got %s, want the byte ID %s", name, got, want)
		}
	}

	// 3. stdin.
	out, stderr, code = runC4WithStdinEnv(t, bin, env, pretty, "id")
	if code != 0 {
		t.Fatalf("c4 id < file: exit %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("c4 id < file: got %s, want the byte ID %s", got, want)
	}

	// 4. Store from stdin, then cat the ID: the same bytes.
	out, stderr, code = runC4WithStdinEnv(t, bin, env, pretty, "id", "-s")
	if code != 0 {
		t.Fatalf("c4 id -s < file: exit %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("c4 id -s < file: got %s, want the byte ID %s", got, want)
	} else if back := cat(got); back != pretty {
		t.Errorf("c4 cat of the stored ID returned other bytes:\n%s", back)
	}

	// 5. Store a file argument, then cat the entry's ID: the same bytes.
	out, stderr, code = runC4WithEnv(t, bin, env, "id", "-s", filepath.Join(files, "tree.bin"))
	if code != 0 {
		t.Fatalf("c4 id -s tree.bin: exit %d: %s", code, stderr)
	}
	if got := entryID(out, "tree.bin"); got != want {
		t.Errorf("c4 id -s tree.bin: got %s, want the byte ID %s", got, want)
	} else if back := cat(got); back != pretty {
		t.Errorf("c4 cat of tree.bin's stored ID returned other bytes:\n%s", back)
	}

	// `c4 cat FILE` without a rendering flag writes the file's bytes.
	for _, name := range []string{"tree.c4m", "tree.bin"} {
		out, stderr, code := runC4WithEnv(t, bin, env, "cat", filepath.Join(files, name))
		if code != 0 {
			t.Fatalf("c4 cat %s: exit %d: %s", name, code, stderr)
		}
		if out != pretty {
			t.Errorf("c4 cat %s did not return the file's bytes:\n%s", name, out)
		}
	}

	// The intended exception, as on master: a .c4m argument to `c4 id` is
	// read as a description and written in canonical form.
	out, stderr, code = runC4WithEnv(t, bin, env, "id", filepath.Join(files, "tree.c4m"))
	if code != 0 {
		t.Fatalf("c4 id tree.c4m: exit %d: %s", code, stderr)
	}
	if out != canonical {
		t.Errorf("c4 id tree.c4m should print the canonical description:\n--- got\n%s--- want\n%s", out, canonical)
	}
}
