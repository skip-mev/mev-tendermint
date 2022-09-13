package mock

import (
	"sync"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/clist"
	mempl "github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/types"
)

// Mempool is an empty implementation of a Mempool, useful for testing.
type Mempool struct{}

// mempoolTx is a transaction that successfully ran
type mempoolTx struct {
	height      int64 // height that this tx wants to be included in
	bundleId    int64 // ordered id of bundle
	bundleOrder int64 // order of tx within bundle
	bundleSize  int64 // total size of bundle

	gasWanted int64    // amount of gas this tx states it will require
	tx        types.Tx // tx bytes

	// ids of peers who've sent us this tx (as a map for quick lookups).
	// senders: PeerID -> bool
	senders sync.Map
}

var _ mempl.Mempool = Mempool{}

func (Mempool) Lock()     {}
func (Mempool) Unlock()   {}
func (Mempool) Size() int { return 0 }
func (Mempool) CheckTx(_ types.Tx, _ func(*abci.Response), _ mempl.TxInfo) error {
	return nil
}
func (Mempool) ReapMaxBytesMaxGas(_, _ int64, _ []*mempl.MempoolTx) types.Txs { return types.Txs{} }
func (Mempool) ReapMaxTxs(n int) types.Txs                                    { return types.Txs{} }
func (Mempool) Update(
	_ int64,
	_ types.Txs,
	_ []*abci.ResponseDeliverTx,
	_ mempl.PreCheckFunc,
	_ mempl.PostCheckFunc,
) error {
	return nil
}
func (Mempool) Flush()                        {}
func (Mempool) FlushAppConn() error           { return nil }
func (Mempool) TxsAvailable() <-chan struct{} { return make(chan struct{}) }
func (Mempool) EnableTxsAvailable()           {}
func (Mempool) TxsBytes() int64               { return 0 }

func (Mempool) TxsFront() *clist.CElement    { return nil }
func (Mempool) TxsWaitChan() <-chan struct{} { return nil }

func (Mempool) InitWAL() error { return nil }
func (Mempool) CloseWAL()      {}

// PriorityTxSidecar is an empty implementation of a PriorityTxSidecar, useful for testing.
type PriorityTxSidecar struct{}

var _ mempl.PriorityTxSidecar = PriorityTxSidecar{}

func (PriorityTxSidecar) AddTx(_ types.Tx, _ mempl.TxInfo) error { return nil }
func (PriorityTxSidecar) ReapMaxTxs(_ int64) []*mempl.MempoolTx  { return []*mempl.MempoolTx{} }

func (PriorityTxSidecar) Lock()   {}
func (PriorityTxSidecar) Unlock() {}

func (PriorityTxSidecar) HeightForFiringAuction() int64 { return 0 }

func (PriorityTxSidecar) Flush(_ int64) {}
func (PriorityTxSidecar) Update(
	blockHeight int64,
	blockTxs types.Txs,
	_ []*abci.ResponseDeliverTx,
) error {
	return nil
}

func (PriorityTxSidecar) TxsAvailable() <-chan struct{} { return make(chan struct{}) }
func (PriorityTxSidecar) EnableTxsAvailable()           {}

func (PriorityTxSidecar) TxsWaitChan(_ int64) <-chan struct{} { return nil }

func (PriorityTxSidecar) TargetValidator() []byte { return []byte{} }

func (PriorityTxSidecar) Size(_ int64) int       { return 0 }
func (PriorityTxSidecar) TxsBytes(_ int64) int64 { return 0 }
