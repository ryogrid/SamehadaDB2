// this code is from https://github.com/brunocalza/go-bustub
// there is license and copyright notice in licenses/go-bustub dir

package buffer

import (
	//"github.com/sasha-s/go-deadlock"
	"fmt"
	"github.com/ryogrid/SamehadaDB/common"
	"github.com/ryogrid/SamehadaDB/recovery"
	"github.com/ryogrid/SamehadaDB/storage/disk"
	"github.com/ryogrid/SamehadaDB/storage/page"
	"github.com/ryogrid/SamehadaDB/types"
	"sync"
)

// BufferPoolManager represents the buffer pool manager
type BufferPoolManager struct {
	diskManager disk.DiskManager
	pages       []*page.Page // index is FrameID
	replacer    *ClockReplacer
	freeList    []FrameID
	pageTable   map[types.PageID]FrameID
	log_manager *recovery.LogManager
	mutex       *sync.Mutex
	//// when using go-dedlock package
	//mutex *deadlock.Mutex
}

// FetchPage fetches the requested page from the buffer pool.
func (b *BufferPoolManager) FetchPage(pageID types.PageID) *page.Page {
	// if it is on buffer pool return it
	//b.mutex.WLock()
	b.mutex.Lock()
	if frameID, ok := b.pageTable[pageID]; ok {
		pg := b.pages[frameID]
		pg.IncPinCount()
		(*b.replacer).Pin(frameID)
		b.mutex.Unlock()
		if common.EnableDebug {
			common.ShPrintf(common.DEBUG_INFO, "FetchPage: PageId=%d PinCount=%d\n", pg.GetPageId(), pg.PinCount())
		}
		return pg
	}

	//b.mutex.WUnlock()
	// get the id from free list or from replacer
	frameID, isFromFreeList := b.getFrameID()
	//b.mutex.WLock()
	if frameID == nil {
		b.mutex.Unlock()
		return nil
	}

	if !isFromFreeList {
		// remove page from current frame
		//b.mutex.WLock()
		currentPage := b.pages[*frameID]
		//b.mutex.WUnlock()
		//common.SH_Assert(currentPage.PinCount() >= 0, "BPM::FetchPage Victim page's pin count is not zero!!!")
		if currentPage != nil {
			fmt.Printf("BPM::FetchPage Cache out occurs! pageId:%d requested pageId:%d\n", currentPage.GetPageId(), pageID)
			if currentPage.IsDirty() {
				b.log_manager.Flush()
				currentPage.WLatch()
				data := *currentPage.Data()
				b.diskManager.WritePage(currentPage.GetPageId(), data[:])
				currentPage.WUnlatch()
			}
			//b.mutex.WLock()
			if common.EnableDebug {
				common.ShPrintf(common.DEBUG_INFO, "FetchPage: page=%d is removed from pageTable.\n", currentPage.GetPageId())
			}
			delete(b.pageTable, currentPage.GetPageId())
			//b.mutex.WUnlock()
		}
		//b.mutex.WUnlock()
	}

	//b.mutex.WLock()
	data := make([]byte, common.PageSize)
	fmt.Printf("BPM::FetchPage Cache in occurs! requested pageId:%d\n", pageID)
	err := b.diskManager.ReadPage(pageID, data)
	if err != nil {
		fmt.Println(err)
		panic("ReadPage returned error!")
		//return nil
	}
	var pageData [common.PageSize]byte
	copy(pageData[:], data)
	pg := page.New(pageID, false, &pageData)
	b.pageTable[pageID] = *frameID
	b.pages[*frameID] = pg
	b.mutex.Unlock()

	if common.EnableDebug {
		common.ShPrintf(common.DEBUG_INFO, "FetchPage: PageId=%d PinCount=%d\n", pg.GetPageId(), pg.PinCount())
	}
	return pg
}

