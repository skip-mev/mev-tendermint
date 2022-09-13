package mempool

import (
	"fmt"
	"sync"
	"sync/atomic"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/clist"
	tmsync "github.com/tendermint/tendermint/libs/sync"
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

	// notify listeners (ie. consensus) when txs are available
	notifiedTxsAvailable bool
	txsAvailable         chan struct{} // fires once for each height, when the mempool is not empty

	updateMtx tmsync.RWMutex

	// Keep a cache of already-seen txs.
	// This reduces the pressure on the proxyApp.
	cache txCache

	// map from height -> HeightState
	heightStates sync.Map

	// fires one per height
	heightChan chan struct{}
}

var _ PriorityTxSidecar = &CListPriorityTxSidecar{}

// NewCListSidecar returns a new sidecar with the given configuration
func NewCListSidecar(
	height int64,
) *CListPriorityTxSidecar {
	sidecar := &CListPriorityTxSidecar{
		height:                 height,
		heightForFiringAuction: height + 1,
	}
	// TODO: update
	sidecar.cache = newMapTxCache(10000)

	sidecar.heightStates.Store(height, &HeightState{
		maxBundleId: 0,
		txs:         clist.New(),
	})
	sidecar.heightStates.Store(height+1, &HeightState{
		maxBundleId: 0,
		txs:         clist.New(),
	})

	sidecar.heightChan = make(chan struct{}, 1)

	return sidecar
}

