package plans

import (
	"github.com/ryogrid/SamehadaDB/catalog"
	"github.com/ryogrid/SamehadaDB/common"
	"github.com/ryogrid/SamehadaDB/execution/expression"
	"github.com/ryogrid/SamehadaDB/storage/table/schema"
	"math"
)

type IndexJoinPlanNode struct {
	*AbstractPlanNode
	onPredicate    expression.Expression
	rigthTableOID  uint32
	rightOutSchema *schema.Schema
}

func NewIndexJoinPlanNode(leftChild Plan, leftKeys []expression.Expression, rightOutSchema *schema.Schema, rightTblOID uint32, rightKeys []expression.Expression) *IndexJoinPlanNode {
	if leftKeys == nil || rightKeys == nil {
		panic("NewIndexJoinPlanNode needs keys info.")
	}
	if len(leftKeys) != 1 || len(rightKeys) != 1 {
		panic("NewIndexJoinPlanNode supports only one key for left and right now.")
	}

	outputSchema := makeMergedOutputSchema(leftChild.OutputSchema(), rightOutSchema)
	onPredicate := constructOnExpressionFromKeysInfo(leftKeys, rightKeys)

	return &IndexJoinPlanNode{&AbstractPlanNode{outputSchema, []Plan{leftChild, nil}}, onPredicate, rightTblOID, rightOutSchema}
}

func (p *IndexJoinPlanNode) GetLeftPlan() Plan {
	common.SH_Assert(len(p.GetChildren()) == 2, "Index joins should have exactly two children plans.")
	return p.GetChildAt(0)
}

func (p *IndexJoinPlanNode) GetRightPlan() Plan {
	panic("IndexJoinPlanNode::GetRightPlan() should not be called.")
}

func (p *IndexJoinPlanNode) GetType() PlanType { return IndexJoin }

func (p *IndexJoinPlanNode) OnPredicate() expression.Expression { return p.onPredicate }

// can not be used
func (p *IndexJoinPlanNode) GetTableOID() uint32 {
	return math.MaxUint32
}

func (p *IndexJoinPlanNode) GetRightTableOID() uint32 {
	return p.rigthTableOID
}

func (p *IndexJoinPlanNode) getRightTableRows(c *catalog.Catalog) uint64 {
	tm := c.GetTableByOID(p.rigthTableOID)
	return tm.GetStatistics().Rows()
}

func (p *IndexJoinPlanNode) AccessRowCount(c *catalog.Catalog) uint64 {
	return p.getRightTableRows(c) * 3
}

func (p *IndexJoinPlanNode) EmitRowCount(c *catalog.Catalog) uint64 {
	return uint64(math.Min(float64(p.GetLeftPlan().EmitRowCount(c)), float64(p.getRightTableRows(c))))
}
