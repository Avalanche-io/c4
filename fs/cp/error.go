package cp

const (
	usage string = "usage: cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file target_file\n       cp [-R [-H | -L | -P]] [-fi | -n] [-apvX] source_file ... target_directory\n"
)

type Error string

func (e Error) Error() string {
	return string(e)
}
