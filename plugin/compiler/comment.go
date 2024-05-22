//go:build !js

package bldr_plugin_compiler

import (
	"go/ast"
	"go/token"
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

// FindTagComments searches for comments with the given tag & parses them.
//
// Returns a map of packages -> variable names -> variable args.
// checkParseComment should return nil, false, nil if the comment doesn't have the tag prefix.
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
	return packagesMap, nil
}

// CombineShellComments searches for & strips the given tag from the list of comments.
// Parses each comment as shell args (splits with shell quote rules).
// Returns the merged list of shell args.
// Returns if the tag was found in any of the comments.
// Ignores any comments without the prefix.
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
