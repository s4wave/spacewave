package gocompiler

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
)

// FormatCodeFile formats ast to go code.
func FormatCodeFile(fset *token.FileSet, pkgCodeFile *ast.File) ([]byte, error) {
	var outBytes bytes.Buffer
	var printerConf printer.Config
	printerConf.Mode |= printer.SourcePos
	err := printer.Fprint(&outBytes, fset, pkgCodeFile)
	return outBytes.Bytes(), err
}