// ATTENTION: when Unpin a page which has pageID arg as self ID, caller thread must have WLatch of the page
// UnpinPage unpins the target page from the buffer pool.
func (b *BufferPoolManager) UnpinPage(pageID types.PageID, isDirty bool) error {

	b.mutex.Lock()
	//b.mutex.RLock()
	if frameID, ok := b.pageTable[pageID]; ok {
		pg := b.pages[frameID]
		//b.mutex.RUnlock()
		pg.DecPinCount()

		if pg.PinCount() < 0 {
			panic("pin coint is less than 0!")
		}

		if pg.PinCount() <= 0 {
			(*b.replacer).Unpin(frameID)
		}

		if pg.IsDirty() || isDirty {
			pg.SetIsDirty(true)
		} else {
			pg.SetIsDirty(false)
		}
		b.mutex.Unlock()

		if common.EnableDebug {
			common.ShPrintf(common.DEBUG_INFO, "UnpinPage: PageId=%d PinCount=%d\n", pg.GetPageId(), pg.PinCount())
		}
		return nil
	}
	b.mutex.Unlock()
	//b.mutex.RUnlock()

	if common.EnableDebug {
		common.ShPrintf(common.DEBUG_INFO, "UnpinPage: could not find page! PageId=%d\n", pageID)
		panic("could not find page")
	}
	panic("could not find page")
	//return errors.New("could not find page")

}

// Decrement pincount of passed page (this can be used only when a thread has pin of page more than 1
// this get lock of BufferPoolManager
func (b *BufferPoolManager) IncPinOfPage(page_ page.PageIF) {
	b.mutex.Lock()
	page_.IncPinCount()
	b.mutex.Unlock()
}

// Decrement pin count of passed page (this can be used only when a thread has pin of page more than 1
// this get lock of BufferPoolManager but overhead is smaller than UnpinPage
func (b *BufferPoolManager) DecPinOfPage(page_ page.PageIF) {
	b.mutex.Lock()
	page_.DecPinCount()
	b.mutex.Unlock()
}

// FlushPage Flushes the target page to disk.
func (b *BufferPoolManager) FlushPage(pageID types.PageID) bool {
	b.mutex.Lock()
	if frameID, ok := b.pageTable[pageID]; ok {
		pg := b.pages[frameID]
		//pg.WLatch()
		//pg.DecPinCount()
		b.mutex.Unlock()

		data := pg.Data()
		pg.SetIsDirty(false)

		//b.mutex.WLock()
		b.diskManager.WritePage(pageID, data[:])
		//pg.WUnlatch()
		//b.mutex.WUnlock()
		return true
	}
	b.mutex.Unlock()
	return false
}

// NewPage allocates a new page in the buffer pool with the disk manager help
func (b *BufferPoolManager) NewPage() *page.Page {

	b.mutex.Lock()
	frameID, isFromFreeList := b.getFrameID()
	if frameID == nil {
		b.mutex.Unlock()
		return nil // the buffer is full, it can't find a frame
	}

	if !isFromFreeList {
		// remove page from current frame
		currentPage := b.pages[*frameID]
		if currentPage != nil {
			fmt.Println("BPM::NewPage Cache out occurs!")
			if currentPage.IsDirty() {
				b.log_manager.Flush()
				data := currentPage.Data()
				b.diskManager.WritePage(currentPage.GetPageId(), data[:])
			}

			if common.EnableDebug {
				common.ShPrintf(common.DEBUG_INFO, "NewPage: page=%d is removed from pageTable.\n", currentPage.GetPageId())
			}
			delete(b.pageTable, currentPage.GetPageId())
		}
	}

	// allocates new page
	pageID := b.diskManager.AllocatePage()
	pg := page.NewEmpty(pageID)

	b.pageTable[pageID] = *frameID
	b.pages[*frameID] = pg

	b.mutex.Unlock()

	if common.EnableDebug {
		common.ShPrintf(common.DEBUG_INFO, "NewPage: returned pageID: %d\n", pageID)
	}

	return pg
}

