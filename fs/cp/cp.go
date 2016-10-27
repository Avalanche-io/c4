package cp

import "os"

func CpMain(io *IoHandler, recursive bool, verbose bool) {

	if io == nil {
		return
	}
	for _, file := range io.Files() {
		if file == "" {
			continue
		}
		if info, err := os.Stat(file); err != nil {
			io.IfError(err)
		} else if info.IsDir() {
			if recursive {
				io.Walk(file, verbose)
			} else {
				io.IfError(dirError(file))
			}
		} else {
			if verbose {
				io.LogCopy(file)
			}
			io.Copy(file, info)
		}
	}
}
