package v0

import (
	"fmt"
	"sync"
	"sync/atomic"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/clist"
	"github.com/tendermint/tendermint/libs/log"
	tmsync "github.com/tendermint/tendermint/libs/sync"
	"github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/types"
)

// CListPriorityTxSidecar is an ordered in-memory pool for transactions stored
// on sentry nodes that validators can pull from. Transaction validity is not checked using the
// CheckTx abci message since no transactions are added to the mempool. The
// CListPriorityTxSidecar uses a concurrent list structure for storing transactions that can
// be efficiently accessed by multiple concurrent readers.
type CListPriorityTxSidecar struct {
	// Atomic integers
	height                 int64 // the last block Update()'d to
	heightForFiringAuction int64 // the height of the block to fire the auction for
	txsBytes               int64 // total size of sidecar, in bytes
	lastBundleHeight       int64 // height of last accepted bundle tx, for status rpc purposes

	// notify listeners (ie. consensus) when txs are available
	notifiedTxsAvailable bool
	txsAvailable         chan struct{} // fires once for each height, when the mempool is not empty

	txs    *clist.CList // concurrent linked-list of good SidecarTxs
	txsMap sync.Map

	// sync.Map: Key{height, bundleID} -> Bundle{
	// // height int64
	// // enforcedSize int64
	// // currSize int64
	// // bundleID int64
	// // sync.Map bundleOrder -> *SidecarTx
	// }
	bundles     sync.Map
	maxBundleID int64

	updateMtx tmsync.RWMutex

	// Keep a cache of already-seen txs.
	// This reduces the pressure on the proxyApp.
	cache mempool.TxCache

	logger log.Logger

	metrics *mempool.Metrics
}

var _ mempool.PriorityTxSidecar = &CListPriorityTxSidecar{}

type Key struct {
	height, bundleID int64
}

// NewCListSidecar returns a new sidecar with the given configuration
// takes in the logger for the mempool
func NewCListSidecar(
	height int64,
	memLogger log.Logger,
	memMetrics *mempool.Metrics,
) *CListPriorityTxSidecar {
	sidecar := &CListPriorityTxSidecar{
		txs:                    clist.New(),
		height:                 height,
		heightForFiringAuction: height + 1,
		logger:                 memLogger,
		metrics:                memMetrics,
	}
	sidecar.cache = mempool.NewLRUTxCache(10000)
	return sidecar
}

func (sc *CListPriorityTxSidecar) PrettyPrintBundles() {
	fmt.Println("-------------")
	for bundleIDIter := 0; bundleIDIter <= int(sc.maxBundleID); bundleIDIter++ {
		bundleIDIter := int64(bundleIDIter)
		if bundle, ok := sc.bundles.Load(Key{sc.heightForFiringAuction, bundleIDIter}); ok {
			bundle := bundle.(*mempool.Bundle)
			fmt.Printf("BUNDLE ID: %d\n", bundleIDIter)

			innerOrderMap := bundle.OrderedTxsMap
			bundleSize := bundle.CurrSize
			for bundleOrderIter := 0; bundleOrderIter < int(bundleSize); bundleOrderIter++ {
				bundleOrderIter := int64(bundleOrderIter)

				if scTx, ok := innerOrderMap.Load(bundleOrderIter); ok {
					scTx := scTx.(*mempool.SidecarTx)
					fmt.Printf("---> ORDER %d: %s\n", bundleOrderIter, scTx.Tx)
				}
			}
		}
	}
	fmt.Println("-------------")
}

//--------------------------------------------------------------------------------