func (sc *CListPriorityTxSidecar) PrettyPrintBundles() {
	fmt.Println(fmt.Sprintf("-------------"))
	if hs, ok := sc.heightStates.Load(sc.heightForFiringAuction); ok {
		hs := hs.(*HeightState)
		for bundleIdIter := 0; bundleIdIter <= int(hs.maxBundleId); bundleIdIter++ {
			bundleIdIter := int64(bundleIdIter)
			if bundle, ok := hs.bundles.Load(bundleIdIter); ok {
				bundle := bundle.(*Bundle)
				fmt.Println(fmt.Sprintf("BUNDLE ID: %d", bundleIdIter))

				innerOrderMap := bundle.orderedTxsMap
				bundleSize := bundle.currSize
				for bundleOrderIter := 0; bundleOrderIter < int(bundleSize); bundleOrderIter++ {
					bundleOrderIter := int64(bundleOrderIter)

					if scTx, ok := innerOrderMap.Load(bundleOrderIter); ok {
						scTx := scTx.(*SidecarTx)
						fmt.Println(fmt.Sprintf("---> ORDER %d: %s", bundleOrderIter, scTx.tx))
					}
				}
			}
		}
	}
	fmt.Println(fmt.Sprintf("-------------"))
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

func (sc *CListPriorityTxSidecar) notifyNewHeight() {
	if sc.heightChan != nil {
		// channel cap is 1, so this will send once
		select {
		case sc.heightChan <- struct{}{}:
		default:
		}
	}
}

func (sc *CListPriorityTxSidecar) NewHeightChan() <-chan struct{} {
	return sc.heightChan
}

//--------------------------------------------------------------------------------

// TODO: Update to AddTx(tx types.Tx, txInfo TxInfo, order int64) error
func (sc *CListPriorityTxSidecar) AddTx(tx types.Tx, txInfo TxInfo) error {

	sc.updateMtx.RLock()
	// use defer to unlock mutex because application (*local client*) might panic
	defer sc.updateMtx.RUnlock()

	fmt.Println(fmt.Sprintf("[mev-tendermint]: STARTING TO ADD TRANSACTION %.20q TO SIDECAR! with bundleId %d, bundleOrder %d, desiredHeight %d, bundleSize %d", tx, txInfo.BundleId, txInfo.BundleOrder, txInfo.DesiredHeight, txInfo.BundleSize))

	// don't add any txs already in cache
	if !sc.cache.Push(tx) {
		fmt.Println(fmt.Sprintf("[mev-tendermint]: trying to add tx %.20q to sidecar AddTx - but already in cache!", tx))

		if hs, ok := sc.heightStates.Load(txInfo.DesiredHeight); ok {
			hs := hs.(*HeightState)
			if e, ok := hs.txsMap.Load(TxKey(tx)); ok {
				scTx := e.(*clist.CElement).Value.(*SidecarTx)
				scTx.senders.LoadOrStore(txInfo.SenderID, true)
			}
		}

		return ErrTxInCache
	}

	// load or store new heightstate
	var hs *HeightState = &HeightState{
		maxBundleId: 0,
		txs:         clist.New(),
	}
	if hsLoad, loaded := sc.heightStates.LoadOrStore(txInfo.DesiredHeight, hs); loaded {
		hsLoad := hsLoad.(*HeightState)
		hs = hsLoad
	}

	scTx := &SidecarTx{
		desiredHeight: txInfo.DesiredHeight,
		tx:            tx,
		bundleId:      txInfo.BundleId,
		bundleOrder:   txInfo.BundleOrder,
		bundleSize:    txInfo.BundleSize,
		// TODO: gas
	}

	// -------- BASIC CHECKS ON TX INFO ---------

	// Can't add transactions asking to be included in a height for auction we're not on
	if txInfo.DesiredHeight < sc.heightForFiringAuction {
		fmt.Println(fmt.Sprintf("[mev-tendermint]: AddTx() skip tx... trying to add a tx for height %d (TOO LOW) whereas height for curr auction is %d", txInfo.DesiredHeight, sc.heightForFiringAuction))
		return ErrWrongHeight{
			int(txInfo.DesiredHeight),
			int(sc.heightForFiringAuction),
		}
	}

	// Revert if tx asking to be included has an order greater/equal to size
	if txInfo.BundleOrder >= txInfo.BundleSize {
		fmt.Println("[mev-tendermint]: AddTx() skip tx... trying to insert a tx for bundle at an order greater than the size of the bundle... THIS IS PROBABLY A FATAL ERROR")
		return ErrTxMalformedForBundle{
			txInfo.BundleId,
			txInfo.BundleSize,
			txInfo.DesiredHeight,
			txInfo.BundleOrder,
		}
	}

	// -------- BUNDLE EXISTENCE CHECKS ---------

	var bundle *Bundle
	// load existing bundle, or MAKE NEW if not
	existingBundle, _ := hs.bundles.LoadOrStore(txInfo.BundleId, &Bundle{
		desiredHeight: txInfo.DesiredHeight,
		bundleId:      txInfo.BundleId,
		currSize:      int64(0),
		enforcedSize:  txInfo.BundleSize,
		// TODO: add from gossip info?
		gasWanted:     int64(0),
		orderedTxsMap: &sync.Map{},
	})
	bundle = existingBundle.(*Bundle)

	// -------- BUNDLE SIZE CHECKS ---------

	// check if bundle is asking for a different size than one already stored
	if txInfo.BundleSize != bundle.enforcedSize {
		fmt.Println("[mev-tendermint]: AddTx() skip tx... Trying to insert a tx with a size different than what's said by other txs for this bundle?? ... THIS IS PROBABLY A FATAL ERROR")
		return ErrTxMalformedForBundle{
			txInfo.BundleId,
			txInfo.BundleSize,
			txInfo.DesiredHeight,
			txInfo.BundleOrder,
		}
	}

	// Can't add transactions if the bundle is already full
	// check if the current size of this bundle is greater than the expected size for the bundle, if so skip
	if bundle.currSize >= bundle.enforcedSize {
		fmt.Println("[mev-tendermint]: AddTx() skip tx... already full for this BundleId... THIS IS PROBABLY A FATAL ERROR")
		return ErrBundleFull{
			txInfo.BundleId,
			txInfo.BundleSize,
		}
	}

	// -------- TX INSERTION INTO BUNDLE ---------

	// get the map of order -> scTx
	orderedTxsMap := bundle.orderedTxsMap

	// TODO: could add check to not add if bundleSize already over limit!
	// if we already have a tx at this bundleId, bundleOrder, and height, then skip this one!
	if _, loaded := orderedTxsMap.LoadOrStore(txInfo.BundleOrder, scTx); loaded {
		// if we had the tx already, then skip
		// TODO: return error
		fmt.Println(fmt.Sprintf("[mev-tendermint]: AddTx() skip tx... already have a tx for bundleId %d, height %d, bundleOrder %d", txInfo.BundleId, scTx.desiredHeight, txInfo.BundleOrder))
		return nil
	} else {
		// if we added, then increment bundle size for bundleId
		atomic.AddInt64(&bundle.currSize, int64(1))
	}

	// -------- UPDATE MAX BUNDLE ---------

	if txInfo.BundleId >= hs.maxBundleId {
		fmt.Println("[mev-tendermint]: AddTx(): updating maxBundleId to", txInfo.BundleId)
		hs.maxBundleId = txInfo.BundleId
	}

	// -------- TX INSERTION INTO MAIN TXS LIST ---------
	// -------- TODO: In the future probably want to refactor to not have txs clist ---------

	e := hs.txs.PushBack(scTx)
	hs.txsMap.Store(TxKey(scTx.tx), e)

	atomic.AddInt64(&hs.txsBytes, int64(len(scTx.tx)))
	fmt.Println(fmt.Sprintf("[mev-tendermint]: AddTx(): actually added the tx to the hs.txs CList, sidecar size is now %d for height %d, and heightToFire is %d", sc.Size(txInfo.DesiredHeight), txInfo.DesiredHeight, sc.heightForFiringAuction))

	// TODO: in the future, refactor to only notifyTxsAvailable when we have at least one full bundle
	// if sc.Size() > 0 {
	// 	sc.notifyTxsAvailable()
	// }

	fmt.Println("[mev-tendermint]: ADDING SIDECAR TX FUNCTION COMPLETION")

	return nil
}

// TxsWaitChan returns a channel to wait on transactions. It will be closed
// once the sidecar is not empty (ie. the internal `mem.txs` has at least one
// element)
//
// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsWaitChan(height int64) <-chan struct{} {
	fmt.Println("[mev-tendermint]: calling TxsWaitChan() with height", height)
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		return hs.txs.WaitChan()
	}
	fmt.Println("[mev-tendermint]: TxsWaitChan() - returning nil for height", height)
	return nil
}

