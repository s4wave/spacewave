// Package binpkg reads Gentoo binary packages: the Packages index of a binary
// package directory, the runtime closure of a set of packages, and the files
// of each GPKG archive split into runtime and dev layers.
package binpkg

import (
	"bufio"
	"io"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

// Package is one binary package entry of a Packages index.
type Package struct {
	// CPV is the category, name and version, such as app-editors/nano-9.2.
	CPV string
	// Name is the category and name, such as app-editors/nano.
	Name string
	// Version is the version and revision, such as 9.2-r1.
	Version string
	// License is the license expression.
	License string
	// RDepend is the runtime dependency string with USE conditionals resolved.
	RDepend string
	// Path is the binary package path relative to the package directory.
	Path string
}

// versionSuffix matches the version and revision at the end of a CPV.
var versionSuffix = regexp.MustCompile(`-[0-9][^-/]*(-r[0-9]+)?$`)

// ReadIndex parses a Packages index into its package entries by name. The
// header stanza is skipped. Later entries for the same name replace earlier
// ones, so the result holds the newest build of each package.
func ReadIndex(r io.Reader) (map[string]*Package, error) {
	// Collect each stanza's fields and record its package at the blank line
	// that ends it.
	pkgs := make(map[string]*Package)
	var cur map[string]string
	flush := func() {
		cpv := cur["CPV"]
		if cpv == "" {
			return
		}
		name := versionSuffix.ReplaceAllString(cpv, "")
		pkgs[name] = &Package{
			CPV:     cpv,
			Name:    name,
			Version: strings.TrimPrefix(cpv, name+"-"),
			License: cur["LICENSE"],
			RDepend: cur["RDEPEND"],
			Path:    cur["PATH"],
		}
	}

	// Read the stanzas, each a run of "KEY: value" lines.
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			cur = nil
			continue
		}
		if cur == nil {
			cur = make(map[string]string)
		}
		if key, value, ok := strings.Cut(line, ": "); ok {
			cur[key] = value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, errors.Wrap(err, "read index")
	}
	flush()
	return pkgs, nil
}

// AtomName returns the category and name of a dependency atom, dropping its
// version operator, version, slot and USE dependencies.
func AtomName(atom string) string {
	// Drop the USE dependencies and the slot.
	if i := strings.IndexByte(atom, '['); i >= 0 {
		atom = atom[:i]
	}
	if i := strings.IndexByte(atom, ':'); i >= 0 {
		atom = atom[:i]
	}

	// Drop the version operator and the version it governs.
	trimmed := strings.TrimLeft(atom, "<>=~")
	if trimmed != atom {
		trimmed = strings.TrimSuffix(trimmed, "*")
		trimmed = versionSuffix.ReplaceAllString(trimmed, "")
	}
	return trimmed
}

// Depends returns the names of the packages pkg needs at runtime that pkgs
// holds, in the order its dependency string lists them. In an any-of group
// the first alternative present in pkgs is chosen. Blockers and provided
// packages are dropped.
func Depends(pkgs map[string]*Package, provided map[string]bool, pkg *Package) []string {
	// Walk the tokens, choosing one alternative inside each any-of group.
	var out []string
	toks := strings.Fields(pkg.RDepend)
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		switch {
		case tok == "||":
			// Choose the first present atom of the group and skip the rest.
			chosen := ""
			depth := 0
			for i++; i < len(toks); i++ {
				switch t := toks[i]; t {
				case "(":
					depth++
				case ")":
					depth--
				default:
					if name := AtomName(t); chosen == "" && pkgs[name] != nil {
						chosen = name
					}
				}
				if depth == 0 {
					break
				}
			}
			if chosen != "" {
				out = append(out, chosen)
			}
		case tok == "(" || tok == ")" || strings.HasPrefix(tok, "!"):
		default:
			out = append(out, AtomName(tok))
		}
	}
	return removeProvided(out, provided)
}

// removeProvided drops the provided names from names.
func removeProvided(names []string, provided map[string]bool) []string {
	out := names[:0]
	for _, name := range names {
		if !provided[name] {
			out = append(out, name)
		}
	}
	return out
}

// Resolve returns the runtime closure of the named packages, each package
// after the packages it depends on. A provided package, one the profile's
// package.provided lists, is satisfied without installing.
func Resolve(pkgs map[string]*Package, provided map[string]bool, names []string) ([]*Package, error) {
	// Visit each package depth first and emit it after its dependencies. A
	// package is marked 1 while its dependencies are visited, so a dependency
	// cycle ends there, and 2 once emitted.
	var out []*Package
	state := make(map[string]int)
	var visit func(name string) error
	visit = func(name string) error {
		// Skip a provided package, or one already visited or on the
		// current path.
		if provided[name] || state[name] != 0 {
			return nil
		}

		// Visit the dependencies, then emit the package.
		pkg := pkgs[name]
		if pkg == nil {
			return errors.Errorf("package not in index: %s", name)
		}
		state[name] = 1
		for _, dep := range Depends(pkgs, provided, pkg) {
			if err := visit(dep); err != nil {
				return errors.Wrap(err, pkg.CPV)
			}
		}
		state[name] = 2
		out = append(out, pkg)
		return nil
	}

	// Resolve every requested package.
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return out, nil
}
