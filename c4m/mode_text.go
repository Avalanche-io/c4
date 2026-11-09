package c4m

import "os"

// modeText renders the entry's mode field for every form. A null mode
// (Mode == 0) is "-"; a known mode is its ten characters, the known zero
// (ZeroMode) among them as "----------".
//
// The type character must agree with the name (SPECIFICATION.md "Null
// and Zero Values"): the name's trailing "/" says the entry is a
// directory, and a directory's known zero is "d---------", never
// "----------", which on a directory line is the legacy null. So an
// entry built in memory with a known mode whose type bits say "regular
// file" on a directory name is written with the directory type; without
// this an Entry{Mode: ZeroMode, Name: "d/"} encoded as "----------" and
// decoded back as null (found by absfs, 2026-11-09). Other type bits are
// left as given: a symlink or device type on a directory name is a
// malformed entry, not one to repair silently.
func (e *Entry) modeText() string {
	if e.Mode == 0 {
		return "-"
	}
	s := formatMode(e.Mode)
	if s[0] == '-' && e.IsDir() && e.Mode&os.ModeType == 0 {
		return "d" + s[1:]
	}
	return s
}
