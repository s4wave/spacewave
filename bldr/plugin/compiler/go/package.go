//go:build !js

package bldr_plugin_compiler_go

import (
	"go/ast"
	"go/types"
)

// Package contains discovery records without retaining dependency syntax or types.
type Package struct {
	// Path is the resolved import path.
	Path string
	// Name is the declared package name used for generated import aliases.
	Name string
	// SourceFiles are the target-selected compiled Go files to watch.
	SourceFiles []string
	// EmbedFiles are the files the package embeds with go:embed.
	EmbedFiles []string
	// Factory describes NewFactory, or is nil when it is absent.
	Factory *Constructor
	// CliCommands describes NewCliCommands, or is nil when it is absent.
	CliCommands *Constructor
}

// discoverConstructors records ordinary functions and requests types for other objects.
func (p *Package) discoverConstructors(files []*ast.File) bool {
	// Limit discovery to package-scope constructors, excluding methods.
	needsTypes := false
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv != nil {
					continue
				}

				// Match constructor names without treating methods as package declarations.
				var target **Constructor
				switch decl.Name.Name {
				case "NewFactory":
					target = &p.Factory
				case "NewCliCommands":
					target = &p.CliCommands
				default:
					continue
				}

				// Grouped identifiers each occupy a parameter slot.
				shape := &Constructor{IsFunction: true}
				for _, param := range decl.Type.Params.List {
					shape.Parameters += max(1, len(param.Names))
					if _, variadic := param.Type.(*ast.Ellipsis); variadic {
						shape.Variadic = true
					}
				}
				*target = shape
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					var names []*ast.Ident
					switch spec := spec.(type) {
					case *ast.ValueSpec:
						names = spec.Names
					case *ast.TypeSpec:
						names = []*ast.Ident{spec.Name}
					}
					for _, name := range names {
						if name.Name == "NewFactory" || name.Name == "NewCliCommands" {
							needsTypes = true
						}
					}
				}
			}
		}
	}
	return needsTypes
}

// resolveConstructors preserves go/types signature classification for aliases and values.
func (p *Package) resolveConstructors(pkg *types.Package) {
	for name, target := range map[string]**Constructor{
		"NewFactory":     &p.Factory,
		"NewCliCommands": &p.CliCommands,
	} {
		// Syntax already settles ordinary functions without another signature inspection.
		if *target != nil {
			continue
		}

		// Resolve only package-scope objects whose callable shape was ambiguous.
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		shape := &Constructor{}
		if sig, ok := obj.Type().(*types.Signature); ok {
			shape.IsFunction = true
			shape.Parameters = sig.Params().Len()
			shape.Variadic = sig.Variadic()
		}
		*target = shape
	}
}