// NOTE: not thread safe - should only be called once, on startup
func (sc *CListPriorityTxSidecar) EnableTxsAvailable() {
	sc.txsAvailable = make(chan struct{}, 1)
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsAvailable() <-chan struct{} {
	return sc.txsAvailable
}

func (sc *CListPriorityTxSidecar) notifyTxsAvailable() {
	if sc.Size() == 0 {
		panic("[mev-tendermint]: notified txs available but sidecar is empty!")
	}
	if sc.txsAvailable != nil && !sc.notifiedTxsAvailable {
		// channel cap is 1, so this will send once
		sc.notifiedTxsAvailable = true
		select {
		case sc.txsAvailable <- struct{}{}:
		default:
		}
	}
}

//--------------------------------------------------------------------------------

func (sc *CListPriorityTxSidecar) AddTx(tx types.Tx, txInfo mempool.TxInfo) error {

	sc.updateMtx.RLock()
	// use defer to unlock mutex because application (*local client*) might panic
	defer sc.updateMtx.RUnlock()

	// don't add any txs already in cache
	if !sc.cache.Push(tx) {
		sc.logger.Debug(
			"rejected sidecarTx: already in cache",
			"bundle id", txInfo.BundleID,
			"bundle order", txInfo.BundleOrder,
			"tx", tx.Hash(),
		)
		// Record a new sender for a tx we've already seen.
		// Note it's possible a tx is still in the cache but no longer in the mempool
		// (eg. after committing a block, txs are removed from mempool but not cache),
		// so we only record the sender for txs still in the mempool.
		// Record a new sender for a tx we've already seen.

		if e, ok := sc.txsMap.Load(tx.Key()); ok {
			scTx := e.(*clist.CElement).Value.(*mempool.SidecarTx)
			scTx.Senders.LoadOrStore(txInfo.SenderID, true)
		}

		return mempool.ErrTxInCache
	}

	scTx := &mempool.SidecarTx{
		DesiredHeight: txInfo.DesiredHeight,
		Tx:            tx,
		BundleID:      txInfo.BundleID,
		BundleOrder:   txInfo.BundleOrder,
		BundleSize:    txInfo.BundleSize,
	}

	// add to metrics that we've received a new tx
	sc.metrics.NumMevTxsTotal.Add(1)

	// -------- BASIC CHECKS ON TX INFO ---------

	// Can't add transactions asking to be included in a height for auction we're not on
	if txInfo.DesiredHeight < sc.heightForFiringAuction {
		sc.logger.Info(
			"failed adding sidecarTx",
			"trying to add a tx for height", txInfo.DesiredHeight,
			"whereas height for curr auction is", sc.heightForFiringAuction,
			"bundle id", txInfo.BundleID,
			"bundle order", txInfo.BundleOrder,
			"tx", tx.Hash(),
		)
		return mempool.ErrWrongHeight{
			DesiredHeight:        int(txInfo.DesiredHeight),
			CurrentAuctionHeight: int(sc.heightForFiringAuction),
		}
	}

	// revert if tx asking to be included has an order greater/equal to size
	if txInfo.BundleOrder >= txInfo.BundleSize {
		sc.logger.Info(
			"failed adding sidecarTx",
			"trying to insert a tx for bundle at an order greater than the size of the bundle...",
			"bundle id", txInfo.BundleID,
			"bundle order", txInfo.BundleOrder,
			"bundle size", txInfo.BundleSize,
			"tx", tx.Hash(),
		)
		return mempool.ErrTxMalformedForBundle{
			BundleID:     txInfo.BundleID,
			BundleSize:   txInfo.BundleSize,
			BundleHeight: txInfo.DesiredHeight,
			BundleOrder:  txInfo.BundleOrder,
		}
	}

	// -------- BUNDLE EXISTENCE CHECKS ---------

	var bundle *mempool.Bundle
	// load existing bundle, or MAKE NEW if not
	existingBundle, _ := sc.bundles.LoadOrStore(Key{txInfo.DesiredHeight, txInfo.BundleID}, &mempool.Bundle{
		DesiredHeight: txInfo.DesiredHeight,
		BundleID:      txInfo.BundleID,
		CurrSize:      int64(0),
		EnforcedSize:  txInfo.BundleSize,
		// TODO: add from gossip info?
		GasWanted:     int64(0),
		OrderedTxsMap: &sync.Map{},
	})
	bundle = existingBundle.(*mempool.Bundle)

	// -------- BUNDLE SIZE CHECKS ---------

	// check if bundle is asking for a different size than one already stored
	if txInfo.BundleSize != bundle.EnforcedSize {
		sc.logger.Info(
			"failed adding sidecarTx",
			"trying to insert a tx for bundle at an order greater than the size of the bundle...",
			"bundle id", txInfo.BundleID,
			"bundle size", txInfo.BundleSize,
			"enforced size", bundle.EnforcedSize,
			"tx", tx.Hash(),
		)
		return mempool.ErrTxMalformedForBundle{
			BundleID:     txInfo.BundleID,
			BundleSize:   txInfo.BundleSize,
			BundleHeight: txInfo.DesiredHeight,
			BundleOrder:  txInfo.BundleOrder,
		}
	}

	// Can't add transactions if the bundle is already full
	// check if the current size of this bundle is greater than the expected size for the bundle, if so skip
	if bundle.CurrSize >= bundle.EnforcedSize {
		sc.logger.Info(
			"failed adding sidecarTx",
			"bundle already full for this BundleID...",
			"bundle id", txInfo.BundleID,
			"bundle curr size", bundle.CurrSize,
			"enforced size", bundle.EnforcedSize,
			"tx", tx.Hash(),
		)
		return mempool.ErrBundleFull{
			BundleID:     txInfo.BundleID,
			BundleHeight: txInfo.BundleSize,
		}
	}

	// -------- TX INSERTION INTO BUNDLE ---------

	// get the map of order -> scTx
	orderedTxsMap := bundle.OrderedTxsMap

	// if we already have a tx at this BundleID, bundleOrder, and height, then skip this one!
	if _, loaded := orderedTxsMap.LoadOrStore(txInfo.BundleOrder, scTx); loaded {
		// if we had the tx already, then skip
		sc.logger.Info(
			"failed adding sidecarTx",
			"already have a tx for this BundleID, height, and bundleOrder...",
			"bundle id", txInfo.BundleID,
			"bundle height", txInfo.DesiredHeight,
			"bundle order", txInfo.BundleOrder,
			"tx", tx.Hash(),
		)
		return nil
	}
	// if we added, then increment bundle size for BundleID
	atomic.AddInt64(&bundle.CurrSize, int64(1))

	// -------- UPDATE MAX BUNDLE ---------

	if txInfo.BundleID >= sc.maxBundleID {
		sc.maxBundleID = txInfo.BundleID
	}

	// -------- TX INSERTION INTO MAIN TXS LIST ---------
	// -------- TODO: In the future probably want to refactor to not have txs clist ---------

	e := sc.txs.PushBack(scTx)
	sc.txsMap.Store(scTx.Tx.Key(), e)
	atomic.AddInt64(&sc.txsBytes, int64(len(scTx.Tx)))

	// add metric for new sidecar size
	sc.metrics.SidecarSize.Set(float64(sc.Size()))

	// add metric for sidecar tx sampling
	sc.metrics.SidecarTxSizeBytes.Observe(float64(len(scTx.Tx)))

	sc.lastBundleHeight = scTx.DesiredHeight

	// TODO: in the future, refactor to only notifyTxsAvailable when we have at least one full bundle
	if sc.Size() > 0 {
		sc.notifyTxsAvailable()
	}
	sc.logger.Info(
		"Added sidecar tx successfully",
		"tx", tx,
		"BundleID", txInfo.BundleID,
		"bundleOrder", txInfo.BundleOrder,
		"desiredHeight", txInfo.DesiredHeight,
		"bundleSize", txInfo.BundleSize,
		"sidecar total size", sc.Size(),
	)

	return nil
}

// Gets the height of the last received bundle tx. For status rpc purposes.
func (sc *CListPriorityTxSidecar) GetLastBundleHeight() int64 {
	if sc == nil {
		return 0
	}
	return sc.lastBundleHeight
}

// TxsWaitChan returns a channel to wait on transactions. It will be closed
// once the sidecar is not empty (ie. the internal `mem.txs` has at least one
// element)
//
// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsWaitChan() <-chan struct{} {
	return sc.txs.WaitChan()
}

