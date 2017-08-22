package main

import (
	"fmt"
	"os"

	flag "github.com/ogier/pflag"
)

var (
	recursive_flag    bool
	version_flag      bool
	arg_links         bool
	links_flag        bool
	no_links          bool
	summery           bool
	depth             int
	include_meta      bool
	absolute_flag     bool
	formatting_string string
)

func init() {
	message := versionString() + "\n\nUsage: c4 [flags] [file]\n\n" +
		"  c4 generates C4 IDs for all files and folders spacified.\n" +
		"  If no file is given c4 will read piped data.\n\n" +
		"flags:\n"
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, message)
		flag.PrintDefaults()
	}
	flag.BoolVarP(&version_flag, "version", "v", false, "Show version information.")
	flag.BoolVarP(&recursive_flag, "recursive", "R", false, "Recursively identify all files for the given path.")
	flag.BoolVarP(&absolute_flag, "absolute", "a", false, "Output absolute paths, instead of relative paths.")
	// flag.BoolVarP(&arg_links, "arg_links", "H", false, "If the -R option is specified, symbolic links on the command line are followed.\n          (Symbolic links encountered in the tree traversal are not followed by default.)")
	flag.BoolVarP(&links_flag, "links", "L", false, "All symbolic links are followed.")
	// flag.BoolVarP(&no_links, "no_links", "P", true, "If the -R option is specified, no symbolic links are followed.  This is the default.")
	flag.IntVarP(&depth, "depth", "d", 0, "Only output ids for files and folders 'depth' directories deep.")
	flag.BoolVarP(&include_meta, "metadata", "m", false, "Include filesystem metadata.\n          \"path\" is always included unless data is piped, or only a single file is specified.")
	flag.StringVarP(&formatting_string, "formatting", "f", "id", "Output formatting options.\n          \"id\": c4id oriented.\n          \"path\": path oriented.")
}
