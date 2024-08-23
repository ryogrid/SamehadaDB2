package index

import (
	"encoding/binary"
	"github.com/ryogrid/SamehadaDB/lib/common"
	"github.com/ryogrid/SamehadaDB/lib/container/btree"
	"github.com/ryogrid/SamehadaDB/lib/recovery"
	"github.com/ryogrid/SamehadaDB/lib/samehada/samehada_util"
	"github.com/ryogrid/SamehadaDB/lib/storage/buffer"
	"github.com/ryogrid/SamehadaDB/lib/storage/page"
	"github.com/ryogrid/SamehadaDB/lib/storage/table/schema"
	"github.com/ryogrid/SamehadaDB/lib/storage/tuple"
	"github.com/ryogrid/SamehadaDB/lib/types"
	blink_tree "github.com/ryogrid/bltree-go-for-embedding"
	"math"
	"sync"
)

type BtreeIndexIterator struct {
	itr     *blink_tree.BLTreeItr
	valType types.TypeID
}

func NewBtreeIndexIterator(itr *blink_tree.BLTreeItr, valType types.TypeID) *BtreeIndexIterator {
	return &BtreeIndexIterator{itr, valType}
}

func (btreeItr *BtreeIndexIterator) Next() (done bool, err error, key *types.Value, rid *page.RID) {
	ok, keyBytes, packedRID := btreeItr.itr.Next()
	if ok == false {
		return true, nil, nil, &page.RID{-1, 0}
	}
	uintRID := binary.BigEndian.Uint64(packedRID)
	unpackedRID := samehada_util.UnpackUint64toRID(uintRID)

	// attach isNull flag and length of value due to these info is not stored in BLTree
	keyLen := uint16(len(keyBytes) - 8) // 8 is length of packedRID
	keyLenBuf := make([]byte, 2)
	binary.LittleEndian.PutUint16(keyLenBuf, keyLen)
	newKeyBytes := make([]byte, 0, len(keyBytes)+3)
	newKeyBytes = append(newKeyBytes, 0)
	newKeyBytes = append(newKeyBytes, keyLenBuf...)
	newKeyBytes = append(newKeyBytes, keyBytes...)

	//decodedKey := samehada_util.ExtractOrgKeyFromDicOrderComparableEncodedVarchar(types.NewValueFromBytes(newKeyBytes, types.Varchar), btreeItr.valType)
	//decodedKey := samehada_util.ExtractOrgKeyFromDicOrderComparableEncodedVarchar(samehada_util.GetPonterOfValue(types.NewVarchar(string(newKeyBytes))), btreeItr.valType)
	decodedKey := samehada_util.ExtractOrgKeyFromDicOrderComparableEncodedBytes(newKeyBytes, btreeItr.valType)
	return false, nil, decodedKey, &unpackedRID
}

type BTreeIndex struct {
	container *blink_tree.BLTree
	metadata  *IndexMetadata
	// idx of target column on table
	col_idx     uint32
	log_manager *recovery.LogManager
	// UpdateEntry only get Write lock
	updateMtx sync.RWMutex
	// for call of Close method ....
	bufMgr *blink_tree.BufMgr
}

func NewBTreeIndex(metadata *IndexMetadata, buffer_pool_manager *buffer.BufferPoolManager, col_idx uint32, log_manager *recovery.LogManager, lastPageZeroId *int32) *BTreeIndex {
	ret := new(BTreeIndex)
	ret.metadata = metadata

	// BTreeIndex uses special technique to support key duplication with SkipList supporting unique key only
	// for the thechnique, key type is fixed to Varchar (comparison is done on dict order as byte array)

	bufMgr := blink_tree.NewBufMgr(12, blink_tree.HASH_TABLE_ENTRY_CHAIN_LEN*common.MaxTxnThreadNum*2, btree.NewParentBufMgrImpl(buffer_pool_manager), lastPageZeroId)
	ret.container = blink_tree.NewBLTree(bufMgr)
	ret.col_idx = col_idx
	ret.updateMtx = sync.RWMutex{}
	ret.log_manager = log_manager
	ret.bufMgr = bufMgr
	return ret
}

func (btidx *BTreeIndex) insertEntryInner(key *tuple.Tuple, rid page.RID, txn interface{}, isNoLock bool) {
	tupleSchema_ := btidx.GetTupleSchema()
	orgKeyVal := key.GetValue(tupleSchema_, btidx.col_idx)

	convedKeyVal := samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgKeyVal, &rid)

	if isNoLock == false {
		btidx.updateMtx.RLock()
		defer btidx.updateMtx.RUnlock()
	}

	packedRID := samehada_util.PackRIDtoUint64(&rid)
	var valBuf [8]byte
	binary.BigEndian.PutUint64(valBuf[:], packedRID)
	btidx.container.InsertKey(convedKeyVal.SerializeOnlyVal(), 0, valBuf, true)
}

func (btidx *BTreeIndex) InsertEntry(key *tuple.Tuple, rid page.RID, txn interface{}) {
	btidx.insertEntryInner(key, rid, txn, false)
}