// DeletePage make disk space of db file which is idenfied by pageID
func (b *BufferPoolManager) DeletePage(pageID types.PageID) error {
	// 0.   Make sure you call DiskManager::DeallocatePage!
	// 1.   Search the page table for the requested page (P).
	// 1.   If P does not exist, return true.
	// 2.   If P exists, but has a non-zero pin-count, return false. Someone is using the page.
	// 3.   Otherwise, P can be deleted. Remove P from the page table, reset its metadata and return it to the free list.

	//// operations even if the page is not on memory ////

	// do something if needed
	b.diskManager.DeallocatePage(pageID)
	// TODO: (SDB) implement reusable page list and add pageId to it (BufferPoolManager::DeletePage)

	////////////////////////////////////////////////////////

	var frameID FrameID
	var ok bool
	b.mutex.Lock()
	if frameID, ok = b.pageTable[pageID]; !ok {
		b.mutex.Unlock()
		//panic("delete target page not found on pageTable")
		return nil
	}

	page := b.pages[frameID]
	page.WLatch()
	if page.PinCount() > 0 {
		page.WUnlatch()
		b.mutex.Unlock()
		return nil
		//panic("Pin count greater than 0")
		//return errors.New("Pin count greater than 0")
	}

	// when the page is on memory
	if page.GetPageId() == pageID {
		delete(b.pageTable, pageID)
		(*b.replacer).Pin(frameID)
		b.freeList = append(b.freeList, frameID)
	}

	page.WUnlatch()
	b.mutex.Unlock()

	return nil
}

// FlushAllPages flushes all the pages in the buffer pool to disk.
func (b *BufferPoolManager) FlushAllPages() {
	pageIDs := make([]types.PageID, 0)
	b.mutex.Lock()
	for pageID, _ := range b.pageTable {
		pageIDs = append(pageIDs, pageID)
	}
	b.mutex.Unlock()

	for _, pageID := range pageIDs {
		b.FlushPage(pageID)
	}
}

// FlushAllDitryPages flushes all dirty pages in the buffer pool to disk.
func (b *BufferPoolManager) FlushAllDirtyPages() {
	pageIDs := make([]types.PageID, 0)
	b.mutex.Lock()
	for pageID, _ := range b.pageTable {
		if frameID, ok := b.pageTable[pageID]; ok {
			pg := b.pages[frameID]
			pg.RLatch()
			if pg.IsDirty() {
				pageIDs = append(pageIDs, pageID)
			}
			pg.RUnlatch()
		}
	}
	b.mutex.Unlock()

	for _, pageID := range pageIDs {
		b.FlushPage(pageID)
	}
}

func (b *BufferPoolManager) getFrameID() (*FrameID, bool) {
	//b.mutex.WLock()
	if len(b.freeList) > 0 {
		frameID, newFreeList := b.freeList[0], b.freeList[1:]
		b.freeList = newFreeList

		//b.mutex.WUnlock()
		return &frameID, true
	}

	ret := (*b.replacer).Victim()
	//b.mutex.WUnlock()
	if ret == nil {
		fmt.Printf("getFrameID: Victime page is nil! len(b.freeList)=%d\n", len(b.freeList))
		//panic("getFrameID: Victime page is nil!")
	}
	return ret, false
}

func (b *BufferPoolManager) GetPages() []*page.Page {
	return b.pages
}

func (b *BufferPoolManager) GetPoolSize() int {
	return len(b.pageTable)
}

// NewBufferPoolManager returns a empty buffer pool manager
func NewBufferPoolManager(poolSize uint32, DiskManager disk.DiskManager, log_manager *recovery.LogManager) *BufferPoolManager {
	freeList := make([]FrameID, poolSize)
	pages := make([]*page.Page, poolSize)
	for i := uint32(0); i < poolSize; i++ {
		freeList[i] = FrameID(i)
		pages[i] = nil
	}

	replacer := NewClockReplacer(poolSize)
	return &BufferPoolManager{DiskManager, pages, replacer, freeList, make(map[types.PageID]FrameID), log_manager, new(sync.Mutex)}
	//// when using "go-deadlock" package
	//deadlock.Opts.DisableLockOrderDetection = true
	//return &BufferPoolManager{DiskManager, pages, replacer, freeList, make(map[types.PageID]FrameID), log_manager, new(deadlock.Mutex)}
}
