// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: BSD-2-Clause

package yamled

import (
	"sort"
	"unicode/utf8"

	"github.com/twskipper/go-yaml-edit/splice"
	"golang.org/x/text/transform"
	"gopkg.in/yaml.v3"
)

// Node returns a selection that spans over a YAML node.
func Node(n *yaml.Node) splice.Selection {
	// IndexEnd incorrectly includes trailing newline when strings are multiline.
	// TODO(mkm): remove hack once upstream is patched
	d := 0
	if n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		d = 1
	}
	return splice.Span(n.Index, n.IndexEnd-d)
}

// T creates a transformer that performs YAML-aware edit operations.
func T(ops ...splice.Op) *Transformer {
	t := &Transformer{}
	qops := make([]splice.Op, len(ops))
	for i := range ops {
		qops[i] = quotedOp(ops[i], t)
	}
	t.t = splice.T(qops...)
	return t
}

// quotedOp transforms a splice.Op into an op that quotes the replacement string according to YAML rules.
func quotedOp(op splice.Op, t *Transformer) splice.Op {
	o := op
	saved := o.Replace
	o.Replace = func(prev string) (string, error) {
		v, err := saved(prev)
		if err != nil {
			return "", err
		}

		line := sort.SearchInts(t.lineStarts, op.Start) - 1
		return quote(v, prev, t.indents[line])
	}
	return o
}

// A Transformer implements golang.org/x/text/transform.Transformer and can be used to perform
// precise in-place edits of yaml nodes in an byte stream.
type Transformer struct {
	t          *splice.Transformer
	linesDone  bool
	indents    []int // indent depth per each line
	lineStarts []int // codepoint position of each line
}

// Transform implements the golang.org/x/text/transform.Transformer interface.
func (t *Transformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	// This implementation is suboptimal as it has to load the whole source in memory
	// in order to create a line indent mapping.
	// It's technically possible to keep track of line indentation information incrementally
	// and thus have a minor optimization. Left as an exercise for the reader.

	// let's load the whole buffer in memory.
	if !atEOF {
		return 0, 0, transform.ErrShortSrc
	}

	// TODO: simplify and only keep current line indentation level, iterate the ops slice in parallel
	// and update the ops with their indent level as the current rune position matches the op's selection
	// starting position.

	if !t.linesDone {
		t.linesDone = true
		rpos := 0
		for i := 0; i < len(src); {
			r, size := utf8.DecodeRune(src[i:])
			i += size
			rpos++
			if t.indents == nil || r == '\n' {
				t.lineStarts = append(t.lineStarts, rpos)
				j := 0
				for ; i < len(src) && src[i] == ' '; i++ {
					j++
					rpos++
				}
				t.indents = append(t.indents, j)
			}
		}
	}

	return t.t.Transform(dst, src, atEOF)
}

// Reset implements the golang.org/x/text/transform.Transformer interface.
func (t *Transformer) Reset() {
	t.t.Reset()
	t.indents = nil
}