func (btidx *BTreeIndex) deleteEntryInner(key *tuple.Tuple, rid page.RID, txn interface{}, isNoLock bool) {
	tupleSchema_ := btidx.GetTupleSchema()
	orgKeyVal := key.GetValue(tupleSchema_, btidx.col_idx)

	convedKeyVal := samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgKeyVal, &rid)

	if isNoLock == false {
		btidx.updateMtx.RLock()
		defer btidx.updateMtx.RUnlock()
	}
	btidx.container.DeleteKey(convedKeyVal.SerializeOnlyVal(), 0)
}

func (btidx *BTreeIndex) DeleteEntry(key *tuple.Tuple, rid page.RID, txn interface{}) {
	btidx.deleteEntryInner(key, rid, txn, false)
}

func (btidx *BTreeIndex) ScanKey(key *tuple.Tuple, txn interface{}) []page.RID {
	tupleSchema_ := btidx.GetTupleSchema()
	orgKeyVal := key.GetValue(tupleSchema_, btidx.col_idx)
	smallestKeyVal := samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgKeyVal, &page.RID{0, 0})
	biggestKeyVal := samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgKeyVal, &page.RID{math.MaxInt32, math.MaxUint32})

	btidx.updateMtx.RLock()
	// Attention: returned itr's containing keys are string type Value which is constructed with byte arr of concatenated  original key and value
	rangeItr := btidx.container.GetRangeItr(smallestKeyVal.SerializeOnlyVal(), biggestKeyVal.SerializeOnlyVal())

	retArr := make([]page.RID, 0)
	for ok, _, packedRID := rangeItr.Next(); ok; ok, _, packedRID = rangeItr.Next() {
		uintRID := binary.BigEndian.Uint64(packedRID)
		retArr = append(retArr, samehada_util.UnpackUint64toRID(uintRID))
	}
	btidx.updateMtx.RUnlock()

	return retArr
}

func (btidx *BTreeIndex) UpdateEntry(oldKey *tuple.Tuple, oldRID page.RID, newKey *tuple.Tuple, newRID page.RID, txn interface{}) {
	btidx.updateMtx.Lock()
	defer btidx.updateMtx.Unlock()
	btidx.deleteEntryInner(oldKey, oldRID, txn, true)
	btidx.insertEntryInner(newKey, newRID, txn, true)
}

// get iterator which iterates entry in key sorted order
// and iterates specified key range.
// when start_key arg is nil , start point is head of entry list. when end_key, end point is tail of the list
// Attention: returned itr's containing keys are string type Value which is constructed with byte arr of concatenated original key and value
func (btidx *BTreeIndex) GetRangeScanIterator(start_key *tuple.Tuple, end_key *tuple.Tuple, transaction interface{}) IndexRangeScanIterator {
	tupleSchema_ := btidx.GetTupleSchema()
	var smallestKeyVal *types.Value = nil
	if start_key != nil {
		orgStartKeyVal := start_key.GetValue(tupleSchema_, btidx.col_idx)
		smallestKeyVal = samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgStartKeyVal, &page.RID{0, 0})
	}

	var biggestKeyVal *types.Value = nil
	if end_key != nil {
		orgEndKeyVal := end_key.GetValue(tupleSchema_, btidx.col_idx)
		biggestKeyVal = samehada_util.EncodeValueAndRIDToDicOrderComparableVarchar(&orgEndKeyVal, &page.RID{math.MaxInt32, math.MaxUint32})
	}

	btidx.updateMtx.RLock()
	defer btidx.updateMtx.RUnlock()
	var smalledKeyBytes []byte
	var biggestKeyBytes []byte

	if smallestKeyVal != nil {
		smalledKeyBytes = smallestKeyVal.SerializeOnlyVal()
	}
	if biggestKeyVal != nil {
		biggestKeyBytes = biggestKeyVal.SerializeOnlyVal()
	}
	return NewBtreeIndexIterator(btidx.container.GetRangeItr(smalledKeyBytes, biggestKeyBytes), btidx.metadata.tuple_schema.GetColumn(btidx.col_idx).GetType())
}

// Return the metadata object associated with the index
func (btidx *BTreeIndex) GetMetadata() *IndexMetadata { return btidx.metadata }

func (btidx *BTreeIndex) GetIndexColumnCount() uint32 {
	return btidx.metadata.GetIndexColumnCount()
}

func (btidx *BTreeIndex) GetName() *string { return btidx.metadata.GetName() }

func (btidx *BTreeIndex) GetTupleSchema() *schema.Schema {
	return btidx.metadata.GetTupleSchema()
}

func (btidx *BTreeIndex) GetKeyAttrs() []uint32 { return btidx.metadata.GetKeyAttrs() }

func (slidx *BTreeIndex) GetHeaderPageId() types.PageID {
	return types.PageID(slidx.bufMgr.GetMappedPPageIdOfPageZero())
}

// call this at shutdown of the system
// to write out the state and allocated pages of the BLTree container to BPM
func (btidx *BTreeIndex) WriteOutContainerStateToBPM() {
	btidx.bufMgr.Close()
}
