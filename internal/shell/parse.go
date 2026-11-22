package shell

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func parseScript(line, name string) (*syntax.File, error) {
	file, err := syntax.NewParser().Parse(strings.NewReader(line), name)
	if err != nil {
		return nil, err
	}
	if err := validateSyntax(file); err != nil {
		return nil, err
	}
	return file, nil
}

// These constructs use host operations inside mvdan/sh, before any virtual
// handler can run. Validate code introduced through eval, alias, and trap too.
func validateSyntax(node syntax.Node) error {
	var err error
	syntax.Walk(node, func(node syntax.Node) bool {
		if err != nil {
			return false
		}
		switch node := node.(type) {
		case *syntax.ProcSubst:
			err = fmt.Errorf("%s: process substitution is unavailable in the virtual shell", node.Pos())
		case *syntax.UnaryTest:
			if node.Op == syntax.TsUsrOwn || node.Op == syntax.TsGrpOwn {
				err = fmt.Errorf("%s: use test for virtual file ownership checks", node.Pos())
			}
		case *syntax.Word:
			err = validateTilde(node)
		case *syntax.DeclClause:
			err = validateDeclaration(node)
		}
		return err == nil
	})
	return err
}

// The interpreter expands computed declaration arguments twice, turning the
// part after '=' into a fresh, unquoted literal on the second pass. Restrict
// that form to static arguments so it cannot introduce a named home lookup.
func validateDeclaration(decl *syntax.DeclClause) error {
	for _, arg := range decl.Args {
		if arg.Name != nil {
			continue
		}
		static := true
		syntax.Walk(arg.Value, func(node syntax.Node) bool {
			switch node := node.(type) {
			case *syntax.Lit:
				static = !strings.ContainsAny(node.Value, "~*?[")
				return static
			case *syntax.Word, *syntax.SglQuoted, *syntax.DblQuoted, nil:
				return true
			default:
				static = false
				return false
			}
		})
		if !static {
			return fmt.Errorf("%s: use a literal variable name in %s assignments", arg.Pos(), decl.Variant.Value)
		}
		fields, err := expand.Fields(nil, arg.Value)
		if err != nil {
			return err
		}
		for _, field := range fields {
			if _, value, assignment := strings.Cut(field, "="); assignment && namedHome(value) {
				return fmt.Errorf("%s: named home expansion is unavailable in the virtual shell", arg.Pos())
			}
		}
	}
	return nil
}

func validateTilde(word *syntax.Word) error {
	// Brace expansion can put a tilde at the start of a word. Check the
	// expanded alternatives before interpolation can look up a host username.
	clone := *word
	if !syntax.SplitBraces(&clone) {
		return checkTilde(word)
	}
	for alternative, err := range expand.BracesSeq(nil, &clone) {
		if err != nil {
			return err
		}
		if err := checkTilde(alternative); err != nil {
			return err
		}
	}
	return nil
}

func checkTilde(word *syntax.Word) error {
	if len(word.Parts) == 0 {
		return nil
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return nil
	}
	if namedHome(lit.Value) {
		return fmt.Errorf("%s: named home expansion is unavailable in the virtual shell", word.Pos())
	}
	return nil
}

func namedHome(value string) bool {
	tail, ok := strings.CutPrefix(value, "~")
	return ok && tail != "" && tail[0] != '/'
}

func literalCall(args ...string) *syntax.CallExpr {
	call := &syntax.CallExpr{}
	for _, arg := range args {
		call.Args = append(call.Args, &syntax.Word{Parts: []syntax.WordPart{&syntax.SglQuoted{Value: arg}}})
	}
	return call
}
