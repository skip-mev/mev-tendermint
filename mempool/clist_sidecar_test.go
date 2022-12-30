package mempool

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/mev"
	"github.com/tendermint/tendermint/types"
)

type testBundleInfo struct {
	BundleSize    int64
	DesiredHeight int64
	BundleID      int64
	PeerID        uint16
}

func addNumBundlesToSidecar(t *testing.T, sidecar PriorityTxSidecar, numBundles int, bundleSize int64, peerID uint16) types.Txs {
	totalTxsCount := 0
	txs := make(types.Txs, 0)
	for i := 0; i < numBundles; i++ {
		totalTxsCount += int(bundleSize)
		newTxs := createSidecarBundleAndTxs(t, sidecar, testBundleInfo{BundleSize: bundleSize,
			PeerID: UnknownPeerID, DesiredHeight: sidecar.HeightForFiringAuction(), BundleID: int64(i)})
		txs = append(txs, newTxs...)
	}
	return txs
}

func addTxToSidecar(t *testing.T, sidecar PriorityTxSidecar, bInfo testBundleInfo, bundleOrder int64) types.Tx {
	txInfo := TxInfo{SenderID: bInfo.PeerID, BundleSize: bInfo.BundleSize,
		BundleID: bInfo.BundleID, DesiredHeight: bInfo.DesiredHeight, BundleOrder: bundleOrder}
	txBytes := make([]byte, 20)
	_, err := rand.Read(txBytes)
	if err != nil {
		t.Error(err)
	}
	if err := sidecar.AddTx(txBytes, txInfo); err != nil {
		fmt.Println("Ignoring error in AddTx:", err)
	}
	return txBytes
}

func createSidecarBundleAndTxs(t *testing.T, sidecar PriorityTxSidecar, bInfo testBundleInfo) types.Txs {
	txs := make(types.Txs, bInfo.BundleSize)
	for i := 0; i < int(bInfo.BundleSize); i++ {
		txBytes := addTxToSidecar(t, sidecar, bInfo, int64(i))
		txs[i] = txBytes
	}
	return txs
}

func addBundlesToSidecar(t *testing.T, sidecar PriorityTxSidecar, bundles []testBundleInfo, peerID uint16) {
	for _, bundle := range bundles {
		// createSidecarBundleWithTxs(t, sidecar, bundle.BundleSize, peerID, bundle.BundleID, bundle.DesiredHeight)
		createSidecarBundleAndTxs(t, sidecar, bundle)
	}
}

func TestSidecarUpdate(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	// 1. Flushes the sidecar
	{
		bInfo := testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		var bundleOrder int64
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		err := sidecar.Update(0, []types.Tx{[]byte{0x02}}, abciResponses(1, abci.CodeTypeOK))
		require.NoError(t, err)
		require.Equal(t, 2, sidecar.Size(), "foo with a newline should be written")
		err = sidecar.Update(1, []types.Tx{[]byte{0x02}}, abciResponses(1, abci.CodeTypeOK))
		require.NoError(t, err)
		require.Equal(t, 0, sidecar.Size(), "foo with a newline should be written")
	}
}

func TestSidecarTxsAvailable(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())
	sidecar.EnableTxsAvailable()

	timeoutMS := 500

	// with no txs, it shouldnt fire
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// send a bunch of txs, it should only fire once
	txs := addNumBundlesToSidecar(t, sidecar, 100, 10, UnknownPeerID)
	ensureFire(t, sidecar.TxsAvailable(), timeoutMS)
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// call update with half the txs.
	// it should fire once now for the new height
	// since there are still txs left
	txs = txs[50:]

	// send a bunch more txs. we already fired for this height so it shouldnt fire again
	moreTxs := addNumBundlesToSidecar(t, sidecar, 50, 10, UnknownPeerID)
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// now call update with all the txs. it should not fire as there are no txs left
	committedTxs := append(txs, moreTxs...) //nolint: gocritic
	if err := sidecar.Update(2, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK)); err != nil {
		t.Error(err)
	}
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// send a bunch more txs, it should only fire once
	addNumBundlesToSidecar(t, sidecar, 100, 10, UnknownPeerID)
	ensureFire(t, sidecar.TxsAvailable(), timeoutMS)
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)
}

