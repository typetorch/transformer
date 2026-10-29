// Port of src/classes/nodeMetadata.ts.

package transformer

import (
	"regexp"
	"strings"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

// nodeMetadata is what a node asks for with `@metadata` JSDoc tags, on itself, on its decorators' declarations, or
// on the interfaces it implements.
type nodeMetadata struct {
	set     map[string]bool
	symbols map[string][]*ts.Symbol
	types   map[string][]*ts.Type
	// trace maps a name (string), *ts.Symbol or *ts.Type to the node that requested it.
	trace map[any]*ts.Node
}

func nodeMetadataFromSymbol(state *TransformState, symbol *ts.Symbol) *nodeMetadata {
	if symbol.ValueDeclaration != nil {
		return newNodeMetadata(state, symbol.ValueDeclaration)
	}
	return nil
}

func newNodeMetadata(state *TransformState, node *ts.Node) *nodeMetadata {
	metadata := &nodeMetadata{
		set:     map[string]bool{},
		symbols: map[string][]*ts.Symbol{},
		types:   map[string][]*ts.Type{},
		trace:   map[any]*ts.Node{},
	}
	metadata.parse(state, node)
	return metadata
}

var firstWhitespaceRegex = regexp.MustCompile(`\s+`)

func (m *nodeMetadata) parseText(text string, node *ts.Node) {
	// JavaScript's `.replace(/\s+/, " ")` only replaces the first run.
	trimmed := strings.TrimSpace(text)
	if loc := firstWhitespaceRegex.FindStringIndex(trimmed); loc != nil {
		trimmed = trimmed[:loc[0]] + " " + trimmed[loc[1]:]
	}
	for _, name := range strings.Split(trimmed, " ") {
		m.set[name] = true
		m.trace[name] = node
	}
}

func (m *nodeMetadata) parseMetadata(state *TransformState, tag *ts.Node) {
	comments := tag.CommentList()
	if comments == nil {
		return
	}
	// TypeScript's `tag.comment` is a plain string when it holds no links.
	plain := true
	for _, comment := range comments.Nodes {
		plain = plain && comment.Kind == ts.KindJSDocText
	}
	if plain {
		m.parseText(ts.GetTextOfJSDocComment(comments), tag)
		return
	}
	for _, comment := range comments.Nodes {
		if isJSDocLinkLike(comment) {
			name := comment.Name()
			if name == nil {
				continue
			}

			symbol := state.getSymbol(name)
			if symbol == nil {
				continue
			}

			var t *ts.Type
			if symbol.Flags&ts.SymbolFlagsTypeAlias != 0 {
				t = state.typeChecker.GetDeclaredTypeOfSymbol(symbol)
			} else {
				t = state.typeChecker.GetTypeAtLocation(name)
			}

			text := comment.Text()
			m.symbols[text] = append(m.symbols[text], symbol)
			m.types[text] = append(m.types[text], t)
			m.trace[symbol] = comment
			m.trace[t] = comment
		} else {
			m.parseText(comment.Text(), comment)
		}
	}
}

func (m *nodeMetadata) parse(state *TransformState, node *ts.Node) {
	for _, tag := range getJSDocTags(node) {
		if tag.TagName().Text() == "metadata" {
			m.parseMetadata(state, tag)
		}
	}

	if ts.CanHaveDecorators(node) {
		for _, decorator := range ts.GetDecorators(node) {
			expression := decorator.Expression()
			target := expression
			if expression.Kind == ts.KindCallExpression {
				target = expression.Expression()
			}
			symbol := state.getSymbol(target)
			if symbol == nil || symbol.Declarations == nil {
				continue
			}

			for _, declaration := range symbol.Declarations {
				m.parse(state, declaration)
			}
		}
	}

	if ts.IsClassElement(node) && node.Name() != nil {
		// Interfaces are able to request metadata for their own property/methods.
		name, ok := ts.GetPropertyNameForPropertyNameNode(node.Name())
		if ok && node.Parent != nil && ts.IsClassLike(node.Parent) {
			for _, implement := range ts.GetEffectiveImplementsTypeNodes(node.Parent) {
				symbol := state.getSymbol(heritageExpression(implement))
				if symbol == nil || symbol.Members == nil {
					continue
				}
				if member := symbol.Members[name]; member != nil {
					for _, declaration := range member.Declarations {
						m.parse(state, declaration)
					}
				}
			}
		}
	} else if ts.IsClassLike(node) {
		// Interfaces are able to request metadata for the object it is implemented on.
		for _, implement := range ts.GetEffectiveImplementsTypeNodes(node) {
			symbol := state.getSymbol(heritageExpression(implement))
			if symbol != nil && len(symbol.Declarations) > 0 {
				m.parse(state, symbol.Declarations[0])
			}
		}
	}
}

func (m *nodeMetadata) isRequested(metadata string) bool {
	if m.set["~"+metadata] {
		return false
	}
	return m.set[metadata] || m.set["*"]
}

func (m *nodeMetadata) getSymbol(key string) []*ts.Symbol { return m.symbols[key] }

func (m *nodeMetadata) getType(key string) []*ts.Type { return m.types[key] }

func (m *nodeMetadata) getTrace(name any) *ts.Node { return m.trace[name] }

// getJSDocTags is TypeScript's `ts.getJSDocTags`: the tags of the node's last JSDoc comment and of the comments of
// the nodes it inherits JSDoc from (a variable declaration's statement, ...).
func getJSDocTags(node *ts.Node) []*ts.Node {
	if node.Flags&ts.NodeFlagsJSDoc != 0 {
		return nil
	}
	var tags []*ts.Node
	for current := node; current != nil; current = ts.GetNextJSDocCommentLocation(current) {
		jsdocs := current.JSDoc(nil)
		if len(jsdocs) == 0 {
			continue
		}
		if last := jsdocs[len(jsdocs)-1].AsJSDoc(); last.Tags != nil {
			tags = append(tags, last.Tags.Nodes...)
		}
	}
	return tags
}

func isJSDocLinkLike(node *ts.Node) bool {
	return node.Kind == ts.KindJSDocLink || node.Kind == ts.KindJSDocLinkCode || node.Kind == ts.KindJSDocLinkPlain
}

// heritageExpression is TypeScript's `implement.expression` for an `implements` clause element. typescript-go parses
// class `implements` (and interface `extends`) elements as type references, not ExpressionWithTypeArguments.
func heritageExpression(element *ts.Node) *ts.Node {
	if element.Kind == ts.KindTypeReference {
		return element.AsTypeReferenceNode().TypeName
	}
	return element.Expression()
}
