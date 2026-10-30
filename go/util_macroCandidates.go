package transformer

import (
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// collectMacroCandidates is every name a call or `new` callee can have and still resolve to a macro: looking up the
// symbol of any other callee is wasted checker work (most calls of a project). nil means any name can.
//
// A symbol is a macro through the JSDoc of its declarations (`@typetorchMacro`, `@metadata`), the decorators of its
// declarations, the interfaces its class implements, or rbxts-transform-debug's file. The callee is named after the
// symbol unless an alias renames it, so the set is the names of those declarations and of every alias in the program.
// (A mapped type's property keeps the declarations of the property it maps, under the same name: the checker doesn't
// link them when an `as` clause renames it.)
// It errs on the side of too many names: any `@metadata` counts, whatever it requests, and so does any decorator.
func collectMacroCandidates(program *ts.Program) map[string]bool {
	c := &macroCandidateCollector{names: map[string]bool{}}
	for name := range macroNames {
		c.names["$"+name] = true
	}
	for _, file := range program.GetSourceFiles() {
		c.collectAliases(file.Statements.Nodes)

		text := file.Text()
		c.jsDoc = strings.Contains(text, "@metadata") || strings.Contains(text, "@"+macroTagName)
		structural := strings.Contains(text, "implements") || (!file.IsDeclarationFile && strings.Contains(text, "@"))
		if c.jsDoc || structural {
			c.visit(file.AsNode())
		}
	}
	if c.unknownNames {
		return nil
	}
	return c.names
}

type macroCandidateCollector struct {
	names map[string]bool
	// jsDoc is whether the current file has tags worth looking for.
	jsDoc bool
	// unknownNames is whether a candidate has a computed name.
	unknownNames bool
}

func (c *macroCandidateCollector) collectAliases(statements []*ts.Node) {
	for _, statement := range statements {
		switch statement.Kind {
		case ts.KindImportDeclaration:
			clause := statement.AsImportDeclaration().ImportClause
			if clause == nil {
				continue
			}
			c.addName(clause.Name())
			if bindings := clause.AsImportClause().NamedBindings; bindings != nil {
				if bindings.Kind == ts.KindNamespaceImport {
					c.addName(bindings.Name())
				} else {
					c.collectSpecifiers(bindings.AsNamedImports().Elements.Nodes)
				}
			}
		case ts.KindImportEqualsDeclaration:
			c.addName(statement.Name())
		case ts.KindExportDeclaration:
			if clause := statement.AsExportDeclaration().ExportClause; clause != nil {
				if clause.Kind == ts.KindNamespaceExport {
					c.addName(clause.Name())
				} else {
					c.collectSpecifiers(clause.AsNamedExports().Elements.Nodes)
				}
			}
		case ts.KindModuleDeclaration:
			body := statement.AsModuleDeclaration().Body
			if body == nil {
				continue
			}
			if body.Kind == ts.KindModuleBlock {
				c.collectAliases(body.AsModuleBlock().Statements.Nodes)
			} else {
				c.collectAliases([]*ts.Node{body})
			}
		}
	}
}

// collectSpecifiers: only `{ a as b }` renames.
func (c *macroCandidateCollector) collectSpecifiers(specifiers []*ts.Node) {
	for _, specifier := range specifiers {
		var propertyName *ts.Node
		if specifier.Kind == ts.KindImportSpecifier {
			propertyName = specifier.AsImportSpecifier().PropertyName
		} else {
			propertyName = specifier.AsExportSpecifier().PropertyName
		}
		if propertyName != nil {
			c.addName(specifier.Name())
		}
	}
}

func (c *macroCandidateCollector) visit(node *ts.Node) bool {
	if c.jsDoc && len(getJSDocTags(node)) > 0 {
		c.addName(node.Name())
	}
	if ts.CanHaveDecorators(node) && len(ts.GetDecorators(node)) > 0 {
		c.addName(node.Name())
	}
	if ts.IsClassLike(node) && len(ts.GetEffectiveImplementsTypeNodes(node)) > 0 {
		c.addName(node.Name())
		for _, member := range node.Members() {
			c.addName(member.Name())
		}
	}
	return node.ForEachChild(c.visit)
}

func (c *macroCandidateCollector) addName(name *ts.Node) {
	if name == nil {
		return
	}
	switch name.Kind {
	case ts.KindObjectBindingPattern, ts.KindArrayBindingPattern:
		for _, element := range name.Elements() {
			if element.Kind == ts.KindBindingElement {
				c.addName(element.Name())
			}
		}
	default:
		if text, ok := ts.GetPropertyNameForPropertyNameNode(name); ok {
			c.names[text] = true
		} else {
			c.unknownNames = true
		}
	}
}

// mayBeMacro is whether a call or `new` of callee has to look up the callee's symbol. Only a plain name or a property
// access can be ruled out: `super` resolves to the base class, and a parenthesized callee or an element access isn't
// worth the risk.
func (s *TransformState) mayBeMacro(callee *ts.Node) bool {
	if s.macroCandidates == nil {
		return true
	}
	if callee.Kind == ts.KindPropertyAccessExpression {
		callee = callee.Name()
	}
	if callee.Kind != ts.KindIdentifier {
		return true
	}
	return s.macroCandidates[callee.Text()]
}
