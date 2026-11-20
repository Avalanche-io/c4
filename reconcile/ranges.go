package reconcile

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// RangeProblem names a folded range of a target that cannot be written,
// by its full path and its C4 ID (the C4 ID of its frame ID list), and
// says why.
type RangeProblem struct {
	Path   string
	ID     c4.ID
	Reason RangeReason
	Err    error // for RangeBadList: what was wrong with the list
}

// RangeReason says why a folded range cannot be written.
type RangeReason int

const (
	// RangeNoList: neither the description nor any source holds the
	// range's ID list. The missing content is the list.
	RangeNoList RangeReason = iota
	// RangeLinks: the members are symbolic links, and a folded line does
	// not carry their link targets.
	RangeLinks
	// RangeBadList: the list was found but does not name the range's
	// frames (it does not parse, or its count differs from the range's).
	RangeBadList
)

// String is the problem as a person reads it.
func (p RangeProblem) String() string {
	switch p.Reason {
	case RangeNoList:
		return fmt.Sprintf("the ID list of %s (%s)", p.Path, p.ID)
	case RangeLinks:
		return fmt.Sprintf("%s: a folded range of symbolic links; the members' link targets are not in a folded line", p.Path)
	default:
		return fmt.Sprintf("%s: its ID list (%s) does not name its frames: %v", p.Path, p.ID, p.Err)
	}
}

// ExpandRanges returns target with every folded range replaced by its
// frames, so a materialiser can write them as ordinary files. A frame ID
// list is handled as a subdirectory listing is (ruled 2026-11-13 by the
// maintainer: "Everything that you would expect of the creation and
// retention of that subdirectory c4m file you would also expect of the
// frame id list"): it is read from the description when it is inline, and
// from src by the range's C4 ID when it is not, and it must hash to that
// ID. Each frame is named by the range expression, takes its C4 ID from
// the list, and takes the range line's mode and timestamp; its size is
// left null, since a frame's own size is not carried and comes from its
// content.
//
// Three kinds of range stay folded, and are returned so the caller can
// refuse them before anything is written:
//   - RangeNoList: neither the description nor src holds the list ("You can't
//     expand a sequence if you don't have a description of it's contents,
//     this is the same for any content", 2026-11-13). This is the ordinary
//     missing-content case, and the missing content is the list.
//   - RangeLinks: a range of symbolic links. A folded line carries the members'
//     C4 IDs but not their link targets, so the links cannot be written
//     (held until the maintainer rules on folding links).
//   - RangeBadList: a list that is found but does not name the range's frames.
//
// target itself is not changed. A target without ranges is returned as is.
func ExpandRanges(target *c4m.Manifest, src store.Source) (out *c4m.Manifest, problems []RangeProblem) {
	has := false
	for _, e := range target.Entries {
		if e.IsSequence {
			has = true
			break
		}
	}
	if !has {
		return target, nil
	}

	pathOf := make(map[*c4m.Entry]string, len(target.Entries))
	for p, e := range c4m.EntryPaths(target.Entries) {
		pathOf[e] = p
	}

	ex := c4m.NewSequenceExpander(c4m.SequenceEmbedded)
	if src != nil {
		ex.SetSource(src)
	}
	out = c4m.NewManifest()
	out.Version = target.Version
	out.Base = target.Base
	for _, e := range target.Entries {
		if !e.IsSequence {
			out.AddEntry(e)
			continue
		}
		if e.Mode&os.ModeSymlink != 0 || e.Target != "" {
			problems = append(problems, RangeProblem{Path: pathOf[e], ID: e.C4ID, Reason: RangeLinks})
			out.AddEntry(e)
			continue
		}
		frames, err := ex.ExpandRange(target, e)
		if err != nil {
			p := RangeProblem{Path: pathOf[e], ID: e.C4ID, Reason: RangeNoList}
			if !errors.Is(err, os.ErrNotExist) {
				p.Reason, p.Err = RangeBadList, err
			}
			problems = append(problems, p)
			out.AddEntry(e)
			continue
		}
		for _, f := range frames {
			out.AddEntry(f)
		}
	}
	out.RangeData = target.RangeData
	return out, problems
}

// sourcesOpener reads an object from any of the reconciler's sources.
type sourcesOpener struct{ r *Reconciler }

func (s sourcesOpener) Open(id c4.ID) (io.ReadCloser, error) {
	rc, _, err := s.r.openContent(id)
	return rc, err
}
