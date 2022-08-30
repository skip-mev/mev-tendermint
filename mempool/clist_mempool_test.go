package mempool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	gogotypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/abci/example/counter"
	"github.com/tendermint/tendermint/abci/example/kvstore"
	abciserver "github.com/tendermint/tendermint/abci/server"
	abci "github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	tmrand "github.com/tendermint/tendermint/libs/rand"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
)

// A cleanupFunc cleans up any config / test files created for a particular
// test.
type cleanupFunc func()

type testBundleInfo struct {
	BundleSize    int64
	DesiredHeight int64
	BundleId      int64
	PeerId        uint16
}

var (
	ZeroedTxInfoForSidecar = TxInfo{DesiredHeight: 1, BundleId: 0, BundleOrder: 0, BundleSize: 1}
)

func newMempoolWithApp(cc proxy.ClientCreator) (*CListMempool, *CListPriorityTxSidecar, cleanupFunc) {
	return newMempoolWithAppAndConfig(cc, cfg.ResetTestRoot("mempool_test"))
}

func newMempoolWithAppAndConfig(cc proxy.ClientCreator, config *cfg.Config) (*CListMempool, *CListPriorityTxSidecar, cleanupFunc) {
	appConnMem, _ := cc.NewABCIClient()
	appConnMem.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "mempool"))
	err := appConnMem.Start()
	if err != nil {
		panic(err)
	}
	mempool := NewCListMempool(config.Mempool, appConnMem, 0)
	sidecar := NewCListSidecar(0)
	mempool.SetLogger(log.TestingLogger())
	return mempool, sidecar, func() { os.RemoveAll(config.RootDir) }
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

func checkTxs(t *testing.T, mempool Mempool, count int, peerID uint16, sidecar PriorityTxSidecar, addToSidecar bool) types.Txs {
	txs := make(types.Txs, count)
	txInfo := TxInfo{SenderID: peerID}
	for i := 0; i < count; i++ {
		txBytes := make([]byte, 20)
		txs[i] = txBytes
		_, err := rand.Read(txBytes)
		if err != nil {
			t.Error(err)
		}
		if err := mempool.CheckTx(txBytes, nil, txInfo); err != nil {
			// Skip invalid txs.
			// TestMempoolFilters will fail otherwise. It asserts a number of txs
			// returned.
			if IsPreCheckError(err) {
				continue
			}
			t.Fatalf("CheckTx failed: %v while checking #%d tx", err, i)
		}
		if addToSidecar {
			sidecar.AddTx(txBytes, txInfo)
		}
	}
	return txs
}

func addNumBundlesToSidecar(t *testing.T, sidecar PriorityTxSidecar, numBundles int, bundleSize int64, peerID uint16) types.Txs {

	totalTxsCount := 0
	txs := make(types.Txs, 0)
	for i := 0; i < numBundles; i++ {
		totalTxsCount += int(bundleSize)
		newTxs := createSidecarBundleAndTxs(t, sidecar, testBundleInfo{BundleSize: bundleSize, PeerId: UnknownPeerID, DesiredHeight: sidecar.HeightForFiringAuction(), BundleId: int64(i)})
		txs = append(txs, newTxs...)
	}
	return txs
}

func addSpecificTxsToSidecarOneBundle(t *testing.T, sidecar PriorityTxSidecar, txs types.Txs, peerID uint16) types.Txs {

	bInfo := testBundleInfo{BundleSize: int64(len(txs)), PeerId: peerID, DesiredHeight: sidecar.HeightForFiringAuction(), BundleId: 0}
	for i := 0; i < len(txs); i++ {
		sidecar.AddTx(txs[i], TxInfo{SenderID: bInfo.PeerId, BundleSize: bInfo.BundleSize, BundleId: bInfo.BundleId, DesiredHeight: bInfo.DesiredHeight, BundleOrder: int64(i)})
	}
	return txs
}

