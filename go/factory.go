// Port of src/util/factory.ts: shorthand factory methods (the subset the transformer uses).
//
// TypeScript's `f` namespace holds a module-level factory set per compile (`f.setFactory`); here it is a value owned
// by the TransformState, so concurrent compiles don't share it. Method names follow the TS ones.

package transformer

import (
	"fmt"
	"math"

	ts "github.com/microsoft/typescript-go/pkg/goblox"
)

type factory struct {
	nf *ts.NodeFactory
	ec *ts.EmitContext
}

func newFactory(ec *ts.EmitContext) *factory {
	return &factory{nf: ec.Factory, ec: ec}
}

// convertableExpression is `f.ConvertableExpression`: string | number | Expression | []convertableExpression | bool.
type convertableExpression = any

func (f *factory) list(nodes []*ts.Node) *ts.NodeList {
	return f.nf.NewNodeList(nodes)
}

func (f *factory) optionalList(nodes []*ts.Node) *ts.NodeList {
	if nodes == nil {
		return nil
	}
	return f.nf.NewNodeList(nodes)
}

// toExpression converts strings with `stringFn` (string literals by default).
func (f *factory) toExpression(expression convertableExpression, stringFn ...func(string) *ts.Node) *ts.Node {
	switch value := expression.(type) {
	case string:
		if len(stringFn) > 0 {
			return stringFn[0](value)
		}
		return f.string(value)
	case int:
		return f.number(float64(value))
	case float64:
		return f.number(value)
	case bool:
		return f.bool(value)
	case []convertableExpression:
		elements := make([]*ts.Node, len(value))
		for i, element := range value {
			elements[i] = f.toExpression(element)
		}
		return f.array(elements, true)
	case []string:
		elements := make([]*ts.Node, len(value))
		for i, element := range value {
			elements[i] = f.toExpression(element)
		}
		return f.array(elements, true)
	case []*ts.Node:
		return f.array(value, true)
	case *ts.Node:
		return value
	}
	panic(fmt.Sprintf("toExpression: unsupported %T", expression))
}

/// Expressions

func (f *factory) string(str string) *ts.Node {
	return f.nf.NewStringLiteral(str, 0)
}

func (f *factory) bool(value bool) *ts.Node {
	if value {
		return f.nf.NewTrueExpression()
	}
	return f.nf.NewFalseExpression()
}

func (f *factory) array(values []*ts.Node, multiLine bool) *ts.Node {
	return f.nf.NewArrayLiteralExpression(f.list(values), multiLine)
}

func (f *factory) number(value float64) *ts.Node {
	if value < 0 || (value == 0 && math.Signbit(value)) {
		return f.nf.NewPrefixUnaryExpression(ts.KindMinusToken, f.nf.NewNumericLiteral(ts.NumberToString(-value), 0))
	}
	return f.nf.NewNumericLiteral(ts.NumberToString(value), 0)
}

func (f *factory) identifier(name string, unique ...bool) *ts.Node {
	if len(unique) > 0 && unique[0] {
		return f.nf.NewUniqueNameEx(name, ts.AutoGenerateOptions{Flags: ts.GeneratedIdentifierFlagsOptimistic})
	}
	return f.nf.NewIdentifier(name)
}

func (f *factory) identifierFn() func(string) *ts.Node {
	return func(name string) *ts.Node { return f.identifier(name) }
}

func (f *factory) nil() *ts.Node {
	return f.identifier("undefined")
}

// field is `name[property]` for a string property, `name.property` for a member name.
func (f *factory) field(name convertableExpression, property convertableExpression) *ts.Node {
	object := f.toExpression(name, f.identifierFn())
	if text, ok := property.(string); ok {
		return f.nf.NewElementAccessExpression(object, nil, f.string(text), ts.NodeFlagsNone)
	}
	node := property.(*ts.Node)
	if node.Kind == ts.KindComputedPropertyName {
		return f.field(name, node.Expression())
	}
	if node.Kind == ts.KindIdentifier || node.Kind == ts.KindPrivateIdentifier {
		return f.nf.NewPropertyAccessExpression(object, nil, node, ts.NodeFlagsNone)
	}
	return f.nf.NewElementAccessExpression(object, nil, f.toExpression(node), ts.NodeFlagsNone)
}

