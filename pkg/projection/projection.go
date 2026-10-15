// Package projection trims structured values down to selected fields so
// automations and LLM/agent flows stop paying tokens for payloads they
// never read.
//
// It works on any JSON-marshalable value — every unified model in the
// parent SDK (ChangeRequest, Issue, CommitStatus, ...) carries json tags
// and qualifies. Projection is applied AFTER JSON normalization, so
// paths use the marshaled (snake_case) field names, nested pointers are
// transparent, and slices are traversed element-wise.
package projection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	oekiterrors "github.com/oekit/pkg/errors"
)

// keepTree marks which parts of a document survive projection. A true
// leaf keeps the whole subtree at that path; a nested keepTree keeps
// only the marked children. keepAll is the "*" wildcard.
type keepTree map[string]any

const keepAll = "*"

// Project marshals v, then prunes the document down to fields. Paths are
// dotted ("head.ref"); a path segment applied to an array projects each
// element; the "*" field means "everything" and returns the unpruned
// document.
//
// An empty field list is an error: the whole point of the helper is to
// make payloads small, and silently returning the full document would be
// the exact failure mode it exists to prevent.
func Project(v any, fields ...string) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, oekiterrors.Wrap(err, "projection: marshal")
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, oekiterrors.Wrap(err, "projection: document is not a JSON object")
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("projection: no fields selected (pass at least one field, or \"*\" for the full document)")
	}

	keep := keepTree{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == keepAll {
			return doc, nil
		}
		segs := strings.Split(f, ".")
		for _, seg := range segs {
			switch seg {
			case "":
				return nil, fmt.Errorf("projection: invalid field path %q", f)
			case keepAll:
				// A segment-level wildcard would otherwise be treated as a
				// literal "*" key and silently drop the field — a trap for
				// LLM-generated field lists. Only the bare "*" field means
				// "everything".
				return nil, fmt.Errorf("projection: wildcard is only supported as the standalone field %q, not inside path %q", keepAll, f)
			}
		}
		keep.add(segs)
	}

	return prune(doc, keep).(map[string]any), nil
}

// ProjectList projects every element of items with the same field list,
// preserving order — the list-shape helper for "show me 50 PRs as
// number/title/author".
func ProjectList[T any](items []T, fields ...string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		m, err := Project(item, fields...)
		if err != nil {
			return nil, oekiterrors.Wrap(err, "projection: project item", "index", i)
		}
		out = append(out, m)
	}
	return out, nil
}

// add folds one dotted path into the tree. Later paths never shrink
// earlier ones: "head" followed by "head.ref" keeps all of head.
func (t keepTree) add(segs []string) {
	cur := t
	for i, seg := range segs {
		if i == len(segs)-1 {
			if _, exists := cur[seg]; !exists {
				cur[seg] = true
			}
			return
		}
		next, ok := cur[seg].(keepTree)
		if !ok {
			if _, isLeaf := cur[seg]; isLeaf {
				return // already kept whole subtree
			}
			next = keepTree{}
			cur[seg] = next
		}
		cur = next
	}
}

// prune returns a copy of node restricted to keep: maps keep only
// marked keys, arrays keep every element (each pruned with the same
// subtree), leaves pass through untouched.
func prune(node any, keep keepTree) any {
	switch cur := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(keep))
		for key, sub := range keep {
			if sub == true {
				if v, exists := cur[key]; exists {
					out[key] = v
				}
				continue
			}
			if v, exists := cur[key]; exists {
				out[key] = prune(v, sub.(keepTree))
			}
		}
		return out
	case []any:
		out := make([]any, len(cur))
		for i, elem := range cur {
			out[i] = prune(elem, keep)
		}
		return out
	default:
		return node
	}
}
