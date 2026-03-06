// Package container implements tar archive support for the c4 CLI.
//
// Archives are first-class locations behind the colon. The same verbs
// that work on c4m files and remote locations work on archives:
//
//	c4 ls archive.tar:               # list tar contents as c4m
//	c4 cat archive.tar:src/main.go   # file content from tar to stdout
//	c4 cp ./files/ archive.tar:      # create tar from local
//	c4 cp archive.tar: ./output/     # extract tar to local
//	c4 diff old.tar: new.tar:        # diff two archives
package container

import (
	"archive/tar"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ReadManifest reads a tar archive and returns a c4m manifest of its contents.
// The format parameter specifies compression: "tar", "gzip", "bzip2".
func ReadManifest(archivePath, format string) (*c4m.Manifest, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tr, err := newTarReader(f, format)
	if err != nil {
		return nil, err
	}

	// dirEntries holds explicitly seen directory metadata.
	dirEntries := make(map[string]*c4m.Entry)

	// tree maps directory path → list of child names (both files and subdirs).
	// Files are stored as-is; directories end with "/".
	tree := make(map[string][]*c4m.Entry)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		if hdr.Typeflag == tar.TypeXHeader || hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		name := path.Clean(hdr.Name)
		if name == "." {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			entry := &c4m.Entry{
				Mode:      hdr.FileInfo().Mode(),
				Timestamp: hdr.ModTime.UTC(),
				Name:      path.Base(name) + "/",
			}
			dirEntries[name] = entry
			ensureParents(tree, dirEntries, name)

		case tar.TypeSymlink:
			entry := &c4m.Entry{
				Mode:       hdr.FileInfo().Mode(),
				Timestamp:  hdr.ModTime.UTC(),
				Name:       path.Base(name),
				Target: hdr.Linkname,
			}
			parent := path.Dir(name)
			if parent == "." {
				parent = ""
			}
			ensureParents(tree, dirEntries, name)
			tree[parent] = append(tree[parent], entry)

		default:
			// Regular file — compute C4 ID
			id := c4.Identify(tr)
			entry := &c4m.Entry{
				Mode:      hdr.FileInfo().Mode(),
				Timestamp: hdr.ModTime.UTC(),
				Size:      hdr.Size,
				Name:      path.Base(name),
				C4ID:      id,
			}
			parent := path.Dir(name)
			if parent == "." {
				parent = ""
			}
			ensureParents(tree, dirEntries, name)
			tree[parent] = append(tree[parent], entry)
		}
	}

	// Build nested manifest from the tree
	m := c4m.NewManifest()
	writeDir(m, tree, dirEntries, "", 0)
	return m, nil
}

// ensureParents creates implicit directory entries for all ancestors of p.
func ensureParents(tree map[string][]*c4m.Entry, dirEntries map[string]*c4m.Entry, p string) {
	dir := path.Dir(p)
	if dir == "." || dir == "" {
		return
	}

	// Walk up creating missing directories
	parts := strings.Split(dir, "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/")
		if _, ok := dirEntries[d]; !ok {
			dirEntries[d] = &c4m.Entry{
				Mode: os.ModeDir | 0755,
				Name: parts[i] + "/",
			}
		}
		parent := ""
		if i > 0 {
			parent = strings.Join(parts[:i], "/")
		}
		// Add dir to parent's children if not already there
		if !hasChild(tree[parent], parts[i]+"/") {
			tree[parent] = append(tree[parent], dirEntries[d])
		}
	}
}

func hasChild(children []*c4m.Entry, name string) bool {
	for _, c := range children {
		if c.Name == name {
			return true
		}
	}
	return false
}

// writeDir recursively writes directory contents to the manifest.
func writeDir(m *c4m.Manifest, tree map[string][]*c4m.Entry, dirEntries map[string]*c4m.Entry, dirPath string, depth int) {
	children := tree[dirPath]
	if len(children) == 0 {
		return
	}

	// Separate files and directories, then sort each group
	var files, dirs []*c4m.Entry
	for _, e := range children {
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	// Files first, then directories (c4m sort order)
	for _, e := range files {
		entry := *e
		entry.Depth = depth
		m.AddEntry(&entry)
	}
	for _, e := range dirs {
		entry := *e
		entry.Depth = depth
		m.AddEntry(&entry)

		// Recurse into subdirectory
		subPath := e.Name[:len(e.Name)-1] // strip trailing /
		if dirPath != "" {
			subPath = dirPath + "/" + subPath
		}
		writeDir(m, tree, dirEntries, subPath, depth+1)
	}
}

// CatFile extracts a single file's content from a tar archive.
// The caller must close the returned ReadCloser.
func CatFile(archivePath, format, filePath string) (io.ReadCloser, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}

	tr, err := newTarReader(f, format)
	if err != nil {
		f.Close()
		return nil, err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			f.Close()
			return nil, fmt.Errorf("%s not found in archive", filePath)
		}
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		name := path.Clean(hdr.Name)
		if name == filePath {
			if hdr.Typeflag == tar.TypeDir {
				f.Close()
				return nil, fmt.Errorf("%s is a directory, use c4 ls", filePath)
			}
			return &fileCloser{Reader: tr, closer: f}, nil
		}
	}
}

// WriteTar creates a tar archive from a manifest.
// getContent returns a ReadCloser for each file entry given its full path.
func WriteTar(archivePath, format string, manifest *c4m.Manifest, getContent func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error)) error {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	tw, compressor, err := newTarWriter(f, format)
	if err != nil {
		return err
	}

	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}

		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}

		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name

			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     fullPath,
				Mode:     int64(entry.Mode.Perm()),
				ModTime:  entry.Timestamp,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("writing dir header %s: %w", fullPath, err)
			}
			continue
		}

		if entry.IsSymlink() {
			hdr := &tar.Header{
				Typeflag: tar.TypeSymlink,
				Name:     strings.TrimSuffix(fullPath, "/"),
				Linkname: entry.Target,
				Mode:     int64(entry.Mode.Perm()),
				ModTime:  entry.Timestamp,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("writing symlink header %s: %w", fullPath, err)
			}
			continue
		}

		rc, err := getContent(fullPath, entry)
		if err != nil {
			return fmt.Errorf("content for %s: %w", fullPath, err)
		}

		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     fullPath,
			Size:     entry.Size,
			Mode:     int64(entry.Mode.Perm()),
			ModTime:  entry.Timestamp,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			rc.Close()
			return fmt.Errorf("writing file header %s: %w", fullPath, err)
		}
		if _, err := io.Copy(tw, rc); err != nil {
			rc.Close()
			return fmt.Errorf("writing file content %s: %w", fullPath, err)
		}
		rc.Close()
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("finalizing tar: %w", err)
	}
	if compressor != nil {
		if err := compressor.Close(); err != nil {
			return fmt.Errorf("finalizing compression: %w", err)
		}
	}
	return nil
}

func newTarReader(r io.Reader, format string) (*tar.Reader, error) {
	switch format {
	case "gzip":
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		return tar.NewReader(gr), nil
	case "bzip2":
		return tar.NewReader(bzip2.NewReader(r)), nil
	case "tar":
		return tar.NewReader(r), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func newTarWriter(w io.Writer, format string) (*tar.Writer, io.Closer, error) {
	switch format {
	case "gzip":
		gw := gzip.NewWriter(w)
		return tar.NewWriter(gw), gw, nil
	case "tar":
		return tar.NewWriter(w), nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported write format: %s (only tar and gzip)", format)
	}
}

type fileCloser struct {
	io.Reader
	closer io.Closer
}

func (fc *fileCloser) Close() error {
	return fc.closer.Close()
}