func addNumTxsToSidecarOneBundle(t *testing.T, sidecar PriorityTxSidecar, numTxs int, peerID uint16) types.Txs {

	txs := make(types.Txs, numTxs)
	bInfo := testBundleInfo{BundleSize: int64(numTxs), PeerId: peerID, DesiredHeight: sidecar.HeightForFiringAuction(), BundleId: 0}
	for i := 0; i < numTxs; i++ {
		txBytes := addTxToSidecar(t, sidecar, bInfo, int64(i))
		txs = append(txs, txBytes)
	}
	return txs
}

func addTxToSidecar(t *testing.T, sidecar PriorityTxSidecar, bInfo testBundleInfo, bundleOrder int64) types.Tx {
	txInfo := TxInfo{SenderID: bInfo.PeerId, BundleSize: bInfo.BundleSize, BundleId: bInfo.BundleId, DesiredHeight: bInfo.DesiredHeight, BundleOrder: bundleOrder}
	txBytes := make([]byte, 20)
	_, err := rand.Read(txBytes)
	if err != nil {
		t.Error(err)
	}
	sidecar.AddTx(txBytes, txInfo)
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

func addBundlesToSidecar(t *testing.T, sidecar PriorityTxSidecar, bundles []testBundleInfo, peerId uint16) {

	for _, bundle := range bundles {
		// createSidecarBundleWithTxs(t, sidecar, bundle.BundleSize, peerId, bundle.BundleId, bundle.DesiredHeight)
		createSidecarBundleAndTxs(t, sidecar, bundle)
	}
}

func TestReapMaxBytesMaxGasMempool(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	// Ensure gas calculation behaves as expected
	checkTxs(t, mempool, 1, UnknownPeerID, sidecar, false)
	tx0 := mempool.TxsFront().Value.(*MempoolTx)
	// assert that kv store has gas wanted = 1.
	require.Equal(t, app.CheckTx(abci.RequestCheckTx{Tx: tx0.tx}).GasWanted, int64(1), "KVStore had a gas value neq to 1")
	require.Equal(t, tx0.gasWanted, int64(1), "transactions gas was set incorrectly")
	// ensure each tx is 20 bytes long
	require.Equal(t, len(tx0.tx), 20, "Tx is longer than 20 bytes")
	mempool.Flush()

	// each table driven test creates numTxsToCreate txs with checkTx, and at the end clears all remaining txs.
	// each tx has 20 bytes
	tests := []struct {
		numTxsToCreate int
		maxBytes       int64
		maxGas         int64
		expectedNumTxs int
	}{
		{1, 100, 100, 1},
		{20, -1, -1, 20},
		{20, -1, 0, 0},
		{20, -1, 10, 10},
		{20, -1, 30, 20},
		{20, 0, -1, 0},
		{20, 0, 10, 0},
		{20, 10, 10, 0},
		{20, 24, 10, 1},
		{20, 240, 5, 5},
		{20, 240, -1, 10},
		{20, 240, 10, 10},
		{20, 240, 15, 10},
		{20, 20000, -1, 20},
		{20, 20000, 5, 5},
		{20, 20000, 30, 20},
	}
	for tcIndex, tt := range tests {
		checkTxs(t, mempool, tt.numTxsToCreate, UnknownPeerID, sidecar, false)

		got := mempool.ReapMaxBytesMaxGas(tt.maxBytes, tt.maxGas, sidecar.ReapMaxTxs())
		assert.Equal(t, tt.expectedNumTxs, len(got), "Got %d txs, expected %d, tc #%d",
			len(got), tt.expectedNumTxs, tcIndex)
		mempool.Flush()
	}
}

func TestBasicAddMultipleBundles(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

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
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	// only one since no txs in first
	{
		// bundleSize, bundleHeight, bundleId
		bundles := []testBundleInfo{
			{0, 1, 0, 0},
			{5, 1, 1, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 1, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 1)
		sidecar.Flush()
	}

	// only one for first since later two out of range
	{
		// bundleSize, bundleHeight, bundleId
		bundles := []testBundleInfo{
			{5, 1, 0, 0},
			{5, 5, 1, 0},
			{5, 10, 1, 0},
		}
		addBundlesToSidecar(t, sidecar, bundles, UnknownPeerID)
		assert.Equal(t, 1, sidecar.NumBundles(), "Got %d bundles, expected %d",
			sidecar.NumBundles(), 1)
		sidecar.Flush()
	}

	// only one bundle since we already have all these bundleOrders
	{
		// bundleSize, bundleHeight, bundleId
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
		// bundleSize, bundleHeight, bundleId
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

// TODO: shorten this test
func TestReapSidecarWithTxsOutOfOrder(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	// 1. Inserted out of order, but sequential, but bundleSize 1, so should get one tx
	{
		bInfo := testBundleInfo{
			BundleSize:    1,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		var bundleOrder int64 = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    1,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		sidecar.PrettyPrintBundles()

		txs := sidecar.ReapMaxTxs()
		assert.Equal(t, 1, len(txs), "Got %d txs, expected %d",
			len(txs), 1)

		sidecar.Flush()
	}

	// 2. Same as before but now size is open to 2, so expect 2
	{
		bInfo := testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		var bundleOrder int64 = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs()
		assert.Equal(t, 2, len(txs), "Got %d txs, expected %d",
			len(txs), 2)

		sidecar.Flush()
	}

	// 3. Insert a bundle out of order and non sequential, so nothing should happen
	{
		bInfo := testBundleInfo{
			BundleSize:    5,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		var bundleOrder int64 = 3
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    5,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs()
		assert.Equal(t, 0, len(txs), "Got %d txs, expected %d",
			len(txs), 0)

		sidecar.Flush()
	}

	// 4. Insert three successful bundles out of order
	{
		bInfo := testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      2,
		}
		var bundleOrder int64 = 2
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      2,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      2,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      1,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      1,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs()
		assert.Equal(t, 7, len(txs), "Got %d txs, expected %d",
			len(txs), 7)
		sidecar.PrettyPrintBundles()

		fmt.Println("TXS FROM REAP ----------")
		for _, memTx := range txs {
			fmt.Println(fmt.Sprintf("%s", memTx.tx))
		}
		fmt.Println("----------")

		sidecar.Flush()
	}

	// 5. Multiple unsuccessful bundles, nothing reaped
	{
		// size not filled
		bInfo := testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      2,
		}
		var bundleOrder int64 = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      2,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		// wrong orders
		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 2
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    3,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 3
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		// ============

		// wrong heights
		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 2,
			BundleId:      1,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 0,
			BundleId:      1,
		}
		bundleOrder = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		txs := sidecar.ReapMaxTxs()
		assert.Equal(t, 0, len(txs), "Got %d txs, expected %d",
			len(txs), 0)
		sidecar.PrettyPrintBundles()

		fmt.Println("TXS FROM REAP ----------")
		for _, memTx := range txs {
			fmt.Println(fmt.Sprintf("%s", memTx.tx))
		}
		fmt.Println("----------")

		sidecar.Flush()
	}

}

// multiple go routines constantly try to insert bundles
// as they get reaped
func TestMempoolConcurrency(t *testing.T) {

	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	var wg sync.WaitGroup

	numProcesses := 15
	numBundlesToAddPerProcess := 5
	numTxPerBundle := 10
	wg.Add(numProcesses)

	for i := 0; i < numProcesses; i++ {

		go func() {
			defer wg.Done()
			addNumBundlesToSidecar(t, sidecar, numBundlesToAddPerProcess, int64(numTxPerBundle), UnknownPeerID)
		}()

	}

	wg.Wait()

	txs := sidecar.ReapMaxTxs()
	assert.Equal(t, (numBundlesToAddPerProcess * numTxPerBundle), len(txs), "Got %d txs, expected %d",
		len(txs), (numBundlesToAddPerProcess * numTxPerBundle))
}

func TestMempoolFilters(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()
	emptyTxArr := []types.Tx{[]byte{}}

	nopPreFilter := func(tx types.Tx) error { return nil }
	nopPostFilter := func(tx types.Tx, res *abci.ResponseCheckTx) error { return nil }

	// each table driven test creates numTxsToCreate txs with checkTx, and at the end clears all remaining txs.
	// each tx has 20 bytes
	tests := []struct {
		numTxsToCreate int
		preFilter      PreCheckFunc
		postFilter     PostCheckFunc
		expectedNumTxs int
	}{
		{10, nopPreFilter, nopPostFilter, 10},
		{10, PreCheckMaxBytes(10), nopPostFilter, 0},
		{10, PreCheckMaxBytes(22), nopPostFilter, 10},
		{10, nopPreFilter, PostCheckMaxGas(-1), 10},
		{10, nopPreFilter, PostCheckMaxGas(0), 0},
		{10, nopPreFilter, PostCheckMaxGas(1), 10},
		{10, nopPreFilter, PostCheckMaxGas(3000), 10},
		{10, PreCheckMaxBytes(10), PostCheckMaxGas(20), 0},
		{10, PreCheckMaxBytes(30), PostCheckMaxGas(20), 10},
		{10, PreCheckMaxBytes(22), PostCheckMaxGas(1), 10},
		{10, PreCheckMaxBytes(22), PostCheckMaxGas(0), 0},
	}
	for tcIndex, tt := range tests {
		err := mempool.Update(1, emptyTxArr, abciResponses(len(emptyTxArr), abci.CodeTypeOK), tt.preFilter, tt.postFilter)
		require.NoError(t, err)
		checkTxs(t, mempool, tt.numTxsToCreate, UnknownPeerID, sidecar, false)
		require.Equal(t, tt.expectedNumTxs, mempool.Size(), "mempool had the incorrect size, on test case %d", tcIndex)
		mempool.Flush()
	}
}

func TestMempoolUpdate(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, _, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	// 1. Adds valid txs to the cache
	{
		err := mempool.Update(1, []types.Tx{[]byte{0x01}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
		require.NoError(t, err)
		err = mempool.CheckTx([]byte{0x01}, nil, TxInfo{})
		if assert.Error(t, err) {
			assert.Equal(t, ErrTxInCache, err)
		}
	}

	// 2. Removes valid txs from the mempool
	{
		err := mempool.CheckTx([]byte{0x02}, nil, TxInfo{})
		require.NoError(t, err)
		err = mempool.Update(1, []types.Tx{[]byte{0x02}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
		require.NoError(t, err)
		assert.Zero(t, mempool.Size())
	}

	// 3. Removes invalid transactions from the cache and the mempool (if present)
	{
		err := mempool.CheckTx([]byte{0x03}, nil, TxInfo{})
		require.NoError(t, err)
		err = mempool.Update(1, []types.Tx{[]byte{0x03}}, abciResponses(1, 1), nil, nil)
		require.NoError(t, err)
		assert.Zero(t, mempool.Size())

		err = mempool.CheckTx([]byte{0x03}, nil, TxInfo{})
		require.NoError(t, err)
	}
}

func TestSidecarUpdate(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	// 1. Flushes the sidecar
	{
		bInfo := testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		var bundleOrder int64 = 0
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		bInfo = testBundleInfo{
			BundleSize:    2,
			PeerId:        UnknownPeerID,
			DesiredHeight: 1,
			BundleId:      0,
		}
		bundleOrder = 1
		addTxToSidecar(t, sidecar, bInfo, bundleOrder)

		err := sidecar.Update(0, []types.Tx{[]byte{0x02}}, abciResponses(1, abci.CodeTypeOK))
		require.NoError(t, err)
		assert.Zero(t, sidecar.Size())
	}
}

func TestMempool_KeepInvalidTxsInCache(t *testing.T) {
	app := counter.NewApplication(true)
	cc := proxy.NewLocalClientCreator(app)
	wcfg := cfg.DefaultConfig()
	wcfg.Mempool.KeepInvalidTxsInCache = true
	mempool, _, cleanup := newMempoolWithAppAndConfig(cc, wcfg)
	defer cleanup()

	// 1. An invalid transaction must remain in the cache after Update
	{
		a := make([]byte, 8)
		binary.BigEndian.PutUint64(a, 0)

		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, 1)

		err := mempool.CheckTx(b, nil, TxInfo{})
		require.NoError(t, err)

		// simulate new block
		_ = app.DeliverTx(abci.RequestDeliverTx{Tx: a})
		_ = app.DeliverTx(abci.RequestDeliverTx{Tx: b})
		err = mempool.Update(1, []types.Tx{a, b},
			[]*abci.ResponseDeliverTx{{Code: abci.CodeTypeOK}, {Code: 2}}, nil, nil)
		require.NoError(t, err)

		// a must be added to the cache
		err = mempool.CheckTx(a, nil, TxInfo{})
		if assert.Error(t, err) {
			assert.Equal(t, ErrTxInCache, err)
		}

		// b must remain in the cache
		err = mempool.CheckTx(b, nil, TxInfo{})
		if assert.Error(t, err) {
			assert.Equal(t, ErrTxInCache, err)
		}
	}

	// 2. An invalid transaction must remain in the cache
	{
		a := make([]byte, 8)
		binary.BigEndian.PutUint64(a, 0)

		// remove a from the cache to test (2)
		mempool.cache.Remove(a)

		err := mempool.CheckTx(a, nil, TxInfo{})
		require.NoError(t, err)

		err = mempool.CheckTx(a, nil, TxInfo{})
		if assert.Error(t, err) {
			assert.Equal(t, ErrTxInCache, err)
		}
	}
}

func TestTxsAvailable(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()
	mempool.EnableTxsAvailable()

	timeoutMS := 500

	// with no txs, it shouldnt fire
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch of txs, it should only fire once
	txs := checkTxs(t, mempool, 100, UnknownPeerID, sidecar, false)
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// call update with half the txs.
	// it should fire once now for the new height
	// since there are still txs left
	committedTxs, txs := txs[:50], txs[50:]
	if err := mempool.Update(1, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK), nil, nil); err != nil {
		t.Error(err)
	}
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch more txs. we already fired for this height so it shouldnt fire again
	moreTxs := checkTxs(t, mempool, 50, UnknownPeerID, sidecar, false)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// now call update with all the txs. it should not fire as there are no txs left
	committedTxs = append(txs, moreTxs...) //nolint: gocritic
	if err := mempool.Update(2, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK), nil, nil); err != nil {
		t.Error(err)
	}
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)

	// send a bunch more txs, it should only fire once
	checkTxs(t, mempool, 100, UnknownPeerID, sidecar, false)
	ensureFire(t, mempool.TxsAvailable(), timeoutMS)
	ensureNoFire(t, mempool.TxsAvailable(), timeoutMS)
}

func TestSidecarTxsAvailable(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	_, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()
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
	committedTxs, txs := txs[:50], txs[50:]

	// send a bunch more txs. we already fired for this height so it shouldnt fire again
	moreTxs := addNumBundlesToSidecar(t, sidecar, 50, 10, UnknownPeerID)
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// now call update with all the txs. it should not fire as there are no txs left
	committedTxs = append(txs, moreTxs...) //nolint: gocritic
	if err := sidecar.Update(2, committedTxs, abciResponses(len(committedTxs), abci.CodeTypeOK)); err != nil {
		t.Error(err)
	}
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)

	// send a bunch more txs, it should only fire once
	addNumBundlesToSidecar(t, sidecar, 100, 10, UnknownPeerID)
	ensureFire(t, sidecar.TxsAvailable(), timeoutMS)
	ensureNoFire(t, sidecar.TxsAvailable(), timeoutMS)
}

func TestSerialReap(t *testing.T) {
	app := counter.NewApplication(true)
	app.SetOption(abci.RequestSetOption{Key: "serial", Value: "on"})
	cc := proxy.NewLocalClientCreator(app)

	mempool, sidecar, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err := appConnCon.Start()
	require.Nil(t, err)

	cacheMap := make(map[string]struct{})
	deliverTxsRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {

			// This will succeed
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			err := mempool.CheckTx(txBytes, nil, TxInfo{})
			_, cached := cacheMap[string(txBytes)]
			if cached {
				require.NotNil(t, err, "expected error for cached tx")
			} else {
				require.Nil(t, err, "expected no err for uncached tx")
			}
			cacheMap[string(txBytes)] = struct{}{}

			// Duplicates are cached and should return error
			err = mempool.CheckTx(txBytes, nil, TxInfo{})
			require.NotNil(t, err, "Expected error after CheckTx on duplicated tx")
		}
	}

	reapCheck := func(exp int) {
		txs := mempool.ReapMaxBytesMaxGas(-1, -1, sidecar.ReapMaxTxs())
		require.Equal(t, len(txs), exp, fmt.Sprintf("Expected to reap %v txs but got %v", exp, len(txs)))
	}

	updateRange := func(start, end int) {
		txs := make([]types.Tx, 0)
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			txs = append(txs, txBytes)
		}
		if err := mempool.Update(0, txs, abciResponses(len(txs), abci.CodeTypeOK), nil, nil); err != nil {
			t.Error(err)
		}
	}

	commitRange := func(start, end int) {
		// Deliver some txs.
		for i := start; i < end; i++ {
			txBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(txBytes, uint64(i))
			res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
			if err != nil {
				t.Errorf("client error committing tx: %v", err)
			}
			if res.IsErr() {
				t.Errorf("error committing tx. Code:%v result:%X log:%v",
					res.Code, res.Data, res.Log)
			}
		}
		res, err := appConnCon.CommitSync()
		if err != nil {
			t.Errorf("client error committing: %v", err)
		}
		if len(res.Data) != 8 {
			t.Errorf("error committing. Hash:%X", res.Data)
		}
	}

	//----------------------------------------

	// Deliver some txs.
	deliverTxsRange(0, 100)

	// Reap the txs.
	reapCheck(100)

	// Reap again.  We should get the same amount
	reapCheck(100)

	// Deliver 0 to 999, we should reap 900 new txs
	// because 100 were already counted.
	deliverTxsRange(0, 1000)

	// Reap the txs.
	reapCheck(1000)

	// Reap again.  We should get the same amount
	reapCheck(1000)

	// Commit from the conensus AppConn
	commitRange(0, 500)
	updateRange(0, 500)

	// We should have 500 left.
	reapCheck(500)

	// Deliver 100 invalid txs and 100 valid txs
	deliverTxsRange(900, 1100)

	// We should have 600 now.
	reapCheck(600)
}

