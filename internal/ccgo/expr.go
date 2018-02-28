// Copyright 2017 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccgo

import (
	"math"
	"strconv"
	"strings"

	"github.com/cznic/ir"
	"github.com/cznic/sqlite2go/internal/c99"
)

func (g *gen) exprListOpt(n *c99.ExprListOpt, void bool) {
	if n == nil {
		return
	}

	g.exprList(n.ExprList, void)
}

func (g *gen) exprList(n *c99.ExprList, void bool) {
	switch {
	case void:
		for ; n != nil; n = n.ExprList {
			g.void(n.Expr)
			g.w(";")
		}
	default:
		if isSingleExpression(n) {
			g.value(n.Expr, false)
			break
		}

		todo("", g.position0(n))
	}
}

func (g *gen) exprList2(n *c99.ExprList, t c99.Type) {
	if isSingleExpression(n) {
		g.convert(n.Expr, t)
		return
	}

	i := 0
	for l := n.ExprList; l != nil; l = l.ExprList {
		if !g.voidCanIgnore(l.Expr) {
			i++
		}
	}
	switch i {
	case 0:
		var e *c99.Expr
		for l := n.ExprList; l != nil; l = l.ExprList {
			e = l.Expr
		}
		g.convert(e, t)
	case 1:
		todo("", g.position0(n))
	default:
		todo("", g.position0(n))
	}
}

func (g *gen) void(n *c99.Expr) {
	if n.Case == c99.ExprPExprList && isSingleExpression(n.ExprList) {
		g.void(n.ExprList.Expr)
		return
	}

	if n.Case == c99.ExprCast && n.Expr.Case == c99.ExprIdent && !isVaList(n.Expr.Operand.Type) {
		g.enqueue(n.Expr.Declarator)
		return
	}

	if g.voidCanIgnore(n) {
		return
	}

	switch n.Case {
	case c99.ExprCall: // Expr '(' ArgumentExprListOpt ')'
		g.value(n, false)
	case c99.ExprAssign: // Expr '=' Expr
		if n.Expr.Equals(n.Expr2) {
			return
		}

		op := n.Expr.Operand
		if op.Bits != 0 {
			g.assignmentValue(n)
			return
		}

		g.w("*")
		g.lvalue(n.Expr)
		g.w(" = ")
		if isVaList(n.Expr.Operand.Type) && n.Expr2.Case == c99.ExprInt && n.Expr2.IsZero() {
			g.w("nil")
			return
		}

		g.convert(n.Expr2, n.Expr.Operand.Type)
	case
		c99.ExprPostInc, // Expr "++"
		c99.ExprPreInc:  // "++" Expr

		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			switch sz := g.model.Sizeof(x.Item); {
			case sz == 1:
				g.w(" *(")
				g.lvalue(n.Expr)
				g.w(")++")
			default:
				g.value(n.Expr, false)
				g.w(" += %d", sz)
			}
		case c99.TypeKind:
			if n.Expr.Operand.Bits != 0 {
				//TODO ../c99/testdata/github.com/gcc-mirror/gcc/gcc/testsuite/gcc.c-torture/execute/pr55750.c:14:3
				todo("bit field %v", g.position0(n))
			}
			if x.IsArithmeticType() {
				g.w(" *(")
				g.lvalue(n.Expr)
				g.w(")++")
				return
			}
			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case
		c99.ExprPostDec, // Expr "--"
		c99.ExprPreDec:  // "--" Expr

		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			switch sz := g.model.Sizeof(x.Item); {
			case sz == 1:
				g.w(" *(")
				g.lvalue(n.Expr)
				g.w(")--")
			default:
				g.value(n.Expr, false)
				g.w(" -= %d", sz)
			}
		case c99.TypeKind:
			if n.Expr.Operand.Bits != 0 {
				todo("", g.position0(n))
			}
			if x.IsArithmeticType() {
				g.w(" *(")
				g.lvalue(n.Expr)
				g.w(")--")
				return
			}
			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprAddAssign: // Expr "+=" Expr
		switch {
		case c99.UnderlyingType(n.Expr.Operand.Type).Kind() == c99.Ptr:
			g.w(" *(")
			g.lvalue(n.Expr)
			g.w(") += %d*uintptr(", g.model.Sizeof(n.Expr.Operand.Type.(*c99.PointerType).Item))
			g.value(n.Expr2, false)
			g.w(")")
		default:
			g.voidArithmeticAsop(n)
		}
	case c99.ExprSubAssign: // Expr "-=" Expr
		switch {
		case n.Expr.Operand.Type.Kind() == c99.Ptr:
			g.w(" *(")
			g.lvalue(n.Expr)
			g.w(") -= %d*uintptr(", g.model.Sizeof(n.Expr.Operand.Type.(*c99.PointerType).Item))
			g.value(n.Expr2, false)
			g.w(")")
		default:
			g.voidArithmeticAsop(n)
		}
	case
		c99.ExprAndAssign, // Expr "&=" Expr
		c99.ExprDivAssign, // Expr "/=" Expr
		c99.ExprLshAssign, // Expr "<<=" Expr
		c99.ExprModAssign, // Expr "%=" Expr
		c99.ExprMulAssign, // Expr "*=" Expr
		c99.ExprOrAssign,  // Expr "|=" Expr
		c99.ExprRshAssign, // Expr ">>=" Expr
		c99.ExprXorAssign: // Expr "^=" Expr

		g.voidArithmeticAsop(n)
	case c99.ExprPExprList: // '(' ExprList ')'
		for l := n.ExprList; l != nil; l = l.ExprList {
			g.void(l.Expr)
			g.w(";")
		}
	case c99.ExprCast: // '(' TypeName ')' Expr
		if isVaList(n.Expr.Operand.Type) { //TODO- ?
			g.w("%sVA%s(&", crt, g.typ(c99.UnderlyingType(n.TypeName.Type)))
			g.value(n.Expr, false)
			g.w(")")
			return
		}

		g.void(n.Expr)
	case c99.ExprCond: // Expr '?' ExprList ':' Expr
		switch {
		case n.Expr.IsZero() && g.voidCanIgnore(n.Expr):
			switch {
			case g.voidCanIgnoreExprList(n.ExprList):
				switch {
				case g.voidCanIgnore(n.Expr2):
					todo("", g.position0(n))
				default:
					g.void(n.Expr2)
				}
			default:
				switch {
				case g.voidCanIgnore(n.Expr2):
					todo("", g.position0(n))
				default:
					todo("", g.position0(n))
				}
			}
		case n.Expr.IsNonZero() && g.voidCanIgnore(n.Expr):
			todo("", g.position0(n))
		default:
			switch {
			case g.voidCanIgnoreExprList(n.ExprList):
				switch {
				case g.voidCanIgnore(n.Expr2):
					todo("", g.position0(n))
				default:
					// if expr == 0 {
					//	expr2
					// }
					g.w("if ")
					g.value(n.Expr, false)
					g.w(" == 0 {")
					g.void(n.Expr2)
					g.w("}")
				}
			default:
				switch {
				case g.voidCanIgnore(n.Expr2):
					todo("", g.position0(n))
				default:
					// if expr != 0 {
					//	exprList
					// } else {
					//	expr2
					// }
					g.w("if ")
					g.value(n.Expr, false)
					g.w(" != 0 {")
					g.exprList(n.ExprList, true)
					g.w("} else {")
					g.void(n.Expr2)
					g.w("}")
				}
			}
		}
	case c99.ExprLAnd: // Expr "&&" Expr
		if n.Expr.IsZero() && g.voidCanIgnore(n.Expr) {
			return
		}

		if n.Expr2.IsZero() {
			g.void(n.Expr)
			return
		}

		g.w("if ")
		g.value(n.Expr, false)
		g.w(" != 0 {")
		g.void(n.Expr2)
		g.w("}")
	case c99.ExprLOr: // Expr "||" Expr
		if n.Expr.IsNonZero() && g.voidCanIgnore(n.Expr) {
			return
		}

		if n.Expr2.IsNonZero() {
			g.void(n.Expr)
			return
		}

		g.w("if ")
		g.value(n.Expr, false)
		g.w(" == 0 {")
		g.void(n.Expr2)
		g.w("}")
	case c99.ExprIndex: // Expr '[' ExprList ']'
		g.void(n.Expr)
		if !g.voidCanIgnoreExprList(n.ExprList) {
			g.w("\n")
		}
		g.exprList(n.ExprList, true)
	case // Unary
		c99.ExprAddrof,     // '&' Expr
		c99.ExprCpl,        // '~' Expr
		c99.ExprDeref,      // '*' Expr
		c99.ExprNot,        // '!' Expr
		c99.ExprUnaryMinus, // '-' Expr
		c99.ExprUnaryPlus:  // '+' Expr

		g.void(n.Expr)
	case // Binary
		c99.ExprAdd, // Expr '+' Expr
		c99.ExprAnd, // Expr '&' Expr
		c99.ExprDiv, // Expr '/' Expr
		c99.ExprEq,  // Expr "==" Expr
		c99.ExprGe,  // Expr ">=" Expr
		c99.ExprGt,  // Expr ">" Expr
		c99.ExprLe,  // Expr "<=" Expr
		c99.ExprLsh, // Expr "<<" Expr
		c99.ExprLt,  // Expr '<' Expr
		c99.ExprMod, // Expr '%' Expr
		c99.ExprMul, // Expr '*' Expr
		c99.ExprNe,  // Expr "!=" Expr
		c99.ExprOr,  // Expr '|' Expr
		c99.ExprRsh, // Expr ">>" Expr
		c99.ExprSub, // Expr '-' Expr
		c99.ExprXor: // Expr '^' Expr

		g.void(n.Expr)
		if !g.voidCanIgnore(n.Expr2) {
			g.w(";")
		}
		g.void(n.Expr2)
	default:
		todo("", g.position0(n), n.Case, n.Operand) // void
	}
}