// TxsFront returns the first transaction in the ordered list for peer
// goroutines to call .NextWait() on.
// FIXME: leaking implementation details!
//
// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsFront() *clist.CElement {
	return sc.txs.Front()
}

// Lock() must be held by the caller during execution.
func (sc *CListPriorityTxSidecar) Update(
	height int64,
	txs types.Txs,
	deliverTxResponses []*abci.ResponseDeliverTx,
) error {

	// Set height for block last updated to (i.e. block last committed)
	sc.height = height
	sc.notifiedTxsAvailable = false
	sc.heightForFiringAuction = height + 1

	for i, tx := range txs {
		if e, ok := sc.txsMap.Load(tx.Key()); ok {
			if deliverTxResponses[i].Code == abci.CodeTypeOK {
				sc.logger.Debug(
					"removed valid commited tx from sidecar",
					"tx", tx,
					"mempool updated height", height,
					"sidecar total size", sc.Size(),
				)
			} else {
				sc.logger.Debug(
					"removed invalid commited tx from sidecar",
					"tx", tx,
					"mempool updated height", height,
					"sidecar total size", sc.Size(),
				)
			}
			sc.removeTx(tx, e.(*clist.CElement), false)
		}
	}

	sc.cache.Reset()
	sc.maxBundleID = 0

	// remove from txs list and txmap
	for e := sc.txs.Front(); e != nil; e = e.Next() {
		scTx := e.Value.(*mempool.SidecarTx)
		if scTx.DesiredHeight <= height {
			sc.logger.Debug(
				"removed uncommited tx from sidecar",
				"tx", scTx.Tx,
				"tx desired height", scTx.DesiredHeight,
				"mempool updated height", height,
				"sidecar total size", sc.Size(),
			)
			tx := scTx.Tx
			sc.removeTx(tx, e, false)
		}
	}

	// remove the bundles
	sc.bundles.Range(func(key, _ interface{}) bool {
		if bundle, ok := sc.bundles.Load(key); ok {
			bundle := bundle.(*mempool.Bundle)
			if bundle.DesiredHeight <= height {
				sc.logger.Debug(
					"removed bundle from sidecar",
					"bundle id", bundle.BundleID,
					"tx desired height", bundle.DesiredHeight,
					"mempool updated height", height,
					"sidecar total size", sc.Size(),
				)
				sc.bundles.Delete(key)
			}
		}
		return true
	})

	// add metric for new sidecar size
	sc.metrics.SidecarSize.Set(float64(sc.Size()))

	return nil
}