func TestMempoolCloseWAL(t *testing.T) {
	// 1. Create the temporary directory for mempool and WAL testing.
	rootDir, err := ioutil.TempDir("", "mempool-test")
	require.Nil(t, err, "expecting successful tmpdir creation")

	// 2. Ensure that it doesn't contain any elements -- Sanity check
	m1, err := filepath.Glob(filepath.Join(rootDir, "*"))
	require.Nil(t, err, "successful globbing expected")
	require.Equal(t, 0, len(m1), "no matches yet")

	// 3. Create the mempool
	wcfg := cfg.DefaultConfig()
	wcfg.Mempool.RootDir = rootDir
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempool, _, cleanup := newMempoolWithAppAndConfig(cc, wcfg)
	defer cleanup()
	mempool.height = 10
	err = mempool.InitWAL()
	require.NoError(t, err)

	// 4. Ensure that the directory contains the WAL file
	m2, err := filepath.Glob(filepath.Join(rootDir, "*"))
	require.Nil(t, err, "successful globbing expected")
	require.Equal(t, 1, len(m2), "expecting the wal match in")

	// 5. Write some contents to the WAL
	err = mempool.CheckTx(types.Tx([]byte("foo")), nil, TxInfo{})
	require.NoError(t, err)
	walFilepath := mempool.wal.Path
	sum1 := checksumFile(walFilepath, t)

	// 6. Sanity check to ensure that the written TX matches the expectation.
	require.Equal(t, sum1, checksumIt([]byte("foo\n")), "foo with a newline should be written")

	// 7. Invoke CloseWAL() and ensure it discards the
	// WAL thus any other write won't go through.
	mempool.CloseWAL()
	err = mempool.CheckTx(types.Tx([]byte("bar")), nil, TxInfo{})
	require.NoError(t, err)
	sum2 := checksumFile(walFilepath, t)
	require.Equal(t, sum1, sum2, "expected no change to the WAL after invoking CloseWAL() since it was discarded")

	// 8. Sanity check to ensure that the WAL file still exists
	m3, err := filepath.Glob(filepath.Join(rootDir, "*"))
	require.Nil(t, err, "successful globbing expected")
	require.Equal(t, 1, len(m3), "expecting the wal match in")
}