func (g *gen) lvalue(n *c99.Expr) {
	g.w("&")
	g.value(n, false)
}

func (g *gen) value(n *c99.Expr, packedField bool) {
	if n.Case == c99.ExprPExprList && isSingleExpression(n.ExprList) {
		g.value(n.ExprList.Expr, false)
		return
	}

	g.w("(")

	defer g.w(")")

	if n.Operand.Value != nil {
		switch {
		case g.voidCanIgnore(n):
			g.constant(n)
		default:
			g.w("func() %v {", g.typ(n.Operand.Type))
			g.void(n)
			g.w("; return ")
			op := n.Operand //TODO unhack
			n.Operand = n.Operand.ConvertTo(g.model, n.Operand.Type)
			g.constant(n)
			n.Operand = op
			g.w(" }()")
		}
		return
	}

	switch n.Case {
	case c99.ExprIdent: // IDENTIFIER
		d := g.normalizeDeclarator(n.Declarator)
		switch {
		case d == nil:
			if n.Operand.Type == nil || n.Operand.Value == nil {
				todo("", g.position0(n), n.Operand)
			}

			// Enum const
			g.w("%s(", g.typ(n.Operand.Type))
			g.constant(n)
			g.w(")")
		default:
			g.enqueue(d)
			switch {
			case d.Type.Kind() == c99.Function:
				g.w("%s(%s)", g.registerHelper("fp%d", g.typ(d.Type)), g.mangleDeclarator(d))
			case g.escaped(d) && c99.UnderlyingType(d.Type).Kind() != c99.Array:
				g.w(" *(*%s)(unsafe.Pointer(%s))", g.typ(d.Type), g.mangleDeclarator(d))
			default:
				g.w("%s", g.mangleDeclarator(d))
			}
		}
	case
		c99.ExprEq, // Expr "==" Expr
		c99.ExprGe, // Expr ">=" Expr
		c99.ExprGt, // Expr ">" Expr
		c99.ExprLe, // Expr "<=" Expr
		c99.ExprLt, // Expr '<' Expr
		c99.ExprNe: // Expr "!=" Expr

		g.relop(n)
	case
		c99.ExprAnd, // Expr '&' Expr
		c99.ExprDiv, // Expr '/' Expr
		c99.ExprMod, // Expr '%' Expr
		c99.ExprMul, // Expr '*' Expr
		c99.ExprOr,  // Expr '|' Expr
		c99.ExprXor: // Expr '^' Expr

		g.binop(n)
	case c99.ExprCall: // Expr '(' ArgumentExprListOpt ')'
		if d := n.Expr.Declarator; d != nil && d.Name() == idAlloca {
			g.w("alloca(&allocs, int(")
			if n.ArgumentExprListOpt.ArgumentExprList.ArgumentExprList != nil {
				todo("", g.position0(n))
			}
			g.value(n.ArgumentExprListOpt.ArgumentExprList.Expr, false)
			g.w("))")
			return
		}

		if n.Expr.Case == c99.ExprIdent && n.Expr.Declarator == nil {
			switch x := n.Expr.Scope.LookupIdent(n.Expr.Token.Val).(type) {
			case *c99.Declarator:
				n.Expr.Declarator = x
				n.Expr.Operand.Type = &c99.PointerType{Item: x.Type}
			default:
				todo("%v: %T", g.position0(n), x)
			}
		}
		var ft0 c99.Type
		if !isFnPtr(n.Expr.Operand.Type, &ft0) {
			todo("", g.position0(n))
		}
		ft := ft0.(*c99.FunctionType)
		g.convert(n.Expr, ft)
		g.w("(tls")
		if o := n.ArgumentExprListOpt; o != nil {
			i := 0
			for l := o.ArgumentExprList; l != nil; l = l.ArgumentExprList {
				switch {
				case i < len(ft.Params) || ft.Variadic:
					g.w(", ")
					if t := n.CallArgs[i].Type; t != nil {
						g.convert(l.Expr, t)
						break
					}

					g.value(l.Expr, false)
				default:
					if !g.voidCanIgnore(l.Expr) && l.Expr.Case != c99.ExprIdent {
						todo("", g.position0(l.Expr))
					}
				}
				i++
			}
		}
		g.w(")")
	case c99.ExprAddrof: // '&' Expr
		g.uintptr(n.Expr, false)
	case c99.ExprSelect: // Expr '.' IDENTIFIER
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case *c99.StructType:
			d := x.Field(n.Token2.Val)
			fp := g.model.Layout(x)[d.Field]
			switch {
			case d.Type.Kind() == c99.Array:
				g.uintptr(n.Expr, false)
				g.w("+%d", fp.Offset)
			default:
				switch {
				case fp.Bits != 0 && !packedField:
					g.bitField(n)
				default:
					t := n.Operand.Type
					if fp.Bits != 0 {
						t = fp.PackedType
					}
					g.w("*(*%s)(unsafe.Pointer(", g.typ(t))
					g.uintptr(n.Expr, false)
					g.w("+%d", fp.Offset)
					g.w("))")
				}
			}
		case *c99.UnionType:
			d := x.Field(n.Token2.Val)
			fp := g.model.Layout(x)[d.Field]
			switch {
			case d.Type.Kind() == c99.Array:
				g.uintptr(n.Expr, false)
			default:
				switch {
				case fp.Bits != 0 && !packedField:
					g.bitField(n)
				default:
					t := n.Operand.Type
					if fp.Bits != 0 {
						t = fp.PackedType
					}
					g.w("*(*%s)(unsafe.Pointer(", g.typ(t))
					g.uintptr(n.Expr, false)
					g.w("))")
				}
			}
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPSelect: // Expr "->" IDENTIFIER
		switch x := c99.UnderlyingType(c99.UnderlyingType(n.Expr.Operand.Type).(*c99.PointerType).Item).(type) {
		case *c99.StructType:
			layout := g.model.Layout(x)
			d := x.Field(n.Token2.Val)
			fp := layout[d.Field]
			switch {
			case d.Type.Kind() == c99.Array:
				g.value(n.Expr, false)
				g.w("+%d", g.model.Layout(x)[d.Field].Offset)
			default:
				switch {
				case fp.Bits != 0 && !packedField:
					g.bitField(n)
				default:
					t := n.Operand.Type
					if fp.Bits != 0 {
						t = fp.PackedType
					}
					g.w("*(*%s)(unsafe.Pointer(", g.typ(t))
					g.value(n.Expr, false)
					g.w("+%d))", g.model.Layout(x)[d.Field].Offset)
				}
			}
		case *c99.UnionType:
			layout := g.model.Layout(x)
			d := x.Field(n.Token2.Val)
			fp := layout[d.Field]
			switch {
			case d.Type.Kind() == c99.Array:
				todo("", g.position0(n))
			default:
				switch {
				case fp.Bits != 0:
					todo("", g.position0(n))
				default:
					t := n.Operand.Type
					if fp.Bits != 0 {
						t = fp.PackedType
					}
					g.w("*(*%s)(unsafe.Pointer(", g.typ(t))
					g.value(n.Expr, false)
					g.w("))")
				}
			}
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprIndex: // Expr '[' ExprList ']'
		var it c99.Type
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case *c99.ArrayType:
			it = x.Item
		case *c99.PointerType:
			it = x.Item
		default:
			todo("%v: %T", g.position0(n), x)
		}
		switch {
		case it.Kind() == c99.Array:
			g.value(n.Expr, false)
			g.indexOff(n.ExprList, it)
		default:
			g.w("*(*%s)(unsafe.Pointer(", g.typ(n.Operand.Type))
			g.value(n.Expr, false)
			g.indexOff(n.ExprList, it)
			g.w("))")
		}
	case c99.ExprAdd: // Expr '+' Expr
		switch t, u := c99.UnderlyingType(n.Expr.Operand.Type), c99.UnderlyingType(n.Expr2.Operand.Type); {
		case t.Kind() == c99.Ptr:
			g.value(n.Expr, false)
			g.w(" + %d*uintptr(", g.model.Sizeof(t.(*c99.PointerType).Item))
			g.value(n.Expr2, false)
			g.w(")")
		case u.Kind() == c99.Ptr:
			g.w("%d*uintptr(", g.model.Sizeof(u.(*c99.PointerType).Item))
			g.value(n.Expr, false)
			g.w(") + ")
			g.value(n.Expr2, false)
		default:
			g.binop(n)
		}
	case c99.ExprSub: // Expr '-' Expr
		switch t, u := c99.UnderlyingType(n.Expr.Operand.Type), c99.UnderlyingType(n.Expr2.Operand.Type); {
		case t.Kind() == c99.Ptr && u.Kind() == c99.Ptr:
			g.w("%s((", g.typ(n.Operand.Type))
			g.value(n.Expr, false)
			g.w(" - ")
			g.value(n.Expr2, false)
			g.w(")/%d)", g.model.Sizeof(t.(*c99.PointerType).Item))
		case t.Kind() == c99.Ptr:
			g.value(n.Expr, false)
			g.w(" - %d*uintptr(", g.model.Sizeof(t.(*c99.PointerType).Item))
			g.value(n.Expr2, false)
			g.w(")")
		default:
			g.binop(n)
		}
	case c99.ExprDeref: // '*' Expr
		it := c99.UnderlyingType(n.Expr.Operand.Type).(*c99.PointerType).Item
		switch it.Kind() {
		case
			c99.Array,
			c99.Function:

			g.value(n.Expr, false)
		default:
			i := 1
			for n.Expr.Case == c99.ExprDeref {
				i++
				n = n.Expr
			}
			g.w("%[1]s(%[1]s%[2]s)(unsafe.Pointer(", strings.Repeat("*", i), g.typ(it))
			g.value(n.Expr, false)
			g.w("))")
		}
	case c99.ExprAssign: // Expr '=' Expr
		g.assignmentValue(n)
	case c99.ExprLAnd: // Expr "&&" Expr
		if n.Operand.Value != nil && g.voidCanIgnore(n) {
			g.constant(n)
			break
		}

		g.needBool2int++
		if n.Expr.IsZero() {
			g.w(" bool2int(")
			g.value(n.Expr, false)
			g.w(" != 0)")
			break
		}

		if n.Expr.Case == c99.ExprIdent && n.Expr2.Case == c99.ExprIdent && n.Expr.Token.Val == n.Expr2.Token.Val {
			g.w(" bool2int(")
			g.value(n.Expr, false)
			g.w(" != 0)")
			break
		}

		g.w(" bool2int((")
		g.value(n.Expr, false)
		g.w(" != 0) && (")
		g.value(n.Expr2, false)
		g.w(" != 0))")
	case c99.ExprLOr: // Expr "||" Expr
		if n.Operand.Value != nil && g.voidCanIgnore(n) {
			g.constant(n)
			break
		}

		g.needBool2int++
		if n.Expr.IsNonZero() {
			g.w(" bool2int(")
			g.value(n.Expr, false)
			g.w(" != 0)")
			break
		}

		g.w(" bool2int((")
		g.value(n.Expr, false)
		g.w(" != 0) || (")
		g.value(n.Expr2, false)
		g.w(" != 0))")
	case c99.ExprCond: // Expr '?' ExprList ':' Expr
		t := n.Operand.Type
		t0 := t
		switch {
		case !g.voidCanIgnore(n.Expr):
			fallthrough
		default:
			g.w(" func() %s { if ", g.typ(t))
			g.value(n.Expr, false)
			g.w(" != 0 { return ")
			g.exprList2(n.ExprList, t0)
			g.w(" }\n\nreturn ")
			g.convert(n.Expr2, t0)
			g.w(" }()")
		case n.Expr.IsZero():
			g.value(n.Expr2, false)
		case n.Expr.IsNonZero():
			g.exprList(n.ExprList, false)
		}
	case c99.ExprCast: // '(' TypeName ')' Expr
		t := n.TypeName.Type
		op := n.Expr.Operand
		if isVaList(op.Type) {
			g.w("%sVA%s(&", crt, g.typ(c99.UnderlyingType(t)))
			g.value(n.Expr, false)
			g.w(")")
			return
		}

		switch x := c99.UnderlyingType(t).(type) {
		case *c99.PointerType:
			if d := n.Expr.Declarator; x.Item.Kind() == c99.Function && d != nil && g.normalizeDeclarator(d).Type.Equal(x.Item) {
				g.value(n.Expr, false)
				return
			}
		}

		g.convert(n.Expr, t)
	case c99.ExprPreInc: // "++" Expr
		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			g.w("%s(", g.registerHelper("preinc%d", g.typ(x), g.model.Sizeof(x.Item)))
			g.lvalue(n.Expr)
			g.w(")")
		case c99.TypeKind:
			if op := n.Expr.Operand; op.Bits != 0 {
				g.w("%s(&", g.registerHelper("preinc%db", 1, g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
				g.value(n.Expr, true)
				g.w(")")
				return
			}

			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("preinc%d", g.typ(x), 1))
				g.lvalue(n.Expr)
				g.w(")")
				return
			}

			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPostInc: // Expr "++"
		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			g.w("%s(", g.registerHelper("postinc%d", g.typ(x), g.model.Sizeof(x.Item)))
			g.lvalue(n.Expr)
			g.w(")")
		case c99.TypeKind:
			if op := n.Expr.Operand; op.Bits != 0 {
				g.w("%s(&", g.registerHelper("postinc%db", 1, g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
				g.value(n.Expr, true)
				g.w(")")
				return
			}

			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("postinc%d", g.typ(x), 1))
				g.lvalue(n.Expr)
				g.w(")")
				return
			}

			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPreDec: // "--" Expr
		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			g.w("%s(", g.registerHelper("preinc%d", g.typ(x), g.int64ToUintptr(-g.model.Sizeof(x.Item))))
			g.lvalue(n.Expr)
			g.w(")")
		case c99.TypeKind:
			if op := n.Expr.Operand; op.Bits != 0 {
				g.w("%s(&", g.registerHelper("preinc%db", c99.ConvertInt64(-1, x, g.model), g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
				g.value(n.Expr, true)
				g.w(")")
				return
			}

			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("preinc%d", g.typ(x), c99.ConvertInt64(-1, x, g.model)))
				g.lvalue(n.Expr)
				g.w(")")
				return
			}
			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPostDec: // Expr "--"
		switch x := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			g.w("%s(", g.registerHelper("postinc%d", g.typ(x), g.int64ToUintptr(-g.model.Sizeof(x.Item))))
			g.lvalue(n.Expr)
			g.w(")")
		case c99.TypeKind:
			op := n.Expr.Operand
			if op.Bits != 0 {
				g.w("%s(&", g.registerHelper("postinc%db", c99.ConvertInt64(-1, x, g.model), g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
				g.value(n.Expr, true)
				g.w(")")
				return
			}

			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("postinc%d", g.typ(x), c99.ConvertInt64(-1, x, g.model)))
				g.lvalue(n.Expr)
				g.w(")")
				return
			}
			todo("%v: %v", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprNot: // '!' Expr
		g.needBool2int++
		g.w(" bool2int(")
		g.value(n.Expr, false)
		g.w(" == 0)")
	case c99.ExprLsh: // Expr "<<" Expr
		g.convert(n.Expr, n.Operand.Type)
		g.w(" << (uint(")
		g.value(n.Expr2, false)
		g.w(") %% %d)", g.shiftMod(c99.UnderlyingType(n.Operand.Type)))
	case c99.ExprRsh: // Expr ">>" Expr
		g.convert(n.Expr, n.Operand.Type)
		g.w(" >> (uint(")
		g.value(n.Expr2, false)
		g.w(") %% %d)", g.shiftMod(c99.UnderlyingType(n.Operand.Type)))
	case c99.ExprUnaryMinus: // '-' Expr
		g.w("- ")
		g.convert(n.Expr, n.Operand.Type)
	case c99.ExprCpl: // '~' Expr
		g.w("^(")
		g.convert(n.Expr, n.Operand.Type)
		g.w(")")
	case c99.ExprAddAssign: // Expr "+=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case *c99.PointerType:
			g.needPreInc = true
			g.w("preinc(")
			g.lvalue(n.Expr)
			g.w(", %d*uintptr(", g.model.Sizeof(x.Item))
			g.value(n.Expr2, false)
			g.w("))")
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("add%d", "+", g.typ(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprSubAssign: // Expr "-=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("sub%d", "-", g.typ(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprOrAssign: // Expr "|=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsIntegerType() {
				switch op := n.Expr.Operand; {
				case op.Bits != 0:
					g.w("%s(&", g.registerHelper("or%db", "|", g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
					g.value(n.Expr, true)
					g.w(", ")
					g.convert(n.Expr2, n.Operand.Type)
					g.w(")")
				default:
					g.w("%s(", g.registerHelper("or%d", "|", g.typ(x)))
					g.lvalue(n.Expr)
					g.w(", ")
					g.convert(n.Expr2, x)
					g.w(")")
				}
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprAndAssign: // Expr "&=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsIntegerType() {
				switch op := n.Expr.Operand; {
				case op.Bits != 0:
					g.w("%s(&", g.registerHelper("and%db", "&", g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
					g.value(n.Expr, true)
					g.w(", ")
					g.convert(n.Expr2, n.Operand.Type)
					g.w(")")
				default:
					g.w("%s(", g.registerHelper("and%d", "&", g.typ(x)))
					g.lvalue(n.Expr)
					g.w(", ")
					g.convert(n.Expr2, x)
					g.w(")")
				}
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprXorAssign: // Expr "^=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsIntegerType() {
				switch op := n.Expr.Operand; {
				case op.Bits != 0:
					g.w("%s(&", g.registerHelper("xor%db", "^", g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
					g.value(n.Expr, true)
					g.w(", ")
					g.convert(n.Expr2, n.Operand.Type)
					g.w(")")
				default:
					g.w("%s(", g.registerHelper("xor%d", "^", g.typ(x)))
					g.lvalue(n.Expr)
					g.w(", ")
					g.convert(n.Expr2, x)
					g.w(")")
				}
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPExprList: // '(' ExprList ')'
		var e, last *c99.Expr
		i := 0
		for l := n.ExprList; l != nil; l = l.ExprList {
			last = l.Expr
			if !g.voidCanIgnore(l.Expr) {
				e = l.Expr
				i++
			}
		}
		switch {
		case i == 0:
			g.value(last, packedField)
		case i == 1 && e == last:
			g.value(e, packedField)
		default:
			g.w("func() %v {", g.typ(n.Operand.Type))
			for l := n.ExprList; l != nil; l = l.ExprList {
				switch {
				case l.ExprList == nil:
					g.w("return ")
					g.convert(l.Expr, n.Operand.Type)
				default:
					g.void(l.Expr)
				}
				g.w(";")
			}
			g.w("}()")
		}
	case c99.ExprMulAssign: // Expr "*=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("mul%d", "*", g.typ(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprDivAssign: // Expr "/=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("div%d", "/", g.typ(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprModAssign: // Expr "%=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("mod%d", "%", g.typ(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprRshAssign: // Expr ">>=" Expr
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case c99.TypeKind:
			if x.IsArithmeticType() {
				g.w("%s(", g.registerHelper("rsh%d", ">>", g.typ(x), g.shiftMod(x)))
				g.lvalue(n.Expr)
				g.w(", ")
				g.convert(n.Expr2, x)
				g.w(")")
				return
			}
			todo("", g.position0(n), x)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprUnaryPlus: // '+' Expr
		g.convert(n.Expr, n.Operand.Type)
	case
		c99.ExprInt,        // INTCONST
		c99.ExprSizeofExpr, // "sizeof" Expr
		c99.ExprSizeofType, // "sizeof" '(' TypeName ')'
		c99.ExprString:     // STRINGLITERAL

		g.constant(n)
	default:
		todo("", g.position0(n), n.Case, n.Operand) // value
	}
}

func (g *gen) bitField(n *c99.Expr) {
	op := n.Operand
	g.w("%s(", g.typ(op.Type))
	g.value(n, true)
	bits := int(g.model.Sizeof(op.Type) * 8)
	g.w(">>%d)<<%d>>%d", op.Bitoff, bits-op.Bits, bits-op.Bits)
}

func (g *gen) indexOff(n *c99.ExprList, it c99.Type) {
	switch {
	case n.Operand.Value != nil && g.voidCanIgnoreExprList(n):
		g.w("%+d", g.model.Sizeof(it)*n.Operand.Value.(*ir.Int64Value).Value)
	default:
		g.w(" + %d*uintptr(", g.model.Sizeof(it))
		g.exprList(n, false)
		g.w(")")
	}
}

func (g *gen) uintptr(n *c99.Expr, packedField bool) {
	if n.Case == c99.ExprPExprList && isSingleExpression(n.ExprList) {
		g.uintptr(n.ExprList.Expr, packedField)
		return
	}

	g.w("(")

	defer g.w(")")

	switch n.Case {
	case c99.ExprIdent: // IDENTIFIER
		d := g.normalizeDeclarator(n.Declarator)
		g.enqueue(d)
		arr := c99.UnderlyingType(d.Type).Kind() == c99.Array
		switch {
		case d.Type.Kind() == c99.Function:
			g.w("%s(%s)", g.registerHelper("fp%d", g.typ(d.Type)), g.mangleDeclarator(d))
		case arr:
			g.w("%s", g.mangleDeclarator(d))
		case g.escaped(d):
			g.w("%s", g.mangleDeclarator(d))
		default:
			g.w("uintptr(unsafe.Pointer(&%s))", g.mangleDeclarator(d))
		}
	case c99.ExprIndex: // Expr '[' ExprList ']'
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case *c99.ArrayType:
			g.uintptr(n.Expr, false)
			g.indexOff(n.ExprList, x.Item)
		case *c99.PointerType:
			g.value(n.Expr, false)
			g.indexOff(n.ExprList, x.Item)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprSelect: // Expr '.' IDENTIFIER
		switch x := c99.UnderlyingType(n.Expr.Operand.Type).(type) {
		case *c99.StructType:
			f := x.Field(n.Token2.Val)
			layout := g.model.Layout(x)
			if bits := layout[f.Field].Bits; bits != 0 && !packedField {
				todo("", g.position0(n), n.Operand)
			}
			g.uintptr(n.Expr, packedField)
			g.w("+%d", layout[f.Field].Offset)
		case *c99.UnionType:
			f := x.Field(n.Token2.Val)
			layout := g.model.Layout(x)
			if bits := layout[f.Field].Bits; bits != 0 && !packedField {
				todo("", g.position0(n), n.Operand)
			}
			g.uintptr(n.Expr, packedField)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprPSelect: // Expr "->" IDENTIFIER
		switch x := c99.UnderlyingType(c99.UnderlyingType(n.Expr.Operand.Type).(*c99.PointerType).Item).(type) {
		case *c99.StructType:
			layout := g.model.Layout(x)
			f := x.Field(n.Token2.Val)
			if bits := layout[f.Field].Bits; bits != 0 && !packedField {
				todo("", g.position0(n), n.Operand)
			}
			g.value(n.Expr, false)
			g.w("+%d", g.model.Layout(x)[f.Field].Offset)
		case *c99.UnionType:
			layout := g.model.Layout(x)
			f := x.Field(n.Token2.Val)
			if bits := layout[f.Field].Bits; bits != 0 && !packedField {
				todo("", g.position0(n), n.Operand)
			}
			g.value(n.Expr, false)
		default:
			todo("%v: %T", g.position0(n), x)
		}
	case c99.ExprDeref: // '*' Expr
		switch c99.UnderlyingType(c99.UnderlyingType(n.Expr.Operand.Type).(*c99.PointerType).Item).(type) {
		case *c99.ArrayType:
			g.value(n.Expr, false)
		default:
			g.value(n.Expr, false)
		}
	case c99.ExprString: // STRINGLITERAL
		g.constant(n)
	default:
		todo("", g.position0(n), n.Case, n.Operand) // uintptr
	}
}

func (g *gen) voidCanIgnore(n *c99.Expr) bool {
	if n.Case == c99.ExprPExprList && isSingleExpression(n.ExprList) {
		return g.voidCanIgnore(n.ExprList.Expr)
	}

	switch n.Case {
	case
		c99.ExprChar,       // CHARCONST
		c99.ExprFloat,      // FLOATCONST
		c99.ExprIdent,      // IDENTIFIER
		c99.ExprInt,        // INTCONST
		c99.ExprSizeofExpr, // "sizeof" Expr
		c99.ExprSizeofType, // "sizeof" '(' TypeName ')'
		c99.ExprString:     // STRINGLITERAL

		return true
	case c99.ExprPExprList: // '(' ExprList ')'
		return g.voidCanIgnoreExprList(n.ExprList)
	case
		c99.ExprAddAssign, // Expr "+=" Expr
		c99.ExprAndAssign, // Expr "&=" Expr
		c99.ExprAssign,    // Expr '=' Expr
		c99.ExprCall,      // Expr '(' ArgumentExprListOpt ')'
		c99.ExprDivAssign, // Expr "/=" Expr
		c99.ExprLshAssign, // Expr "<<=" Expr
		c99.ExprModAssign, // Expr "%=" Expr
		c99.ExprMulAssign, // Expr "*=" Expr
		c99.ExprOrAssign,  // Expr "|=" Expr
		c99.ExprPostDec,   // Expr "--"
		c99.ExprPostInc,   // Expr "++"
		c99.ExprPreDec,    // "--" Expr
		c99.ExprPreInc,    // "++" Expr
		c99.ExprRshAssign, // Expr ">>=" Expr
		c99.ExprSubAssign, // Expr "-=" Expr
		c99.ExprXorAssign: // Expr "^=" Expr

		return false
	case c99.ExprCast: // '(' TypeName ')' Expr
		return !isVaList(n.Expr.Operand.Type) && g.voidCanIgnore(n.Expr)
	case c99.ExprCond: // Expr '?' ExprList ':' Expr
		if !g.voidCanIgnore(n.Expr) {
			return false
		}

		switch {
		case n.Expr.IsNonZero():
			return g.voidCanIgnoreExprList(n.ExprList)
		case n.Expr.IsZero():
			return g.voidCanIgnore(n.Expr2)
		}
		return false
	case
		c99.ExprAdd, // Expr '+' Expr
		c99.ExprAnd, // Expr '&' Expr
		c99.ExprDiv, // Expr '/' Expr
		c99.ExprEq,  // Expr "==" Expr
		c99.ExprGe,  // Expr ">=" Expr
		c99.ExprGt,  // Expr ">" Expr
		c99.ExprLe,  // Expr "<=" Expr
		c99.ExprLsh, // Expr "<<" Expr
		c99.ExprLt,  // Expr '<' Expr
		c99.ExprMod, // Expr '%' Expr
		c99.ExprMul, // Expr '*' Expr
		c99.ExprNe,  // Expr "!=" Expr
		c99.ExprOr,  // Expr '|' Expr
		c99.ExprRsh, // Expr ">>" Expr
		c99.ExprSub, // Expr '-' Expr
		c99.ExprXor: // Expr '^' Expr

		return g.voidCanIgnore(n.Expr) && g.voidCanIgnore(n.Expr2)
	case c99.ExprLAnd: // Expr "&&" Expr
		return g.voidCanIgnore(n.Expr) && g.voidCanIgnore(n.Expr2)
	case c99.ExprLOr: // Expr "||" Expr
		return g.voidCanIgnore(n.Expr) && g.voidCanIgnore(n.Expr2)
	case
		c99.ExprAddrof,     // '&' Expr
		c99.ExprCpl,        // '~' Expr
		c99.ExprDeref,      // '*' Expr
		c99.ExprNot,        // '!' Expr
		c99.ExprPSelect,    // Expr "->" IDENTIFIER
		c99.ExprSelect,     // Expr '.' IDENTIFIER
		c99.ExprUnaryMinus, // '-' Expr
		c99.ExprUnaryPlus:  // '+' Expr

		return g.voidCanIgnore(n.Expr)
	case c99.ExprIndex: // Expr '[' ExprList ']'
		return g.voidCanIgnore(n.Expr) && g.voidCanIgnoreExprList(n.ExprList)
	default:
		todo("", g.position0(n), n.Case, n.Operand) // voidCanIgnore
	}
	panic("unreachable")
}

func (g *gen) voidCanIgnoreExprList(n *c99.ExprList) bool {
	if isSingleExpression(n) {
		return g.voidCanIgnore(n.Expr)
	}

	for l := n; l != nil; l = l.ExprList {
		if !g.voidCanIgnore(l.Expr) {
			return false
		}
	}

	return true
}

func (g *gen) constant(n *c99.Expr) {
	switch x := n.Operand.Value.(type) {
	case *ir.Float32Value:
		switch {
		case math.IsInf(float64(x.Value), 1):
			g.w("math.Inf(1)")
			return
		case math.IsInf(float64(x.Value), -1):
			g.w("math.Inf(-1)")
			return
		}
		switch u := c99.UnderlyingType(n.Operand.Type).(type) {
		case c99.TypeKind:
			switch u {
			case c99.Float:
				g.w(" %v", x.Value)
				return
			default:
				todo("", g.position0(n), u)
			}
		default:
			todo("%v: %T", g.position0(n), u)
		}
	case *ir.Float64Value:
		switch {
		case math.IsInf(x.Value, 1):
			g.w("math.Inf(1)")
			return
		case math.IsInf(x.Value, -1):
			g.w("math.Inf(-1)")
			return
		}
		switch u := c99.UnderlyingType(n.Operand.Type).(type) {
		case c99.TypeKind:
			switch u {
			case c99.Char, c99.SChar:
				g.w(" %v", int8(x.Value))
			case c99.Int:
				g.w(" %v", int32(x.Value))
			case c99.Short:
				g.w(" %v", int16(x.Value))
			case c99.UInt:
				g.w(" %v", uint32(x.Value))
			case c99.UShort:
				g.w(" %v", uint16(x.Value))
			case
				c99.Double,
				c99.LongDouble:

				switch {
				case x.Value == 0 && math.Copysign(1, x.Value) == -1:
					g.w(" nz64")
					g.needNZ64 = true
				default:
					g.w(" %v", x.Value)
				}
				return
			case c99.Float:
				switch {
				case x.Value == 0 && math.Copysign(1, x.Value) == -1:
					g.w(" nz32")
					g.needNZ32 = true
				default:
					g.w(" %v", x.Value)
				}
				return
			default:
				todo("", g.position0(n), u)
			}
		default:
			todo("%v: %T", g.position0(n), u)
		}
	case *ir.Int64Value:
		if n.Case == c99.ExprChar {
			g.w(" %s", strconv.QuoteRuneToASCII(rune(x.Value)))
			return
		}

		f := " %d"
		m := n
		s := ""
		for done := false; !done; { //TODO-
			switch m.Case {
			case c99.ExprInt: // INTCONST
				s = string(m.Token.S())
				done = true
			case
				c99.ExprCast,       // '(' TypeName ')' Expr
				c99.ExprUnaryMinus: // '-' Expr

				m = m.Expr
			default:
				done = true
			}
		}
		s = strings.ToLower(s)
		switch {
		case strings.HasPrefix(s, "0x"):
			f = "%#x"
		case strings.HasPrefix(s, "0"):
			f = "%#o"
		}

		switch y := c99.UnderlyingType(n.Operand.Type).(type) {
		case *c99.PointerType:
			if n.IsZero() {
				g.w("%s", null)
				return
			}

			switch {
			case y.Item.Kind() == c99.Function:
				g.w("uintptr(%v)", uintptr(x.Value))
			case n.Operand.Type.String() == vaListType && x.Value == 1:
				todo("TODO-")
				//TODO- g.w(" %s", ap)
			default:
				g.w("uintptr("+f+")", uintptr(x.Value))
			}
			return
		}

		switch {
		case n.Operand.Type.IsUnsigned():
			g.w(f, uint64(c99.ConvertInt64(x.Value, n.Operand.Type, g.model)))
		default:
			g.w(f, c99.ConvertInt64(x.Value, n.Operand.Type, g.model))
		}
	case *ir.StringValue:
		g.w(" ts+%d %s", g.allocString(int(x.StringID)), strComment(x))
	case *ir.AddressValue:
		if x == c99.Null {
			g.w("%s", null)
			return
		}

		todo("", g.position0(n))
	default:
		todo("%v: %v %T(%v)", g.position0(n), n.Operand, x, x)
	}
} // constant

func (g *gen) voidArithmeticAsop(n *c99.Expr) {
	var mask uint64
	op, _ := c99.UsualArithmeticConversions(g.model, n.Expr.Operand, n.Expr2.Operand)
	lhs := n.Expr.Operand
	switch {
	case lhs.Bits != 0:
		mask = (uint64(1)<<uint(lhs.Bits) - 1) << uint(lhs.Bitoff)
		g.w("{ p := &")
		g.value(n.Expr, true)
		sz := int(g.model.Sizeof(lhs.Type) * 8)
		g.w("; *p = (*p &^ %#x) | (%s((%s(*p>>%d)<<%d>>%[5]d) ", mask, g.typ(lhs.PackedType), g.typ(op.Type), lhs.Bitoff, sz-lhs.Bits)
	case n.Expr.Declarator != nil:
		g.w(" *(")
		g.lvalue(n.Expr)
		g.w(") = %s(", g.typ(n.Expr.Operand.Type))
		g.convert(n.Expr, op.Type)
	default:
		g.w("{ p := ")
		g.lvalue(n.Expr)
		g.w("; *p = %s(%s(*p)", g.typ(n.Expr.Operand.Type), g.typ(op.Type))
	}
	switch n.Token.Rune {
	case c99.ANDASSIGN:
		g.w("&")
	case c99.ADDASSIGN:
		g.w("+")
	case c99.SUBASSIGN:
		g.w("-")
	case c99.MULASSIGN:
		g.w("*")
	case c99.DIVASSIGN:
		g.w("/")
	case c99.ORASSIGN:
		g.w("|")
	case c99.RSHASSIGN:
		g.w(">>")
		op.Type = c99.UInt
	case c99.XORASSIGN:
		g.w("^")
	case c99.MODASSIGN:
		g.w("%%")
	case c99.LSHASSIGN:
		g.w("<<")
		op.Type = c99.UInt
	default:
		todo("", g.position0(n), c99.TokSrc(n.Token))
	}
	if n.Expr.Operand.Bits != 0 {
		g.w("(")
	}
	g.convert(n.Expr2, op.Type)
	switch {
	case lhs.Bits != 0:
		g.w("))<<%d&%#x) }", lhs.Bitoff, mask)
	case n.Expr.Declarator != nil:
		g.w(")")
	default:
		g.w(")}")
	}
}

func (g *gen) assignmentValue(n *c99.Expr) {
	switch op := n.Expr.Operand; {
	case op.Bits != 0:
		g.w("%s(&", g.registerHelper("set%db", "=", g.typ(op.Type), g.typ(op.PackedType), g.model.Sizeof(op.Type)*8, op.Bits, op.Bitoff))
		g.value(n.Expr, true)
		g.w(", ")
		g.convert(n.Expr2, n.Operand.Type)
		g.w(")")
	default:
		g.w("%s(", g.registerHelper("set%d", "", g.typ(op.Type)))
		g.lvalue(n.Expr)
		g.w(", ")
		g.convert(n.Expr2, n.Operand.Type)
		g.w(")")
	}
}

func (g *gen) binop(n *c99.Expr) {
	l, r := n.Expr.Operand.Type, n.Expr2.Operand.Type
	if l.IsArithmeticType() && r.IsArithmeticType() {
		op, _ := c99.UsualArithmeticConversions(g.model, n.Expr.Operand, n.Expr2.Operand)
		l, r = op.Type, op.Type
	}
	switch {
	case
		l.Kind() == c99.Ptr && n.Operand.Type.IsArithmeticType(),
		n.Operand.Type.Kind() == c99.Ptr && l.IsArithmeticType():

		g.convert(n.Expr, n.Operand.Type)
	default:
		g.convert(n.Expr, l)
	}
	g.w(" %s ", c99.TokSrc(n.Token))
	switch {
	case
		r.Kind() == c99.Ptr && n.Operand.Type.IsArithmeticType(),
		n.Operand.Type.Kind() == c99.Ptr && r.IsArithmeticType():

		g.convert(n.Expr2, n.Operand.Type)
	default:
		g.convert(n.Expr2, r)
	}
}

func (g *gen) relop(n *c99.Expr) {
	g.needBool2int++
	g.w(" bool2int(")
	l, r := n.Expr.Operand.Type, n.Expr2.Operand.Type
	if l.IsArithmeticType() && r.IsArithmeticType() {
		op, _ := c99.UsualArithmeticConversions(g.model, n.Expr.Operand, n.Expr2.Operand)
		l, r = op.Type, op.Type
	}
	switch {
	case l.Kind() == c99.Ptr || r.Kind() == c99.Ptr:
		g.value(n.Expr, false)
		g.w(" %s ", c99.TokSrc(n.Token))
		g.value(n.Expr2, false)
		g.w(")")
	default:
		g.convert(n.Expr, l)
		g.w(" %s ", c99.TokSrc(n.Token))
		g.convert(n.Expr2, r)
		g.w(")")
	}
}

func (g *gen) convert(n *c99.Expr, t c99.Type) {
	if n.Case == c99.ExprPExprList && isSingleExpression(n.ExprList) {
		g.convert(n.ExprList.Expr, t)
		return
	}

	if t.Kind() == c99.Function {
		ft := c99.UnderlyingType(t)
		switch n.Case {
		case c99.ExprIdent: // IDENTIFIER
			d := g.normalizeDeclarator(n.Declarator)
			g.enqueue(d)
			dt := c99.UnderlyingType(d.Type)
			if dt.Equal(ft) {
				g.w("%s", g.mangleDeclarator(d))
				return
			}

			if c99.UnderlyingType(n.Operand.Type).Equal(&c99.PointerType{Item: ft}) {
				switch {
				case d.Type.Kind() == c99.Ptr:
					g.w("%s(%s)", g.registerHelper("fn%d", g.typ(ft)), g.mangleDeclarator(n.Declarator))
				default:
					g.w("%s", g.mangleDeclarator(n.Declarator))
				}
				return
			}

			todo("", g.position0(n))
		case c99.ExprCast: // '(' TypeName ')' Expr
			if d := g.normalizeDeclarator(n.Expr.Declarator); d != nil {
				g.enqueue(d)
				if d.Type.Equal(t) {
					g.w("%s", g.mangleDeclarator(d))
					return
				}

				g.w("%s(%s(%s))", g.registerHelper("fn%d", g.typ(t)), g.registerHelper("fp%d", g.typ(d.Type)), g.mangleDeclarator(d))
				return
			}

			g.w("%s(", g.registerHelper("fn%d", g.typ(ft)))
			g.value(n, false)
			g.w(")")
		default:
			g.w("%s(", g.registerHelper("fn%d", g.typ(t)))
			g.value(n, false)
			g.w(")")
		}
		return
	}

	if isVaList(n.Operand.Type) && !isVaList(t) {
		g.w("%sVA%s(&", crt, g.typ(c99.UnderlyingType(t)))
		g.value(n, false)
		g.w(")")
		return
	}

	if t.Kind() == c99.Ptr {
		switch {
		case n.Operand.Value != nil && isVaList(t):
			g.w("%s", ap)
		case n.Operand.Type.Kind() == c99.Ptr:
			g.value(n, false)
		case isVaList(t):
			switch x := n.Operand.Value.(type) {
			case *ir.Int64Value:
				if x.Value == 1 {
					g.w("%s", ap)
					return
				}
			default:
				todo("%v, %T, %v %v -> %v", g.position0(n), x, n.Case, n.Operand, t)
			}
			todo("", g.position0(n))
		case n.Operand.Type.IsIntegerType():
			if n.Operand.Value != nil && g.voidCanIgnore(n) {
				n.Operand.Type = t
				g.constant(n)
				return
			}

			g.w(" uintptr(")
			g.value(n, false)
			g.w(")")
		default:
			todo("%v: %v -> %v, %T, %v", g.position0(n), n.Operand, t, t, c99.UnderlyingType(t))
		}
		return
	}

	if n.Operand.Type.Equal(t) {
		switch {
		case n.Operand.Value != nil && g.voidCanIgnore(n):
			g.w(" %s(", g.typ(t))
			g.constant(n)
			g.w(")")
		default:
			g.value(n, false)
		}
		return
	}

	if c99.UnderlyingType(t).IsArithmeticType() {
		g.w(" %s(", g.typ(t))
		switch {
		case n.Operand.Value != nil && g.voidCanIgnore(n):
			n.Operand.Type = t
			g.constant(n)
		default:
			g.value(n, false)
		}
		g.w(")")
		return
	}

	todo("%v: %v -> %v, %T, %v", g.position0(n), n.Operand, t, t, c99.UnderlyingType(t))
}

func (g *gen) int64ToUintptr(n int64) uint64 {
	switch g.model[c99.Ptr].Size {
	case 4:
		return uint64(uint32(n))
	case 8:
		return uint64(n)
	}
	panic("unreachable")
}
