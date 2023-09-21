package expression

import (
	"github.com/ryogrid/SamehadaDB/types"
	"math"
	"strconv"
)

// get string of "Reverse Polish Notation" style
func PrintExpTree(node interface{}) string {
	retStr := ""

	childTraverse := func(exp Expression) string {
		var idx uint32 = 0
		var tmpStr string = ""
		for exp.GetChildAt(idx) != nil && idx < math.MaxInt32 {
			child := exp.GetChildAt(idx)
			tmpStr += PrintExpTree(child)
			idx++
		}
		return tmpStr
	}

	switch typedNode := node.(type) {
	case *Comparison:
		retStr += childTraverse(typedNode)
		switch typedNode.comparisonType {
		case Equal:
			retStr += "= "
		case NotEqual:
			retStr += "!= "
		case GreaterThan: // A > B
			retStr += "> "
		case GreaterThanOrEqual: // A >= B
			retStr += ">= "
		case LessThan: // A < B
			retStr += "< "
		case LessThanOrEqual: // A <= B
			retStr += "<= "
		default:
			panic("illegal comparisonType!")
		}
		return retStr
	case *LogicalOp:
		retStr += childTraverse(typedNode)
		switch typedNode.logicalOpType {
		case AND:
			retStr += "AND "
		case OR:
			retStr += "OR "
		case NOT:
			retStr += "NOT "
		default:
			panic("illegal logicalOpType!")
		}
		return retStr
	case *AggregateValueExpression:
		panic("AggregateValueExpression is not implemented yet!")
	case *ConstantValue:
		return typedNode.value.ToString() + " "
	case *ColumnValue:
		return "colIndex:" + strconv.Itoa(int(typedNode.GetColIndex())) + " "
	case *types.Value:
		return typedNode.ToString() + ""
	default:
		panic("illegal type expression object is passed!")
	}
}
