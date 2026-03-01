package compiler

import "go/token"

func mapBinaryOp(op string) token.Token {
	switch op {
	case "+":
		return token.ADD
	case "-":
		return token.SUB
	case "*":
		return token.MUL
	case "/":
		return token.QUO
	case "%":
		return token.REM
	case "==", "===":
		return token.EQL
	case "!=", "!==":
		return token.NEQ
	case "<":
		return token.LSS
	case ">":
		return token.GTR
	case "<=":
		return token.LEQ
	case ">=":
		return token.GEQ
	case "&&":
		return token.LAND
	case "||":
		return token.LOR
	case "&":
		return token.AND
	case "|":
		return token.OR
	case "^":
		return token.XOR
	case "<<":
		return token.SHL
	case ">>", ">>>":
		return token.SHR
	default:
		return token.ADD
	}
}

func mapAugmentedOp(op string) token.Token {
	switch op {
	case "+=":
		return token.ADD_ASSIGN
	case "-=":
		return token.SUB_ASSIGN
	case "*=":
		return token.MUL_ASSIGN
	case "/=":
		return token.QUO_ASSIGN
	case "%=":
		return token.REM_ASSIGN
	case "&=":
		return token.AND_ASSIGN
	case "|=":
		return token.OR_ASSIGN
	case "^=":
		return token.XOR_ASSIGN
	case "<<=":
		return token.SHL_ASSIGN
	case ">>=":
		return token.SHR_ASSIGN
	default:
		return token.ADD_ASSIGN
	}
}




// jsvalueOpName maps a Go token operator to the corresponding jsvalue helper function name.
// Returns empty string if no mapping exists.
func jsvalueOpName(op token.Token) string {
	switch op {
	// Arithmetic
	case token.ADD:
		return "Add"
	case token.SUB:
		return "Sub"
	case token.MUL:
		return "Mul"
	case token.QUO:
		return "Div"
	case token.REM:
		return "Mod"
	// Comparison
	case token.EQL:
		return "Eq"
	case token.NEQ:
		return "NEq"
	case token.LSS:
		return "Lt"
	case token.GTR:
		return "Gt"
	case token.LEQ:
		return "LtE"
	case token.GEQ:
		return "GtE"
	// Bitwise
	case token.AND:
		return "BitAnd"
	case token.OR:
		return "BitOr"
	case token.XOR:
		return "BitXor"
	case token.SHL:
		return "Shl"
	case token.SHR:
		return "Shr"
	default:
		return ""
	}
}

// isAugmentedAssignOp returns true if the operator is an augmented assignment.
func isAugmentedAssignOp(op string) bool {
	switch op {
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>=":
		return true
	}
	return false
}

// augmentedOpToJSValueHelper maps augmented assignment operators to jsvalue helper names.
func augmentedOpToJSValueHelper(op string) string {
	switch op {
	case "+=":
		return "Add"
	case "-=":
		return "Sub"
	case "*=":
		return "Mul"
	case "/=":
		return "Div"
	case "%=":
		return "Mod"
	case "&=":
		return "BitAnd"
	case "|=":
		return "BitOr"
	case "^=":
		return "BitXor"
	case "<<=":
		return "Shl"
	case ">>=":
		return "Shr"
	case ">>>=":
		return "UShr"
	}
	return "Add"
}