// Lock() must be help by the caller during execution.
// Lock() must be help by the caller during execution.
func (sc *CListPriorityTxSidecar) Flush() {
	sc.cache.Reset()

	sc.notifiedTxsAvailable = false
	sc.maxBundleID = 0

	_ = atomic.SwapInt64(&sc.txsBytes, 0)

	for e := sc.txs.Front(); e != nil; e = e.Next() {
		sc.txs.Remove(e)
		e.DetachPrev()
	}

	sc.txsMap.Range(func(key, _ interface{}) bool {
		sc.txsMap.Delete(key)
		return true
	})

	sc.bundles.Range(func(key, _ interface{}) bool {
		sc.bundles.Delete(key)
		return true
	})
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Size() int {
	return sc.txs.Len()
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) NumBundles() int {
	i := 0
	sc.bundles.Range(func(key, _ interface{}) bool {
		i++
		return true
	})
	return i
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) MaxBundleID() int64 {
	return sc.maxBundleID
}

func (sc *CListPriorityTxSidecar) HeightForFiringAuction() int64 {
	return sc.heightForFiringAuction
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) GetEnforcedBundleSize(bundleID int64) int {
	if bundle, ok := sc.bundles.Load(Key{sc.heightForFiringAuction, bundleID}); ok {
		bundle := bundle.(*mempool.Bundle)
		return int(bundle.EnforcedSize)
	}
	sc.logger.Info(
		"error GetEnforcedBundleSize: Don't have a bundle for bundleID",
		"BundleID", bundleID,
	)
	return 0
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) GetCurrBundleSize(bundleID int64) int {
	if bundle, ok := sc.bundles.Load(Key{sc.heightForFiringAuction, bundleID}); ok {
		bundle := bundle.(*mempool.Bundle)
		return int(bundle.CurrSize)
	}
	sc.logger.Info(
		"error GetBundleSize: Don't have a bundle for bundleID",
		"BundleID", bundleID,
	)
	return 0
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsBytes() int64 {
	return atomic.LoadInt64(&sc.txsBytes)
}

// Called from:
//   - FlushSidecar (lock held) if tx was committed
func (sc *CListPriorityTxSidecar) removeTx(tx types.Tx, elem *clist.CElement, removeFromCache bool) {
	sc.txs.Remove(elem)
	elem.DetachPrev()
	sc.txsMap.Delete(tx.Key())
	atomic.AddInt64(&sc.txsBytes, int64(-len(tx)))

	if removeFromCache {
		sc.cache.Remove(tx)
	}
}

// Safe for concurrent use by multiple goroutines.

// this reap function iterates over all the bundleIDs up to maxBundleID
// ... then goes over each bundle via the bundleOrders (up to enforcedSize for bundle)
// ... and reaps them in this order
func (sc *CListPriorityTxSidecar) ReapMaxTxs() []*mempool.MempoolTx {
	sc.updateMtx.RLock()
	defer sc.updateMtx.RUnlock()

	sc.logger.Info(
		"Entering sidecar reap: sidecar size at this time is", sc.Size(),
	)

	memTxs := make([]*mempool.MempoolTx, 0, sc.txs.Len())

	if (sc.txs.Len() == 0) || (sc.NumBundles() == 0) {
		return memTxs
	}

	completedBundles := 0
	numTxsInBundles := 0

	// iterate over all BundleIDs up to the max we've seen
	// CONTRACT: this assumes that bundles don't care about previous bundles,
	// so still want to execute if any missing between
	for bundleIDIter := 0; bundleIDIter <= int(sc.maxBundleID); bundleIDIter++ {
		bundleIDIter := int64(bundleIDIter)

		if bundle, ok := sc.bundles.Load(Key{sc.heightForFiringAuction, bundleIDIter}); ok {
			bundle := bundle.(*mempool.Bundle)
			bundleOrderedTxsMap := bundle.OrderedTxsMap

			// check to see if bundle is full, if not, just skip now
			if bundle.CurrSize != bundle.EnforcedSize {
				sc.logger.Info(
					"In reap: skipping bundle, size mismatch",
					"bundleID", bundleIDIter,
					"height", sc.heightForFiringAuction,
					"currSize", bundle.CurrSize,
					"enforcedSize", bundle.EnforcedSize,
				)
				continue
			}

			// if full, iterate over bundle in order and add txs to temporary store, then add all if we have enough (i.e. matches enforcedBundleSize)
			innerTxs := make([]*mempool.MempoolTx, 0, bundle.EnforcedSize)
			for bundleOrderIter := 0; bundleOrderIter < int(bundle.EnforcedSize); bundleOrderIter++ {
				bundleOrderIter := int64(bundleOrderIter)

				if scTx, ok := bundleOrderedTxsMap.Load(bundleOrderIter); ok {
					// loading as sidecar tx, but casting to MempoolTx to return
					scTx := scTx.(*mempool.SidecarTx)
					memTx := &mempool.MempoolTx{
						// CONTRACT: since the only height this could have been added into is desiredHeight = mem.height + 1,
						// then this tx must have been validated against mem.height
						Height:    scTx.DesiredHeight - 1,
						GasWanted: scTx.GasWanted,
						Tx:        scTx.Tx,
						Senders:   scTx.Senders, //nolint:govet // TODO: fix
					}
					innerTxs = append(innerTxs, memTx)
				} else {
					// can't find tx at this bundleOrder for this bundleID
					sc.logger.Info(
						"In reap: skipping bundle, don't have a tx for bundle",
						"bundleID", bundleIDIter,
						"bundleOrder", bundleOrderIter,
						"height", sc.heightForFiringAuction,
					)
				}
			}

			// check to see if we have the right number of transactions for the bundle, comparing to the enforced size
			if bundle.EnforcedSize == int64(len(innerTxs)) {
				// check to see if we've reaped the right number of txs expected for the bundle
				memTxs = append(memTxs, innerTxs...)
				completedBundles++
				numTxsInBundles += len(innerTxs)

				// update metrics with total number of bundles reaped
				sc.metrics.NumBundlesTotal.Add(1)
			} else {
				sc.logger.Info(
					"In reap: skipping bundle, size mismatch",
					"num txs reaped", len(innerTxs),
					"bundle current size", bundle.CurrSize,
					"bundle enforced size", bundle.EnforcedSize,
					"bundleID", bundleIDIter,
					"height", sc.heightForFiringAuction,
				)
			}
		} else {
			// can't find a bundle for this bundleID, panic! (incomplete gossipping)
			sc.logger.Info(
				"In reap: skipping bundle, don't have a bundle entry",
				"bundleID", bundleIDIter,
				"height", sc.heightForFiringAuction,
			)
		}
	}

	// update metrics with number of bundles reaped this block
	sc.metrics.NumBundlesLastBlock.Set(float64(completedBundles))

	// update metrics for number of mev transactions reaped this block
	sc.metrics.NumMevTxsLastBlock.Set(float64(numTxsInBundles))

	return memTxs
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Lock() {
	sc.updateMtx.Lock()
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Unlock() {
	sc.updateMtx.Unlock()
}
