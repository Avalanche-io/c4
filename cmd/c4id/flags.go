package main

import (
	"fmt"
	"os"

	flag "github.com/ogier/pflag"
)

// id flags
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

var id_flags *flag.FlagSet

func init() {
	id_message := versionString() + "\n\nUsage: c4id [flags] [paths]\n\n" +
		"    c4 generates c4ids for all files and folders spacified.\n" +
		"    If no file is given c4 will read piped data.\n" +
		"    Output is in YAML format.\n\n" +
		"  flags:\n"
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, id_message)
		id_flags.PrintDefaults()
	}

	id_flags = flag.NewFlagSet("id", flag.ContinueOnError)

	// id
	id_flags.BoolVarP(&version_flag, "version", "v", false, "Show version information.")
	id_flags.BoolVarP(&recursive_flag, "recursive", "R", false, "Recursively identify all files for the given url.")
	id_flags.BoolVarP(&absolute_flag, "absolute", "a", false, "Output absolute paths, instead of relative paths.")
	id_flags.BoolVarP(&links_flag, "links", "L", false, "All symbolic links are followed.")
	id_flags.IntVarP(&depth, "depth", "d", 0, "Only output ids for files and folders 'depth' directories deep.")
	id_flags.BoolVarP(&include_meta, "metadata", "m", false, "Include filesystem metadata.")
	id_flags.StringVarP(&formatting_string, "formatting", "f", "id", "Output formatting options.\n          \"id\": c4id oriented.\n          \"path\": path oriented.")

}
