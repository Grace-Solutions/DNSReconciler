// Package expansion implements the variable expansion engine (§19).
package expansion

import (
	"fmt"
	"regexp"
	"strings"
)

// varPattern matches ${VAR_NAME} placeholders. The character class allows
// uppercase variables (HOSTNAME, NODE_ID), container label lookups with
// the LABEL: prefix (e.g. ${LABEL:dns.hostname}), and dot-separated keys.
var varPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_:./-]+)\}`)

// Context holds all available variable values for expansion.
// Keys must match the spec's §19.1 names (without ${}).
type Context map[string]string

// Result holds the expanded string and any unresolved variables.
type Result struct {
	Value      string
	Unresolved []string
}

// Expand replaces all ${VAR} placeholders in the input using ctx.
// Per §19.2: unresolved variables are reported but the expansion still
// returns the partially-expanded string so the caller can decide how to handle it.
func Expand(input string, ctx Context) Result {
	var unresolved []string
	expanded := varPattern.ReplaceAllStringFunc(input, func(match string) string {
		sub := varPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := sub[1]
		if val, ok := ctx[key]; ok {
			return val
		}
		unresolved = append(unresolved, key)
		return match // leave unresolved placeholders intact
	})
	return Result{Value: expanded, Unresolved: unresolved}
}

// MustExpand is like Expand but returns an error if any variable is unresolved.
// This enforces §19.2: fail the specific record on unresolved variables.
func MustExpand(input string, ctx Context) (string, error) {
	r := Expand(input, ctx)
	if len(r.Unresolved) > 0 {
		return r.Value, fmt.Errorf("unresolved variables: %s", strings.Join(r.Unresolved, ", "))
	}
	return r.Value, nil
}

// ExpandAll applies Expand to every string in the slice.
func ExpandAll(inputs []string, ctx Context) ([]string, []string) {
	var allUnresolved []string
	out := make([]string, len(inputs))
	for i, s := range inputs {
		r := Expand(s, ctx)
		out[i] = r.Value
		allUnresolved = append(allUnresolved, r.Unresolved...)
	}
	return out, allUnresolved
}

// ExpandMap applies Expand to every value in the map.
func ExpandMap(input map[string]string, ctx Context) (map[string]string, []string) {
	var allUnresolved []string
	out := make(map[string]string, len(input))
	for k, v := range input {
		r := Expand(v, ctx)
		out[k] = r.Value
		allUnresolved = append(allUnresolved, r.Unresolved...)
	}
	return out, allUnresolved
}