// TxsFront returns the first transaction in the ordered list for peer
// goroutines to call .NextWait() on.
// FIXME: leaking implementation details!
//
// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsFront(height int64) *clist.CElement {
	fmt.Println("[mev-tendermint]: calling TxsFront() with height", height)
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		return hs.txs.Front()
	}
	fmt.Println("[mev-tendermint]: TxsFront() - returning nil for height", height)
	return nil
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

	// create new height state if doesn't exist
	var hs *HeightState = &HeightState{
		maxBundleId: 0,
		txs:         clist.New(),
	}
	sc.heightStates.LoadOrStore(sc.heightForFiringAuction, hs)

	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)

		fmt.Println("notifying new height for height", height)
		sc.notifyNewHeight()

		for i, tx := range txs {
			if _, ok := hs.txsMap.Load(TxKey(tx)); ok {
				fmt.Println(fmt.Sprintf("[mev-tendermint]: on sidecar Update() for height %d, and heightToFire %d found tx in sidecar!", height, sc.heightForFiringAuction))
				if deliverTxResponses[i].Code == abci.CodeTypeOK {
					fmt.Println("... and was valid!")
				}
				fmt.Println(fmt.Sprintf("%.20q", tx))
			}
		}
	}

	sc.Flush(height)

	return nil
}

// Lock() must be help by the caller during execution.
func (sc *CListPriorityTxSidecar) Flush(height int64) {
	sc.cache.Reset()

	sc.notifiedTxsAvailable = false

	// TODO: does this persist states? how to delete?
	sc.heightStates.Store(height, nil)
	sc.heightStates.Delete(height)
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Size(height int64) int {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		return hs.txs.Len()
	}
	return 0
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) NumBundles(height int64) int {
	i := 0
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		hs.bundles.Range(func(key, _ interface{}) bool {
			i++
			return true
		})
	}
	return i
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) MaxBundleId(height int64) int64 {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		return hs.maxBundleId
	}
	return 0
}

func (sc *CListPriorityTxSidecar) HeightForFiringAuction() int64 {
	return sc.heightForFiringAuction
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) GetEnforcedBundleSize(height int64, bundleId int64) int {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		if bundle, ok := hs.bundles.Load(bundleId); ok {
			bundle := bundle.(*Bundle)
			return int(bundle.enforcedSize)
		} else {
			fmt.Println("Error GetEnforcedBundleSize(): Don't have a bundle for bundleId", bundleId)
		}
	}
	return 0
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) GetCurrBundleSize(height int64, bundleId int64) int {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		if bundle, ok := hs.bundles.Load(bundleId); ok {
			bundle := bundle.(*Bundle)
			return int(bundle.currSize)
		} else {
			fmt.Println("Error GetBundleSize(): Don't have a bundle for bundleId", bundleId)
		}
	}
	return 0
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) TxsBytes(height int64) int64 {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		return atomic.LoadInt64(&hs.txsBytes)
	}
	return 0
}

// Called from:
//  - FlushSidecar (lock held) if tx was committed
func (sc *CListPriorityTxSidecar) removeTx(height int64, tx types.Tx, elem *clist.CElement, removeFromCache bool) {
	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)
		hs.txs.Remove(elem)
		elem.DetachPrev()

		hs.txsMap.Delete(TxKey(tx))
		atomic.AddInt64(&hs.txsBytes, int64(-len(tx)))

		if removeFromCache {
			sc.cache.Remove(tx)
		}
	}
}

