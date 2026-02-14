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
