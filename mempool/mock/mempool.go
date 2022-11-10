package mock

import (
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/clist"
	"github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/types"
)

// Mempool is an empty implementation of a Mempool, useful for testing.
type Mempool struct{}

var _ mempool.Mempool = Mempool{}

func (Mempool) Lock()     {}
func (Mempool) Unlock()   {}
func (Mempool) Size() int { return 0 }
func (Mempool) CheckTx(_ types.Tx, _ func(*abci.Response), _ mempool.TxInfo) error {
	return nil
}
func (Mempool) RemoveTxByKey(txKey types.TxKey) error         { return nil }
func (Mempool) ReapMaxBytesMaxGas(_, _ int64) types.ReapedTxs { return types.ReapedTxs{} }
func (Mempool) ReapMaxTxs(n int) types.Txs                    { return types.Txs{} }
func (Mempool) Update(
	_ int64,
	_ types.Txs,
	_ []*abci.ResponseDeliverTx,
	_ mempool.PreCheckFunc,
	_ mempool.PostCheckFunc,
) error {
	return nil
}
func (Mempool) Flush()                        {}
func (Mempool) FlushAppConn() error           { return nil }
func (Mempool) TxsAvailable() <-chan struct{} { return make(chan struct{}) }
func (Mempool) EnableTxsAvailable()           {}
func (Mempool) SizeBytes() int64              { return 0 }

func (Mempool) TxsFront() *clist.CElement    { return nil }
func (Mempool) TxsWaitChan() <-chan struct{} { return nil }

func (Mempool) InitWAL() error { return nil }
func (Mempool) CloseWAL()      {}

// PriorityTxSidecar is an empty implementation of a PriorityTxSidecar, useful for testing.
type PriorityTxSidecar struct{}

var _ mempool.PriorityTxSidecar = PriorityTxSidecar{}

func (PriorityTxSidecar) AddTx(_ types.Tx, _ mempool.TxInfo) error { return nil }
func (PriorityTxSidecar) ReapMaxTxs() types.ReapedTxs              { return types.ReapedTxs{} }

func (PriorityTxSidecar) Lock()   {}
func (PriorityTxSidecar) Unlock() {}

func (PriorityTxSidecar) HeightForFiringAuction() int64 { return 0 }

func (PriorityTxSidecar) Flush() {}
func (PriorityTxSidecar) Update(
	blockHeight int64,
	blockTxs types.Txs,
	_ []*abci.ResponseDeliverTx,
) error {
	return nil
}

func (PriorityTxSidecar) TxsAvailable() <-chan struct{} { return make(chan struct{}) }
func (PriorityTxSidecar) EnableTxsAvailable()           {}

func (PriorityTxSidecar) TxsWaitChan() <-chan struct{} { return nil }

func (PriorityTxSidecar) TargetValidator() []byte { return []byte{} }

func (PriorityTxSidecar) Size() int       { return 0 }
func (PriorityTxSidecar) TxsBytes() int64 { return 0 }

func (PriorityTxSidecar) GetLastBundleHeight() int64 { return 0 }
