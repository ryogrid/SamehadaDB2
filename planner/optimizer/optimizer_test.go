package optimizer

import (
	"fmt"
	"github.com/ryogrid/SamehadaDB/catalog"
	"github.com/ryogrid/SamehadaDB/common"
	"github.com/ryogrid/SamehadaDB/execution/executors"
	"github.com/ryogrid/SamehadaDB/parser"
	"github.com/ryogrid/SamehadaDB/recovery"
	"github.com/ryogrid/SamehadaDB/storage/access"
	"github.com/ryogrid/SamehadaDB/storage/buffer"
	"github.com/ryogrid/SamehadaDB/storage/disk"
	"github.com/ryogrid/SamehadaDB/storage/index/index_constants"
	"github.com/ryogrid/SamehadaDB/storage/table/column"
	"github.com/ryogrid/SamehadaDB/storage/table/schema"
	"github.com/ryogrid/SamehadaDB/storage/tuple"
	testingpkg "github.com/ryogrid/SamehadaDB/testing/testing_assert"
	"github.com/ryogrid/SamehadaDB/types"
	"strconv"
	"testing"
)

type ColumnMeta struct {
	Name       string
	ColumnType types.TypeID
	IdxKind    index_constants.IndexKind
}

type ColValGenFunc func(idx int) interface{}

type SetupTableMeta struct {
	TableName      string
	EntriesNum     int64
	Columns        []*ColumnMeta
	ColValGenFuncs []ColValGenFunc
}

func SetupTableWithMetadata(exec_ctx *executors.ExecutorContext, tableMeta *SetupTableMeta) {
	c := exec_ctx.GetCatalog()
	txn := exec_ctx.GetTransaction()

	cols := make([]*column.Column, 0)
	for _, colMeta := range tableMeta.Columns {
		if colMeta.IdxKind != index_constants.INDEX_KIND_INVALID {
			col := column.NewColumn(colMeta.Name, colMeta.ColumnType, true, colMeta.IdxKind, types.PageID(-1), nil)
			cols = append(cols, col)
		} else {
			col := column.NewColumn(colMeta.Name, colMeta.ColumnType, false, colMeta.IdxKind, types.PageID(-1), nil)
			cols = append(cols, col)
		}
	}
	schema_ := schema.NewSchema(cols)
	tm := c.CreateTable(tableMeta.TableName, schema_, txn)

	for ii := 0; ii < int(tableMeta.EntriesNum); ii++ {
		vals := make([]types.Value, 0)
		for jj, genFunc := range tableMeta.ColValGenFuncs {
			vals = append(vals, types.NewValue(genFunc(jj)))
		}
		tuple_ := tuple.NewTupleFromSchema(vals, schema_)
		rid, _ := tm.Table().InsertTuple(tuple_, false, txn, tm.OID())
		for jj, colMeta := range tableMeta.Columns {
			if colMeta.IdxKind != index_constants.INDEX_KIND_INVALID {
				tm.GetIndex(jj).InsertEntry(tuple_, *rid, txn)
			}
		}
	}
}