func (f *factory) statement(expression convertableExpression) *ts.Node {
	if expression == nil {
		return f.nf.NewExpressionStatement(f.identifier("undefined"))
	}
	return f.nf.NewExpressionStatement(f.toExpression(expression))
}

func (f *factory) call(expression convertableExpression, args ...convertableExpression) *ts.Node {
	arguments := make([]*ts.Node, len(args))
	for i, arg := range args {
		arguments[i] = f.toExpression(arg)
	}
	return f.nf.NewCallExpression(f.toExpression(expression, f.identifierFn()), nil, nil, f.list(arguments), ts.NodeFlagsNone)
}

func (f *factory) object(properties []*ts.Node, multiLine bool) *ts.Node {
	return f.nf.NewObjectLiteralExpression(f.list(properties), multiLine)
}

func (f *factory) as(expression *ts.Node, node *ts.Node) *ts.Node {
	return f.nf.NewAsExpression(expression, node)
}

func (f *factory) asNever(expression *ts.Node) *ts.Node {
	return f.as(expression, f.keywordType(ts.KindNeverKeyword))
}

func (f *factory) arrowFunction(body *ts.Node) *ts.Node {
	return f.nf.NewArrowFunction(nil, nil, f.list(nil), nil, nil, f.nf.NewToken(ts.KindEqualsGreaterThanToken), body)
}

func (f *factory) bang(expression convertableExpression) *ts.Node {
	return f.nf.NewNonNullExpression(f.toExpression(expression, f.identifierFn()), ts.NodeFlagsNone)
}

func (f *factory) parenthesized(expression *ts.Node) *ts.Node {
	return f.nf.NewParenthesizedExpression(expression)
}

// templateExpression is `${head}${expression}` (one span, empty tail).
func (f *factory) templateExpression(head string, expression *ts.Node) *ts.Node {
	span := f.nf.NewTemplateSpan(expression, f.nf.NewTemplateTail("", "", 0))
	return f.nf.NewTemplateExpression(f.nf.NewTemplateHead(head, "", 0), f.list([]*ts.Node{span}))
}

/// Statements

func (f *factory) block(statements []*ts.Node) *ts.Node {
	return f.nf.NewBlock(f.list(statements), true)
}

func (f *factory) returnStatement(expression convertableExpression) *ts.Node {
	if expression == nil {
		return f.nf.NewReturnStatement(nil)
	}
	return f.nf.NewReturnStatement(f.toExpression(expression))
}

// variableStatement is `const name = initializer`.
func (f *factory) variableStatement(name *ts.Node, initializer *ts.Node) *ts.Node {
	declaration := f.nf.NewVariableDeclaration(name, nil, nil, initializer)
	list := f.nf.NewVariableDeclarationList(f.list([]*ts.Node{declaration}), ts.NodeFlagsConst)
	return f.nf.NewVariableStatement(nil, list)
}

/// Declarations

func (f *factory) propertyAssignmentDeclaration(name convertableExpression, value convertableExpression) *ts.Node {
	var nameNode *ts.Node
	if text, ok := name.(string); ok {
		nameNode = f.string(text)
	} else {
		nameNode = name.(*ts.Node)
	}
	return f.nf.NewPropertyAssignment(nil, nameNode, nil, nil, f.toExpression(value))
}

// importSpecifier is one named import: `name as identifier`.
type importSpecifier struct {
	name       string
	identifier *ts.Node
}

func (f *factory) importDeclaration(path string, imports []importSpecifier) *ts.Node {
	specifiers := make([]*ts.Node, len(imports))
	for i, entry := range imports {
		specifiers[i] = f.nf.NewImportSpecifier(false, f.identifier(entry.name), entry.identifier)
	}
	clause := f.nf.NewImportClause(ts.KindUnknown, nil, f.nf.NewNamedImports(f.list(specifiers)))
	return f.nf.NewImportDeclaration(nil, clause, f.string(path), nil)
}

func (f *factory) staticBlockDeclaration(statements []*ts.Node) *ts.Node {
	return f.nf.NewClassStaticBlockDeclaration(nil, f.block(statements))
}

/// Type Nodes

func (f *factory) keywordType(kind ts.Kind) *ts.Node {
	return f.nf.NewKeywordTypeNode(kind)
}
