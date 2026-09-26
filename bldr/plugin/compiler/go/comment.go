//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/pkg/errors"
)

// TrimCommentArgs trims a comment tag prefix from a string.
//
// Returns if the string had the comment tag prefix.
func TrimCommentArgs(tag, value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "//")
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), tag+" ") {
		value = strings.TrimSpace(value[len(tag)+1:])
		return value, true
	}
	return value, false
}

// FindTagComments searches for comments associated with variable declarations (`var`)
// that have the given tag prefix. It operates purely on the Abstract Syntax Tree (AST)
// and does not use resolved type information.
//
// Returns a map of packages -> variable names -> parsed result (type T).
// The checkParseComments callback receives the comment lines and the AST node (*ast.ValueSpec)
// for the variable declaration. It should return the parsed result, a boolean indicating
// if the tag was found, and any error. Returning false for the boolean skips the comment.
func FindTagComments[T any](
	tag string,
	fset *token.FileSet,
	codeFiles map[string][]*ast.File,
	checkParseComments func(values []string, spec *ast.ValueSpec) (T, bool, error),
) (map[string](map[string]T), error) {
	packagesMap := make(map[string](map[string]T))
	getPackageMap := func(pkg string) map[string]T {
		m := packagesMap[pkg]
		if m == nil {
			m = make(map[string]T)
		}
		packagesMap[pkg] = m
		return m
	}

	for pkgImportPath, pkgCodeFile := range codeFiles {
		for _, codeFile := range pkgCodeFile {
			// Avoid constructing a comment map for files without this annotation.
			hasTag := false
			for _, group := range codeFile.Comments {
				for _, comment := range group.List {
					if _, found := TrimCommentArgs(tag, comment.Text); found {
						hasTag = true
						break
					}
				}
				if hasTag {
					break
				}
			}
			if !hasTag {
				continue
			}

			// Preserve Go's comment association for declarations and grouped specs.
			cmap := ast.NewCommentMap(fset, codeFile, codeFile.Comments)
			for nod, comments := range cmap {
				for _, comment := range comments {
					posErr := func(err error) error {
						pos := fset.Position(nod.Pos()).String()
						return errors.Wrap(err, pos)
					}
					var commentPts []string
					for _, commentElem := range comment.List {
						commentTxt := strings.TrimPrefix(commentElem.Text, "//")
						if len(commentTxt) != 0 {
							commentPts = append(commentPts, commentTxt)
						}
					}
					if len(commentPts) != 0 {
						decl, declOk := nod.(*ast.GenDecl)
						if !declOk || len(decl.Specs) == 0 {
							continue
						}
						pkgMap := getPackageMap(pkgImportPath)
						for _, spec := range decl.Specs {
							valueSpec, ok := spec.(*ast.ValueSpec)
							if !ok || len(valueSpec.Names) == 0 {
								continue
							}
							args, hasTag, err := checkParseComments(commentPts, valueSpec)
							if err != nil {
								return nil, posErr(err)
							}
							if !hasTag {
								continue
							}
							for _, name := range valueSpec.Names {
								if name != nil && len(name.Name) != 0 {
									pkgMap[name.Name] = args
								}
							}
						}
					}
				}
			}
		}
	}

	// Remove packages that ended up with no tagged variables
	for pkgImportPath, vars := range packagesMap {
		if len(vars) == 0 {
			delete(packagesMap, pkgImportPath)
		}
	}

	return packagesMap, nil
}

// FindTagCommentsWithTypes resolves only tagged declaration candidates.
// Scope lookup preserves inferred types, aliases, and the omission of blank identifiers.
func FindTagCommentsWithTypes[T any](
	tag string,
	analysis *Analysis,
	codeFiles map[string][]*ast.File,
	processComments func(values []string, obj types.Object) (T, bool, error),
) (map[string]map[string]T, error) {
	// Collect comment-bearing declarations before consulting the narrow type universe.
	type candidate struct {
		// values contains the associated comment lines.
		values []string
		// pos locates errors at the declaration.
		pos token.Pos
	}
	candidates, err := FindTagComments(tag, analysis.fset, codeFiles,
		func(values []string, spec *ast.ValueSpec) (candidate, bool, error) {
			for _, value := range values {
				if _, found := TrimCommentArgs(tag, value); found {
					return candidate{values: values, pos: spec.Pos()}, true, nil
				}
			}
			return candidate{}, false, nil
		})
	if err != nil {
		return nil, err
	}

	// Invoke the typed parser only for objects with matching annotation comments.
	result := make(map[string]map[string]T)
	for pkgPath, vars := range candidates {
		pkg := analysis.typedPackages[pkgPath]
		for name, candidate := range vars {
			obj := pkg.Scope().Lookup(name)
			if obj == nil {
				continue
			}
			value, found, err := processComments(candidate.values, obj)
			if err != nil {
				return nil, errors.Wrap(err, analysis.fset.Position(candidate.pos).String())
			}
			if !found {
				continue
			}
			if result[pkgPath] == nil {
				result[pkgPath] = make(map[string]T)
			}
			result[pkgPath][name] = value
		}
	}
	return result, nil
}

// CombineShellComments searches for & strips the given tag from the list of comments.
// Parses each comment as shell args (splits with shell quote rules).
// Returns the merged list of shell args.
// Returns if the tag was found in any of the comments.
// Ignores any comments without the prefix.
// This allows for multi-line shell-style arguments in comments to be combined.
func CombineShellComments(tag string, comments []string) ([]string, bool, error) {
	var tagFound bool
	var args []string
	for _, cmt := range comments {
		cmt, found := TrimCommentArgs(tag, cmt)
		if found {
			tagFound = true
			sargs, err := shellquote.Split(cmt)
			args = append(args, sargs...)
			if err != nil {
				return args, true, err
			}
		}
	}
	return args, tagFound, nil
}