func setupTablesAndStatisticsDataForTesting(exec_ctx *executors.ExecutorContext) {
	/*	c := exec_ctx.GetCatalog()
		txn := exec_ctx.GetTransaction()
		//bpm := exec_ctx.GetBufferPoolManager()

		colC1 := column.NewColumn("c1", types.Integer, true, index_constants.INDEX_KIND_SKIP_LIST, types.PageID(-1), nil)
		colC2 := column.NewColumn("c2", types.Varchar, true, index_constants.INDEX_KIND_SKIP_LIST, types.PageID(-1), nil)
		colC3 := column.NewColumn("c3", types.Float, true, index_constants.INDEX_KIND_SKIP_LIST, types.PageID(-1), nil)
		schemaSc1 := schema.NewSchema([]*column.Column{colC1, colC2, colC3})
		tmSc1 := c.CreateTable("Sc1", schemaSc1, txn)

		idxC1 := tmSc1.GetIndex(0)
		idxC2 := tmSc1.GetIndex(1)
		idxC3 := tmSc1.GetIndex(2)
		for ii := 0; ii < 100; ii++ {
			valCol1 := types.NewInteger(int32(ii))
			valCol2 := types.NewVarchar("c2-" + strconv.Itoa(ii))
			valCol3 := types.NewFloat(float32(ii) + 9.9)
			tuple_ := tuple.NewTupleFromSchema([]types.Value{valCol1, valCol2, valCol3}, schemaSc1)
			rid, _ := tmSc1.Table().InsertTuple(tuple_, false, txn, tmSc1.OID())

			idxC1.InsertEntry(tuple_, *rid, txn)
			idxC2.InsertEntry(tuple_, *rid, txn)
			idxC3.InsertEntry(tuple_, *rid, txn)
		}*/

	Sc1Meta := &SetupTableMeta{
		"Sc1",
		100,
		[]*ColumnMeta{
			{"c1", types.Integer, index_constants.INDEX_KIND_SKIP_LIST},
			{"c2", types.Varchar, index_constants.INDEX_KIND_SKIP_LIST},
			{"c3", types.Float, index_constants.INDEX_KIND_SKIP_LIST},
		},
		[]ColValGenFunc{
			func(idx int) interface{} { return int32(idx) },
			func(idx int) interface{} { return "c2-" + strconv.Itoa(idx) },
			func(idx int) interface{} { return float32(idx) + 9.9 },
		},
	}
	SetupTableWithMetadata(exec_ctx, Sc1Meta)

	// TODO: (SDB) [OPT] not implemented yet (setupTablesAndStatisticsDataForTesting)
	/*
		     prefix_ = "optimizer_test-" + RandomString();
			 rs_->CreateTable(ctx,
							  Schema("Sc1", {Column("c1", ValueType::kInt64),
											 Column("c2", ValueType::kVarChar),
											 Column("c3", ValueType::kDouble)}));
		     for (int i = 0; i < 100; ++i) {
		           tbl.Insert(ctx.txn_,
		                      Row({Value(i), Value("c2-" + std::to_string(i)),
		                           Value(i + 9.9)}));
		     }

			 rs_->CreateTable(ctx,
							  Schema("Sc2", {Column("d1", ValueType::kInt64),
											 Column("d2", ValueType::kDouble),
											 Column("d3", ValueType::kVarChar),
											 Column("d4", ValueType::kInt64)}));
		     for (int i = 0; i < 200; ++i) {
		           tbl.Insert(ctx.txn_,
		                      Row({Value(i), Value(i + 0.2),
		                           Value("d3-" + std::to_string(i % 10)), Value(16)}));
		     }


			 rs_->CreateTable(ctx,
							  Schema("Sc3", {Column("e1", ValueType::kInt64),
											 Column("e2", ValueType::kDouble)}));
		     for (int i = 20; 0 < i; --i) {
		           tbl.Insert(ctx.txn_, Row({Value(i), Value(i + 53.4)}));
		     }

			 rs_->CreateTable(ctx,
							  Schema("Sc4", {Column("c1", ValueType::kInt64),
											 Column("c2", ValueType::kVarChar)}));
		     for (int i = 100; 0 < i; --i) {
		           tbl.Insert(ctx.txn_, Row({Value(i), Value(std::to_string(i % 4))}));
		     }

		     IndexSchema idx_sc("SampleIndex", {1, 2});
		     rs_->CreateIndex(ctx, "Sc1", IndexSchema("KeyIdx", {1, 2}));
		     rs_->CreateIndex(ctx, "Sc1", IndexSchema("Sc1PK", {0}));
		     rs_->CreateIndex(ctx, "Sc2", IndexSchema("Sc2PK", {0}));
		     rs_->CreateIndex(ctx, "Sc2",IndexSchema("NameIdx", {2, 3}, {0, 1}, IndexMode::kNonUnique));
		     rs_->CreateIndex(ctx, "Sc4", IndexSchema("Sc4_IDX", {1}, {}, IndexMode::kNonUnique));
		     ctx.txn_.PreCommit();

		     auto stat_tx = rs_->BeginContext();
		     rs_->RefreshStatistics(stat_tx, "Sc1");
		     rs_->RefreshStatistics(stat_tx, "Sc2");
		     rs_->RefreshStatistics(stat_tx, "Sc3");
		     rs_->RefreshStatistics(stat_tx, "Sc4");
		     stat_tx.PreCommit();
	*/

	// dummy code
	tm1 := c.GetTableByName("Sc1")
	stat1 := tm1.GetStatistics()
	stat1.Update(tm1, txn)

	tm2 := c.GetTableByName("Sc2")
	stat2 := tm2.GetStatistics()
	stat2.Update(tm2, txn)

	tm3 := c.GetTableByName("Sc1")
	stat3 := tm3.GetStatistics()
	stat3.Update(tm3, txn)

	tm4 := c.GetTableByName("Sc1")
	stat4 := tm4.GetStatistics()
	stat4.Update(tm4, txn)

}

