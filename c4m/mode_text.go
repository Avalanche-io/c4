package c4m

// modeText renders the entry's mode field for every form. A null mode
// (Mode == 0) is "-"; a known mode is its ten characters, the known zero
// (ZeroMode) among them as "----------".
//
// The type character must agree with the name (SPECIFICATION.md "Null
// and Zero Values"): the name's trailing "/" says the entry is a
// directory, and a directory's known zero is "d---------", never
// "----------", which on a directory line is the legacy null. So an
// Entry{Mode: ZeroMode, Name: "d/"} is written "d---------"; without this
// it encoded as "----------" and decoded back as null (found by absfs,
// 2026-11-09). Only the known zero is rewritten: it is the one spelling
// the decoder reads two ways. Any other known mode is written as given,
// type character included (a flow link on a directory is written with
// the type its entry carries, and round-trips byte for byte).
func (e *Entry) modeText() string {
	if e.Mode == 0 {
		return "-"
	}
	s := formatMode(e.Mode)
	if e.Mode == ZeroMode && e.IsDir() {
		return "d" + s[1:]
	}
	return s
}