func TestMempool_CheckTxChecksTxSize(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	mempl, _, cleanup := newMempoolWithApp(cc)
	defer cleanup()

	maxTxSize := mempl.config.MaxTxBytes

	testCases := []struct {
		len int
		err bool
	}{
		// check small txs. no error
		0: {10, false},
		1: {1000, false},
		2: {1000000, false},

		// check around maxTxSize
		3: {maxTxSize - 1, false},
		4: {maxTxSize, false},
		5: {maxTxSize + 1, true},
	}

	for i, testCase := range testCases {
		caseString := fmt.Sprintf("case %d, len %d", i, testCase.len)

		tx := tmrand.Bytes(testCase.len)

		err := mempl.CheckTx(tx, nil, TxInfo{})
		bv := gogotypes.BytesValue{Value: tx}
		bz, err2 := bv.Marshal()
		require.NoError(t, err2)
		require.Equal(t, len(bz), proto.Size(&bv), caseString)

		if !testCase.err {
			require.NoError(t, err, caseString)
		} else {
			require.Equal(t, err, ErrTxTooLarge{maxTxSize, testCase.len}, caseString)
		}
	}
}

func TestMempoolTxsBytes(t *testing.T) {
	app := kvstore.NewApplication()
	cc := proxy.NewLocalClientCreator(app)
	config := cfg.ResetTestRoot("mempool_test")
	config.Mempool.MaxTxsBytes = 10
	mempool, _, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	// 1. zero by default
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 2. len(tx) after CheckTx
	err := mempool.CheckTx([]byte{0x01}, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, mempool.TxsBytes())

	// 3. zero again after tx is removed by Update
	err = mempool.Update(1, []types.Tx{[]byte{0x01}}, abciResponses(1, abci.CodeTypeOK), nil, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 4. zero after Flush
	err = mempool.CheckTx([]byte{0x02, 0x03}, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, mempool.TxsBytes())

	mempool.Flush()
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 5. ErrMempoolIsFull is returned when/if MaxTxsBytes limit is reached.
	err = mempool.CheckTx([]byte{0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04}, nil, TxInfo{})
	require.NoError(t, err)
	err = mempool.CheckTx([]byte{0x05}, nil, TxInfo{})
	if assert.Error(t, err) {
		assert.IsType(t, ErrMempoolIsFull{}, err)
	}

	// 6. zero after tx is rechecked and removed due to not being valid anymore
	app2 := counter.NewApplication(true)
	cc = proxy.NewLocalClientCreator(app2)
	mempool, _, cleanup = newMempoolWithApp(cc)
	defer cleanup()

	txBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(txBytes, uint64(0))

	err = mempool.CheckTx(txBytes, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 8, mempool.TxsBytes())

	appConnCon, _ := cc.NewABCIClient()
	appConnCon.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "consensus"))
	err = appConnCon.Start()
	require.Nil(t, err)
	t.Cleanup(func() {
		if err := appConnCon.Stop(); err != nil {
			t.Error(err)
		}
	})
	res, err := appConnCon.DeliverTxSync(abci.RequestDeliverTx{Tx: txBytes})
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Code)
	res2, err := appConnCon.CommitSync()
	require.NoError(t, err)
	require.NotEmpty(t, res2.Data)

	// Pretend like we committed nothing so txBytes gets rechecked and removed.
	err = mempool.Update(1, []types.Tx{}, abciResponses(0, abci.CodeTypeOK), nil, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 0, mempool.TxsBytes())

	// 7. Test RemoveTxByKey function
	err = mempool.CheckTx([]byte{0x06}, nil, TxInfo{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, mempool.TxsBytes())
	mempool.RemoveTxByKey(TxKey([]byte{0x07}), true)
	assert.EqualValues(t, 1, mempool.TxsBytes())
	mempool.RemoveTxByKey(TxKey([]byte{0x06}), true)
	assert.EqualValues(t, 0, mempool.TxsBytes())

}

