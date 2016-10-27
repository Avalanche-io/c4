package client

import (
	"fmt"
	"os"

	flag "github.com/ogier/pflag"
)

// id flags
var (
	recursive_flag bool
	// version_flag      bool
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
var CpFlags *flag.FlagSet

type CPTargetList struct {
	List []string
}

func (t *CPTargetList) String() string {
	return fmt.Sprintf("%s", []string(t.List))
}

func (t *CPTargetList) Set(value string) error {
	t.List = append(t.List, value)

	return nil
}

var (
	archive_cp_flag          bool
	force_cp_flag            bool
	follow_all_links_cp_flag bool
	prompt_cp_flag           bool
	follow_links_cp_flag     bool
	noclobber_cp_flag        bool
	copy_links_cp_flag       bool
	preserve_cp_flag         bool
	RecursiveFlag            bool
	VerboseFlag              bool
	target_cp_flag           CPTargetList
	target_string_flag       string
)

var Version func() string

func init() {
	if Version == nil {
		Version = func() string {
			return "c4 test"
		}
	}

	id_message := Version() + "\n\nUsage: c4 [mode] [flags] [files]\n\n" +
		"# `id` mode \n\n" +
		"    c4 generates c4ids for all files and folders spacified.\n" +
		"    If no file is given c4 will read piped data.\n" +
		"    Output is in YAML format.\n\n" +
		"  flags:\n"
	cp_message := "\n\n" +
		"# `cp` mode (not yet implemented)\n\n" +
		"    cp mode acts as a drop in replacement for the unix cp command.\n" +
		"    It acts the same as the normal cp command, but ids files on the fly\n" +
		"    It also adds the ability to specify multiple copy targets with the -T flag.\n\n" +
		"  flags:\n"
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, id_message)
		id_flags.PrintDefaults()
		fmt.Fprintf(os.Stderr, cp_message)
		CpFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}

	id_flags = flag.NewFlagSet("id", flag.ContinueOnError)
	CpFlags = CpFlagsInit()

	// id
	// id_flags.BoolVarP(&version_flag, "version", "v", false, "Show version information.")
	id_flags.BoolVarP(&recursive_flag, "recursive", "R", false, "Recursively identify all files for the given url.")
	id_flags.BoolVarP(&absolute_flag, "absolute", "a", false, "Output absolute paths, instead of relative paths.")
	// id_flags.BoolVarP(&arg_links, "arg_links", "H", false, "If the -R option is specified, symbolic links on the command line are followed.\n          (Symbolic links encountered in the tree traversal are not followed by default.)")
	id_flags.BoolVarP(&links_flag, "links", "L", false, "All symbolic links are followed.")
	// id_flags.BoolVarP(&no_links, "no_links", "P", true, "If the -R option is specified, no symbolic links are followed.  This is the default.")
	id_flags.IntVarP(&depth, "depth", "d", 0, "Only output ids for files and folders 'depth' directories deep.")
	id_flags.BoolVarP(&include_meta, "metadata", "m", false, "Include filesystem metadata.")
	id_flags.StringVarP(&formatting_string, "formatting", "f", "id", "Output formatting options.\n          \"id\": c4id oriented.\n          \"path\": path oriented.")

}

func CpFlagsInit() *flag.FlagSet {
	fs := flag.NewFlagSet("cp", flag.ContinueOnError)
	// cp
	fs.BoolVarP(&archive_cp_flag, "archive", "a", false, "Same as -pPR options.")
	fs.BoolVarP(&force_cp_flag, "force", "f", false, "Force.")
	fs.BoolVarP(&follow_all_links_cp_flag, "follow_all_links", "H", false, "Follow all symbolic links.")
	fs.BoolVarP(&prompt_cp_flag, "prompt", "i", false, "Prompt before overwriting an existing file.")
	fs.BoolVarP(&follow_links_cp_flag, "follow_links", "L", false, "Follow links instead of copy with -R option.")
	fs.BoolVarP(&noclobber_cp_flag, "noclobber", "n", false, "Do not overwrite existing files.")
	fs.BoolVarP(&copy_links_cp_flag, "copy_links", "P", true, "Copy links with -R option (default).")
	fs.BoolVarP(&preserve_cp_flag, "preserve", "p", false, "Preserve attributes.")
	fs.BoolVarP(&RecursiveFlag, "recursive", "R", false, "Copy recursively.")
	fs.BoolVarP(&VerboseFlag, "verbose", "v", false, "Verbose output.")
	fs.VarP(&target_cp_flag, "target", "T", "Specify additional target paths, can be used more than once.")

	return fs
}
