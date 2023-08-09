// this code is from https://github.com/brunocalza/go-bustub
// there is license and copyright notice in licenses/go-bustub dir

package plans

import (
	"github.com/ryogrid/SamehadaDB/catalog"
	"github.com/ryogrid/SamehadaDB/execution/expression"
	"github.com/ryogrid/SamehadaDB/storage/table/schema"
)

/**
 * PointScanWithIndexPlanNode use hash index to filter rows matches predicate.
 */
type PointScanWithIndexPlanNode struct {
	*AbstractPlanNode
	predicate *expression.Comparison
	tableOID  uint32
}

func NewPointScanWithIndexPlanNode(schema *schema.Schema, predicate *expression.Comparison, tableOID uint32) Plan {
	return &PointScanWithIndexPlanNode{&AbstractPlanNode{schema, nil}, predicate, tableOID}
}

func (p *PointScanWithIndexPlanNode) GetPredicate() *expression.Comparison {
	return p.predicate
}

func (p *PointScanWithIndexPlanNode) GetTableOID() uint32 {
	return p.tableOID
}

func (p *PointScanWithIndexPlanNode) GetType() PlanType {
	return IndexPointScan
}

func (p *PointScanWithIndexPlanNode) EmitRowCount(c *catalog.Catalog) uint64 {
	// TODO: (SDB) [OPT] not implemented yet (RangeScanWithIndexPlanNode::EmitRowCount)
	// TODO: (SDB) [OPT] need to design or decide estimation logic (PointScanWithIndexPlanNode::EmitRowCount)

	return 1
}

func (p *PointScanWithIndexPlanNode) AccessRowCount(c *catalog.Catalog) uint64 {
	return p.EmitRowCount(c)
}