// Safe for concurrent use by multiple goroutines.
// TODO: add gas and byte limits (but requires tracking gas)

// this reap function iterates over all the bundleIds up to maxBundleId
// ... then goes over each bundle via the bundleOrders (up to enforcedSize for bundle)
// ... and reaps them in this order
func (sc *CListPriorityTxSidecar) ReapMaxTxs(height int64) []*MempoolTx {
	sc.updateMtx.RLock()
	defer sc.updateMtx.RUnlock()

	fmt.Println(fmt.Sprintf("REAPING SIDECAR via ReapMaxTxs(): sidecar size at this time is %d", sc.Size(height)))

	if hs, ok := sc.heightStates.Load(height); ok {
		hs := hs.(*HeightState)

		memTxs := make([]*MempoolTx, 0, hs.txs.Len())

		// Return immediately if no bundles
		if (hs.txs.Len() == 0) || (sc.NumBundles(height) == 0) {
			return memTxs
		}

		// iterate over all bundleIds up to the max we've seen
		// CONTRACT: this assumes that bundles don't care about previous bundles, so still want to execute if any missing between
		for bundleIdIter := 0; bundleIdIter <= int(hs.maxBundleId); bundleIdIter++ {
			bundleIdIter := int64(bundleIdIter)

			if bundle, ok := hs.bundles.Load(bundleIdIter); ok {
				bundle := bundle.(*Bundle)
				bundleOrderedTxsMap := bundle.orderedTxsMap

				// check to see if bundle is full, if not, just skip now
				if bundle.currSize != bundle.enforcedSize {
					fmt.Println(fmt.Sprintf("ReapMaxTxs() SKIPPING BUNDLE...: size mismatch for bundleId %d at height %d: currSize %d, enforcedSize %d: SKIPPING...", bundleIdIter, sc.heightForFiringAuction, bundle.currSize, bundle.enforcedSize))
					continue
				}

				// if full, iterate over bundle in order and add txs to temporary store, then add all if we have enough (i.e. matches enforcedBundleSize)
				innerTxs := make([]*MempoolTx, 0, bundle.enforcedSize)
				for bundleOrderIter := 0; bundleOrderIter < int(bundle.enforcedSize); bundleOrderIter++ {
					bundleOrderIter := int64(bundleOrderIter)

					if scTx, ok := bundleOrderedTxsMap.Load(bundleOrderIter); ok {
						// loading as sidecar tx, but casting to MempoolTx to return
						scTx := scTx.(*SidecarTx)
						memTx := &MempoolTx{
							// CONTRACT: since the only height this could have been added into is desiredHeight = mem.height + 1, then this tx must have been validated against mem.height
							height:    scTx.desiredHeight - 1,
							gasWanted: scTx.gasWanted,
							tx:        scTx.tx,
							senders:   scTx.senders,
						}
						innerTxs = append(innerTxs, memTx)
					} else {
						// can't find tx at this bundleOrder for this bundleId
						fmt.Println(fmt.Sprintf("ReapMaxTxs() skip: don't have memTx for bundleOrder %d bundleId %d at height %d", bundleOrderIter, bundleIdIter, sc.heightForFiringAuction))
					}
				}

				// check to see if we have the right number of transactions for the bundle, comparing to the enforced size
				if bundle.enforcedSize == int64(len(innerTxs)) {
					// check to see if we've reaped the right number of txs expected for the bundle
					memTxs = append(memTxs, innerTxs...)
				} else {
					fmt.Println(fmt.Sprintf("ReapMaxTxs() SKIPPING BUNDLE...: size mismatch for bundleId %d at height %d: reaped %d, bundleSize %d, enforcedBundleSize %d: SKIPPING...", bundleIdIter, sc.heightForFiringAuction, len(innerTxs), bundle.currSize, bundle.enforcedSize))
				}
			} else {
				// can't find a bundle for this bundleId, panic! (incomplete gossipping)
				fmt.Println(fmt.Sprintf("ReapMaxTxs() SKIPPING BUNDLE...: don't have bundle entry for bundleId %d at height %d", bundleIdIter, sc.heightForFiringAuction))
			}
		}
		return memTxs
	}
	return make([]*MempoolTx, 0)
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Lock() {
	sc.updateMtx.Lock()
}

// Safe for concurrent use by multiple goroutines.
func (sc *CListPriorityTxSidecar) Unlock() {
	sc.updateMtx.Unlock()
}
