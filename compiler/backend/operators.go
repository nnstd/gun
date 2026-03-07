package backend

import "go/token"

import "github.com/nnstd/gun/compiler/hir"

// mapBinaryOp converts an HIR binary operator to the jsvalue helper function name.
func mapBinaryOpToJSValue(op hir.BinaryOp) string {
	switch op {
	case hir.OpAdd:
		return "Add"
	case hir.OpSub:
		return "Sub"
	case hir.OpMul:
		return "Mul"
	case hir.OpDiv:
		return "Div"
	case hir.OpMod:
		return "Mod"
	case hir.OpEq:
		return "Eq"
	case hir.OpNEq:
		return "NEq"
	case hir.OpEqLoose:
		return "EqLoose"
	case hir.OpNEqLoose:
		return "NEqLoose"
	case hir.OpLt:
		return "Lt"
	case hir.OpGt:
		return "Gt"
	case hir.OpLtE:
		return "LtE"
	case hir.OpGtE:
		return "GtE"
	case hir.OpAnd:
		return "And"
	case hir.OpOr:
		return "Or"
	case hir.OpNullish:
		return "Nullish"
	case hir.OpBitAnd:
		return "BitAnd"
	case hir.OpBitOr:
		return "BitOr"
	case hir.OpBitXor:
		return "BitXor"
	case hir.OpShl:
		return "Shl"
	case hir.OpShr:
		return "Shr"
	case hir.OpUShr:
		return "UShr"
	default:
		return "Add"
	}
}

// mapUnaryOpToJSValue converts an HIR unary operator to the jsvalue helper function name.
func mapUnaryOpToJSValue(op hir.UnaryOp) string {
	switch op {
	case hir.OpNot:
		return "Not"
	case hir.OpNeg:
		return "Neg"
	case hir.OpBitNot:
		return "BitNot"
	case hir.OpTypeof:
		return "TypeOf"
	default:
		return ""
	}
}

// mapAssignOpToJSValue converts an HIR augmented assignment to the jsvalue helper name.
func mapAssignOpToJSValue(op hir.AssignOp) string {
	switch op {
	case hir.OpAddAssign:
		return "Add"
	case hir.OpSubAssign:
		return "Sub"
	case hir.OpMulAssign:
		return "Mul"
	case hir.OpDivAssign:
		return "Div"
	case hir.OpModAssign:
		return "Mod"
	case hir.OpBitAndAssign:
		return "BitAnd"
	case hir.OpBitOrAssign:
		return "BitOr"
	case hir.OpBitXorAssign:
		return "BitXor"
	case hir.OpShlAssign:
		return "Shl"
	case hir.OpShrAssign:
		return "Shr"
	case hir.OpUShrAssign:
		return "UShr"
	case hir.OpNullishAssign:
		return "Nullish"
	case hir.OpAndAssign:
		return "And"
	case hir.OpOrAssign:
		return "Or"
	default:
		return "Add"
	}
}

// mapBinaryOpToToken converts an HIR binary operator to a go/token for native Go expressions.
func mapBinaryOpToToken(op hir.BinaryOp) token.Token {
	switch op {
	case hir.OpAdd:
		return token.ADD
	case hir.OpSub:
		return token.SUB
	case hir.OpMul:
		return token.MUL
	case hir.OpDiv:
		return token.QUO
	case hir.OpMod:
		return token.REM
	case hir.OpEq, hir.OpEqLoose:
		return token.EQL
	case hir.OpNEq, hir.OpNEqLoose:
		return token.NEQ
	case hir.OpLt:
		return token.LSS
	case hir.OpGt:
		return token.GTR
	case hir.OpLtE:
		return token.LEQ
	case hir.OpGtE:
		return token.GEQ
	case hir.OpAnd:
		return token.LAND
	case hir.OpOr:
		return token.LOR
	case hir.OpBitAnd:
		return token.AND
	case hir.OpBitOr:
		return token.OR
	case hir.OpBitXor:
		return token.XOR
	case hir.OpShl:
		return token.SHL
	case hir.OpShr, hir.OpUShr:
		return token.SHR
	default:
		return token.ADD
	}
}