func TestSimplePlanOptimization(t *testing.T) {
	diskManager := disk.NewDiskManagerTest()
	defer diskManager.ShutDown()
	log_mgr := recovery.NewLogManager(&diskManager)
	log_mgr.ActivateLogging()
	testingpkg.Assert(t, log_mgr.IsEnabledLogging(), "")
	fmt.Println("System logging is active.")
	bpm := buffer.NewBufferPoolManager(common.BufferPoolMaxFrameNumForTest, diskManager, log_mgr) //, recovery.NewLogManager(diskManager), access.NewLockManager(access.REGULAR, access.PREVENTION))
	txn_mgr := access.NewTransactionManager(access.NewLockManager(access.REGULAR, access.DETECTION), log_mgr)

	txn := txn_mgr.Begin(nil)
	c := catalog.BootstrapCatalog(bpm, log_mgr, access.NewLockManager(access.REGULAR, access.PREVENTION), txn)
	exec_ctx := executors.NewExecutorContext(c, bpm, txn)

	setupTablesAndStatisticsDataForTesting(exec_ctx)
	txn_mgr.Commit(c, txn)

	// TODO: (SDB) [OPT] need to write query for testing BestJoin func (TestSimplePlanOptimization)
	queryStr := "TO BE WRITTEN"
	queryInfo := parser.ProcessSQLStr(&queryStr)

	optimizer := NewSelingerOptimizer(queryInfo, c)
	solution, err := optimizer.Optimize()
	if err != nil {
		fmt.Println(err)
	}
	testingpkg.Assert(t, err == nil, "err != nil")
	fmt.Println(solution)
}

func TestFindBestScans(t *testing.T) {
	diskManager := disk.NewDiskManagerTest()
	defer diskManager.ShutDown()
	log_mgr := recovery.NewLogManager(&diskManager)
	log_mgr.ActivateLogging()
	testingpkg.Assert(t, log_mgr.IsEnabledLogging(), "")
	fmt.Println("System logging is active.")
	bpm := buffer.NewBufferPoolManager(common.BufferPoolMaxFrameNumForTest, diskManager, log_mgr) //, recovery.NewLogManager(diskManager), access.NewLockManager(access.REGULAR, access.PREVENTION))
	txn_mgr := access.NewTransactionManager(access.NewLockManager(access.REGULAR, access.DETECTION), log_mgr)

	txn := txn_mgr.Begin(nil)
	c := catalog.BootstrapCatalog(bpm, log_mgr, access.NewLockManager(access.REGULAR, access.PREVENTION), txn)
	exec_ctx := executors.NewExecutorContext(c, bpm, txn)

	setupTablesAndStatisticsDataForTesting(exec_ctx)
	txn_mgr.Commit(c, txn)

	// TODO: (SDB) [OPT] need to write query for testing BestJoin func (TestFindBestScans)
	queryStr := "TO BE WRITTEN"
	queryInfo := parser.ProcessSQLStr(&queryStr)

	optimalPlans := NewSelingerOptimizer(queryInfo, c).findBestScans()
	testingpkg.Assert(t, len(optimalPlans) == len(queryInfo.JoinTables_), "len(optimalPlans) != len(query.JoinTables_)")
}
