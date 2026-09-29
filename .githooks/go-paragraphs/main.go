// Command go-paragraphs checks that staged Go changes keep function bodies in
// code paragraphs.
//
// A function body of five or more top-level statements is a sequence of
// actions. Each action is a paragraph that begins with a purpose comment and
// follows exactly one blank line, and no paragraph exceeds eight statements. The
// check reads the staged blobs and reports only functions with a changed line of
// their own, so touching a legacy function brings that function up to shape
// without demanding a repository-wide cleanup. Function literals are checked
// separately from the function that declares them. Generated files are omitted.
//
// It exits with status 1 when it reports a violation.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const (
	// minStatements is the top-level statement count at which a function body
	// is a sequence of actions instead of one condensed action.
	minStatements = 5
	// maxStatements is the largest paragraph that can be one action.
	maxStatements = 8
)

// hunkHeader matches the new-file range of a unified diff hunk.
var hunkHeader = regexp.MustCompile(`^@@ -\S+ \+(\d+)(?:,(\d+))? @@`)

func main() {
	// List the staged Go files that a commit adds or modifies.
	names, err := git("diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR", "--", "*.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// Check each file's staged contents against its changed lines.
	failed := false
	for name := range strings.SplitSeq(names, "\x00") {
		if name == "" {
			continue
		}
		problems, err := checkFile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "%s:%s\n", name, problem)
			failed = true
		}
	}
	if failed {
		fmt.Fprintln(os.Stderr, "Go function bodies you changed need code paragraphs: a purpose comment above each action and one blank line between actions (see AGENTS.md).")
		os.Exit(1)
	}
}

// git runs a git command and returns its standard output.
func git(args ...string) (string, error) {
	// Run git with its output captured.
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}

// checkFile returns the line-prefixed violations in the changed functions of
// one staged file.
func checkFile(name string) ([]string, error) {
	// Read the staged blob and the lines the commit changes in it.
	blob, err := git("show", ":"+name)
	if err != nil {
		return nil, err
	}
	changed, err := changedLines(name)
	if err != nil {
		return nil, err
	}

	// A file that does not parse is left to the compiler.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, blob, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil || ast.IsGenerated(file) {
		return nil, nil
	}

	// Index comment groups by the line they end on to find each purpose comment.
	endingAt := make(map[int]*ast.CommentGroup, len(file.Comments))
	for _, group := range file.Comments {
		endingAt[fset.Position(group.End()).Line] = group
	}

	// Check every function body that has a changed line of its own.
	var bodies []*ast.BlockStmt
	ast.Inspect(file, func(node ast.Node) bool {
		switch fn := node.(type) {
		case *ast.FuncDecl:
			bodies = append(bodies, fn.Body)
		case *ast.FuncLit:
			bodies = append(bodies, fn.Body)
		}
		return true
	})
	var problems []string
	for _, body := range bodies {
		if body != nil && ownsChange(fset, body, bodies, changed) {
			problems = append(problems, checkBody(fset, endingAt, body)...)
		}
	}
	return problems, nil
}

// changedLines returns the new-file line numbers a staged diff adds or modifies.
func changedLines(name string) (map[int]bool, error) {
	// Read the staged diff without context lines.
	diff, err := git("diff", "--cached", "-U0", "--no-color", "--", name)
	if err != nil {
		return nil, err
	}

	// Mark the new-file range of every hunk.
	changed := make(map[int]bool)
	for line := range strings.SplitSeq(diff, "\n") {
		match := hunkHeader.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		first, _ := strconv.Atoi(match[1])
		count := 1
		if match[2] != "" {
			count, _ = strconv.Atoi(match[2])
		}
		for n := first; n < first+count; n++ {
			changed[n] = true
		}
	}
	return changed, nil
}

// ownsChange reports whether a changed line lies in body and outside every
// function literal nested in it.
func ownsChange(fset *token.FileSet, body *ast.BlockStmt, bodies []*ast.BlockStmt, changed map[int]bool) bool {
	first, last := fset.Position(body.Pos()).Line, fset.Position(body.End()).Line
	for line := first; line <= last; line++ {
		if !changed[line] {
			continue
		}
		nested := false
		for _, other := range bodies {
			if other == nil || other == body {
				continue
			}
			from, to := fset.Position(other.Pos()).Line, fset.Position(other.End()).Line
			if from >= first && to <= last && from <= line && line <= to {
				nested = true
				break
			}
		}
		if !nested {
			return true
		}
	}
	return false
}

// checkBody returns the paragraph violations of one function body.
func checkBody(fset *token.FileSet, endingAt map[int]*ast.CommentGroup, body *ast.BlockStmt) []string {
	// A condensed function is one paragraph under its declaration comment.
	if len(body.List) < minStatements {
		return nil
	}

	// Walk the statements, closing a paragraph at each blank line.
	var problems []string
	var paragraph []ast.Stmt
	var purpose *ast.CommentGroup
	prevEnd := fset.Position(body.Lbrace).Line
	flush := func() {
		problems = append(problems, checkParagraph(fset, paragraph, purpose)...)
		paragraph = nil
	}
	for _, stmt := range body.List {
		// A purpose comment ends directly above its statement and starts below the previous one.
		stmtLine := fset.Position(stmt.Pos()).Line
		leading := endingAt[stmtLine-1]
		if leading != nil && fset.Position(leading.Pos()).Line <= prevEnd {
			leading = nil
		}
		unitLine := stmtLine
		if leading != nil {
			unitLine = fset.Position(leading.Pos()).Line
		}

		// A blank line before the unit starts a paragraph; a purpose comment without one is misplaced.
		if unitLine > prevEnd+1 || len(paragraph) == 0 {
			flush()
			purpose = leading
		} else if leading != nil && !isInlineMarker(leading) {
			problems = append(problems, fmt.Sprintf("%d: purpose comment must follow a blank line that ends the previous paragraph", fset.Position(leading.Pos()).Line))
		}
		paragraph = append(paragraph, stmt)
		prevEnd = fset.Position(stmt.End()).Line
	}
	flush()
	return problems
}

// checkParagraph returns the violations of a paragraph with no purpose comment
// or too many statements.
func checkParagraph(fset *token.FileSet, paragraph []ast.Stmt, purpose *ast.CommentGroup) []string {
	// An empty paragraph has nothing to check.
	if len(paragraph) == 0 {
		return nil
	}

	// Locate the paragraph by its first statement.
	line := fset.Position(paragraph[0].Pos()).Line

	// A lone return completes the action before it and needs no outline entry.
	var problems []string
	_, isReturn := paragraph[0].(*ast.ReturnStmt)
	if purpose == nil && (len(paragraph) != 1 || !isReturn) {
		problems = append(problems, fmt.Sprintf("%d: code paragraph needs a purpose comment directly above it", line))
	}
	if len(paragraph) > maxStatements {
		problems = append(problems, fmt.Sprintf("%d: code paragraph has %d statements; split it into separate commented actions", line, len(paragraph)))
	}
	return problems
}

// isInlineMarker reports whether a comment is a TODO or XXX marker, which
// annotates a statement instead of introducing a paragraph.
func isInlineMarker(group *ast.CommentGroup) bool {
	text := strings.TrimSpace(group.Text())
	return strings.HasPrefix(text, "TODO:") || strings.HasPrefix(text, "XXX:")
}
