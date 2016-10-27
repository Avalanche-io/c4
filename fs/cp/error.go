package cp

const (
	usage string = "usage: cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file target_file\n       cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file ... target_directory\n"
)

type cpError string

func (e cpError) Error() string {
	return string(e)
}

type dirError string

func (e dirError) Error() string {
	return "cp: " + string(e) + " is a directory (not copied).\n"
}