// TODO: shorten
func TestReapSidecarWithTxsOutOfOrder(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	// 1. Inserted out of order, but sequential, but bundleSize 1, so should get one tx
	{
		bInfo := testBundleInfo{
			BundleSize:    1,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		var bundleOrder int64 = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    1,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		sidecar.PrettyPrintBundles()

		txs := sidecar.ReapMaxTxs().Txs
		assert.Equal(t, 1, len(txs), "Got %d txs, expected %d",
			len(txs), 1)

		sidecar.Flush()
	}

	// 2. Same as before but now size is open to 2, so expect 2
	{
		bInfo := testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		var bundleOrder int64 = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs().Txs
		assert.Equal(t, 2, len(txs), "Got %d txs, expected %d",
			len(txs), 2)

		sidecar.Flush()
	}

	// 3. Insert a bundle out of order and non sequential, so nothing should happen
	{
		bInfo := testBundleInfo{
			BundleSize:    5,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		var bundleOrder int64 = 3
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    5,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs().Txs
		assert.Equal(t, 0, len(txs), "Got %d txs, expected %d",
			len(txs), 0)

		sidecar.Flush()
	}

	// 4. Insert three successful bundles out of order
	{
		bInfo := testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      2,
		}
		var bundleOrder int64 = 2
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      2,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      2,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      1,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      1,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs().Txs
		assert.Equal(t, 7, len(txs), "Got %d txs, expected %d",
			len(txs), 7)
		sidecar.PrettyPrintBundles()

		fmt.Println("TXS FROM REAP ----------")
		for _, memTx := range txs {
			fmt.Println(memTx.String())
		}
		fmt.Println("----------")

		sidecar.Flush()
	}

	// 5. Multiple unsuccessful bundles, nothing reaped
	{
		// size not filled
		bInfo := testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      2,
		}
		var bundleOrder int64
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      2,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		// wrong orders
		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 2
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerID:        UnknownPeerID,
			DesiredHeight: 1,
			BundleID:      0,
		}
		bundleOrder = 3
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		// wrong heights
		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 2,
			BundleID:      1,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerID:        UnknownPeerID,
			DesiredHeight: 0,
			BundleID:      1,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs().Txs
		assert.Equal(t, 0, len(txs), "Got %d txs, expected %d",
			len(txs), 0)
		sidecar.PrettyPrintBundles()

		fmt.Println("TXS FROM REAP ----------")
		for _, memTx := range txs {
			fmt.Println(memTx.String())
		}
		fmt.Println("----------")

		sidecar.Flush()
	}
}

func TestBasicAddMultipleBundles(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	tests := []struct {
		numBundlesTxsToCreate int
	}{
		{0},
		{1},
		{5},
		{0},
		{100},
	}
	for tcIndex, tt := range tests {
		fmt.Println("Num bundles to create: ", tt.numBundlesTxsToCreate)
		addNumBundlesToSidecar(t, sidecar, tt.numBundlesTxsToCreate, 10, UnknownPeerID)
		sidecar.ReapMaxTxs()
		assert.Equal(t, tt.numBundlesTxsToCreate, sidecar.NumBundles(), "Got %d bundles, expected %d, tc #%d",
			sidecar.NumBundles(), tt.numBundlesTxsToCreate, tcIndex)
		sidecar.Flush()
	}
}

func TestSpecificAddTxsToMultipleBundles(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	// only one since no txs in first
	{
		// bundleSize, bundleHeight, bundleID, peerID
		bundles := []testBundleInfo{
			{0, 1, 0, 0},
			{5, 1, 1, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 1, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 1)
		sidecar.Flush()
	}

	// three bundles
	{
		// bundleSize, bundleHeight, bundleID, peerID
		bundles := []testBundleInfo{
			{5, 1, 0, 0},
			{5, 5, 1, 0},
			{5, 10, 1, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 3, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 3)
		sidecar.Flush()
	}

	// only one bundle since we already have all these bundleOrders
	{
		// bundleSize, bundleHeight, bundleID, peerID
		bundles := []testBundleInfo{
			{5, 1, 0, 0},
			{5, 1, 0, 0},
			{5, 1, 0, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 1, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 1)
		sidecar.Flush()
	}

	// only one bundle since we already have all these bundleOrders
	{
		// bundleSize, bundleHeight, BundleID, peerID
		bundles := []testBundleInfo{
			{5, 1, 0, 0},
			{5, 1, 3, 0},
			{5, 1, 5, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 3, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 3)
		sidecar.Flush()
	}
}

func TestGetEnforcedBundleSize(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	assert.Equal(t, 0, sidecar.GetEnforcedBundleSize(0), "Expected enforced bundle size %d, got %d", 0, sidecar.GetEnforcedBundleSize(0))

	bundles := []testBundleInfo{
		{5, 1, 0, 0},
	}
	addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
	assert.Equal(t, 5, sidecar.GetEnforcedBundleSize(0), "Expected enforced bundle size %d, got %d", 5, sidecar.GetEnforcedBundleSize(0))
}

func TestGetCurrBundleSize(t *testing.T) {
	sidecar := NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())

	assert.Equal(t, 0, sidecar.GetCurrBundleSize(0), "Expected curr bundle size %d, got %d", 0, sidecar.GetCurrBundleSize(0))

	bundles := []testBundleInfo{
		{5, 1, 0, 0},
	}
	addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
	assert.Equal(t, 5, sidecar.GetCurrBundleSize(0), "Expected curr bundle size %d, got %d", 5, sidecar.GetCurrBundleSize(0))
}

func abciResponses(n int, code uint32) []*abci.ResponseDeliverTx {
	responses := make([]*abci.ResponseDeliverTx, 0, n)
	for i := 0; i < n; i++ {
		responses = append(responses, &abci.ResponseDeliverTx{Code: code})
	}
	return responses
}

func ensureNoFire(t *testing.T, ch <-chan struct{}, timeoutMS int) {
	timer := time.NewTimer(time.Duration(timeoutMS) * time.Millisecond)
	select {
	case <-ch:
		t.Fatal("Expected not to fire")
	case <-timer.C:
	}
}

func ensureFire(t *testing.T, ch <-chan struct{}, timeoutMS int) {
	timer := time.NewTimer(time.Duration(timeoutMS) * time.Millisecond)
	select {
	case <-ch:
	case <-timer.C:
		t.Fatal("Expected to fire")
	}
}