// This will non-deterministically catch some concurrency failures like
// https://github.com/tendermint/tendermint/issues/3509
// TODO: all of the tests should probably also run using the remote proxy app
// since otherwise we're not actually testing the concurrency of the mempool here!
func TestMempoolRemoteAppConcurrency(t *testing.T) {
	sockPath := fmt.Sprintf("unix:///tmp/echo_%v.sock", tmrand.Str(6))
	app := kvstore.NewApplication()
	cc, server := newRemoteApp(t, sockPath, app)
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Error(err)
		}
	})
	config := cfg.ResetTestRoot("mempool_test")
	mempool, _, cleanup := newMempoolWithAppAndConfig(cc, config)
	defer cleanup()

	// generate small number of txs
	nTxs := 10
	txLen := 200
	txs := make([]types.Tx, nTxs)
	for i := 0; i < nTxs; i++ {
		txs[i] = tmrand.Bytes(txLen)
	}

	// simulate a group of peers sending them over and over
	N := config.Mempool.Size
	maxPeers := 5
	for i := 0; i < N; i++ {
		peerID := mrand.Intn(maxPeers)
		txNum := mrand.Intn(nTxs)
		tx := txs[txNum]

		// this will err with ErrTxInCache many times ...
		mempool.CheckTx(tx, nil, TxInfo{SenderID: uint16(peerID)}) //nolint: errcheck // will error
	}
	err := mempool.FlushAppConn()
	require.NoError(t, err)
}

// caller must close server
func newRemoteApp(
	t *testing.T,
	addr string,
	app abci.Application,
) (
	clientCreator proxy.ClientCreator,
	server service.Service,
) {
	clientCreator = proxy.NewRemoteClientCreator(addr, "socket", true)

	// Start server
	server = abciserver.NewSocketServer(addr, app)
	server.SetLogger(log.TestingLogger().With("module", "abci-server"))
	if err := server.Start(); err != nil {
		t.Fatalf("Error starting socket server: %v", err.Error())
	}
	return clientCreator, server
}
func checksumIt(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func checksumFile(p string, t *testing.T) string {
	data, err := ioutil.ReadFile(p)
	require.Nil(t, err, "expecting successful read of %q", p)
	return checksumIt(data)
}

func abciResponses(n int, code uint32) []*abci.ResponseDeliverTx {
	responses := make([]*abci.ResponseDeliverTx, 0, n)
	for i := 0; i < n; i++ {
		responses = append(responses, &abci.ResponseDeliverTx{Code: code})
	}
	return responses
}
