package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// flags is a minimal flag parser supporting short (-e) and long (--ergonomic)
// flags with no external dependencies. It replaces pflag for the c4 CLI.
type flags struct {
	name    string
	defs    []*flagDef
	args    []string // remaining positional arguments after parsing
	parsed  bool
}

type flagKind int

const (
	flagBool flagKind = iota
	flagString
	flagInt
	flagStringArray
)

type flagDef struct {
	long     string
	short    byte // 0 if no short form
	kind     flagKind
	boolVal  *bool
	strVal   *string
	intVal   *int
	arrVal   *[]string
	defBool  bool
	defStr   string
	defInt   int
	usage    string
}

func newFlags(name string) *flags {
	return &flags{name: name}
}

func (f *flags) boolFlag(long string, short byte, def bool, usage string) *bool {
	v := new(bool)
	*v = def
	f.defs = append(f.defs, &flagDef{long: long, short: short, kind: flagBool, boolVal: v, defBool: def, usage: usage})
	return v
}

func (f *flags) stringFlag(long string, short byte, def string, usage string) *string {
	v := new(string)
	*v = def
	f.defs = append(f.defs, &flagDef{long: long, short: short, kind: flagString, strVal: v, defStr: def, usage: usage})
	return v
}

func (f *flags) intFlag(long string, short byte, def int, usage string) *int {
	v := new(int)
	*v = def
	f.defs = append(f.defs, &flagDef{long: long, short: short, kind: flagInt, intVal: v, defInt: def, usage: usage})
	return v
}

func (f *flags) stringArrayFlag(long string, usage string) *[]string {
	v := new([]string)
	f.defs = append(f.defs, &flagDef{long: long, kind: flagStringArray, arrVal: v, usage: usage})
	return v
}

func (f *flags) findByLong(name string) *flagDef {
	for _, d := range f.defs {
		if d.long == name {
			return d
		}
	}
	return nil
}

func (f *flags) findByShort(ch byte) *flagDef {
	for _, d := range f.defs {
		if d.short == ch {
			return d
		}
	}
	return nil
}

func (f *flags) parse(args []string) {
	f.parsed = true
	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "--" {
			f.args = append(f.args, args[i+1:]...)
			return
		}

		if strings.HasPrefix(arg, "--") {
			// Long flag: --name or --name=value or --name value
			rest := arg[2:]
			var name, val string
			hasVal := false
			if idx := strings.IndexByte(rest, '='); idx >= 0 {
				name = rest[:idx]
				val = rest[idx+1:]
				hasVal = true
			} else {
				name = rest
			}

			d := f.findByLong(name)
			if d == nil {
				fmt.Fprintf(os.Stderr, "%s: unknown flag --%s\n", f.name, name)
				os.Exit(1)
			}

			switch d.kind {
			case flagBool:
				if hasVal {
					b, err := strconv.ParseBool(val)
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s: invalid bool value %q for --%s\n", f.name, val, name)
						os.Exit(1)
					}
					*d.boolVal = b
				} else {
					*d.boolVal = true
				}
			case flagString:
				if !hasVal {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "%s: flag --%s requires a value\n", f.name, name)
						os.Exit(1)
					}
					val = args[i]
				}
				*d.strVal = val
			case flagInt:
				if !hasVal {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "%s: flag --%s requires a value\n", f.name, name)
						os.Exit(1)
					}
					val = args[i]
				}
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: invalid int value %q for --%s\n", f.name, val, name)
					os.Exit(1)
				}
				*d.intVal = n
			case flagStringArray:
				if !hasVal {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "%s: flag --%s requires a value\n", f.name, name)
						os.Exit(1)
					}
					val = args[i]
				}
				*d.arrVal = append(*d.arrVal, val)
			}
			i++
			continue
		}

		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
			// Short flag: -e or -m value
			ch := arg[1]
			d := f.findByShort(ch)
			if d == nil {
				fmt.Fprintf(os.Stderr, "%s: unknown flag -%c\n", f.name, ch)
				os.Exit(1)
			}

			switch d.kind {
			case flagBool:
				*d.boolVal = true
				// Handle remaining chars as more short flags (e.g. -eS)
				for j := 2; j < len(arg); j++ {
					d2 := f.findByShort(arg[j])
					if d2 == nil {
						fmt.Fprintf(os.Stderr, "%s: unknown flag -%c\n", f.name, arg[j])
						os.Exit(1)
					}
					if d2.kind != flagBool {
						fmt.Fprintf(os.Stderr, "%s: flag -%c requires a value\n", f.name, arg[j])
						os.Exit(1)
					}
					*d2.boolVal = true
				}
			case flagString:
				var val string
				if len(arg) > 2 {
					// -mvalue (value attached)
					val = arg[2:]
				} else {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "%s: flag -%c requires a value\n", f.name, ch)
						os.Exit(1)
					}
					val = args[i]
				}
				*d.strVal = val
			case flagInt:
				var val string
				if len(arg) > 2 {
					val = arg[2:]
				} else {
					i++
					if i >= len(args) {
						fmt.Fprintf(os.Stderr, "%s: flag -%c requires a value\n", f.name, ch)
						os.Exit(1)
					}
					val = args[i]
				}
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: invalid int value %q for -%c\n", f.name, val, ch)
					os.Exit(1)
				}
				*d.intVal = n
			}
			i++
			continue
		}

		// Positional argument
		f.args = append(f.args, arg)
		i++
	}
}
