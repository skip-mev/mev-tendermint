package v1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log/term"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tendermint/tendermint/abci/example/kvstore"
	"github.com/tendermint/tendermint/p2p/mock"

	cfg "github.com/tendermint/tendermint/config"

	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/mev"
	"github.com/tendermint/tendermint/p2p"
	memproto "github.com/tendermint/tendermint/proto/tendermint/mempool"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
)

const (
	numTxs  = 1000
	timeout = 120 * time.Second // ridiculously high because CircleCI is slow
)

type peerState struct {
	height int64
}

func (ps peerState) GetHeight() int64 {
	return ps.height
}

// Send a bunch of txs to the first reactor's mempool and wait for them all to
// be received in the others.
func TestReactorBroadcastTxsMessage(t *testing.T) {
	config := cfg.TestConfig()
	// if there were more than two reactors, the order of transactions could not be
	// asserted in waitForTxsOnReactors (due to transactions gossiping). If we
	// replace Connect2Switches (full mesh) with a func, which connects first
	// reactor to others and nothing else, this test should also pass with >2 reactors.
	const N = 2
	reactors := makeAndConnectReactors(config, N)
	defer func() {
		for _, r := range reactors {
			if err := r.Stop(); err != nil {
				assert.NoError(t, err)
			}
		}
	}()
	for _, r := range reactors {
		for _, peer := range r.Switch.Peers().List() {
			peer.Set(types.PeerStateKey, peerState{1})
		}
	}

	txs := checkTxs(t, reactors[0].mempool, numTxs, mempool.UnknownPeerID)
	transactions := make(types.Txs, len(txs))
	for idx, tx := range txs {
		transactions[idx] = tx.tx
	}

	waitForTxsOnReactors(t, transactions, reactors, false)
}

func TestMempoolVectors(t *testing.T) {
	testCases := []struct {
		testName string
		tx       []byte
		expBytes string
	}{
		{"tx 1", []byte{123}, "0a030a017b"},
		{"tx 2", []byte("proto encoding in mempool"), "0a1b0a1970726f746f20656e636f64696e6720696e206d656d706f6f6c"},
	}

	for _, tc := range testCases {
		tc := tc

		msg := memproto.Message{
			Sum: &memproto.Message_Txs{
				Txs: &memproto.Txs{Txs: [][]byte{tc.tx}},
			},
		}
		bz, err := msg.Marshal()
		require.NoError(t, err, tc.testName)

		require.Equal(t, tc.expBytes, hex.EncodeToString(bz), tc.testName)
	}
}

func TestLegacyReactorReceiveBasic(t *testing.T) {
	config := cfg.TestConfig()
	// if there were more than two reactors, the order of transactions could not be
	// asserted in waitForTxsOnReactors (due to transactions gossiping). If we
	// replace Connect2Switches (full mesh) with a func, which connects first
	// reactor to others and nothing else, this test should also pass with >2 reactors.
	const N = 1
	reactors := makeAndConnectReactors(config, N)
	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
	)
	defer func() {
		err := reactor.Stop()
		assert.NoError(t, err)
	}()

	reactor.InitPeer(peer)
	reactor.AddPeer(peer)
	m := &memproto.Txs{}
	wm := m.Wrap()
	msg, err := proto.Marshal(wm)
	assert.NoError(t, err)

	assert.NotPanics(t, func() {
		reactor.Receive(mempool.MempoolChannel, peer, msg)
	})
}

func TestLegacyReactorReceiveSidecarMEVTxs(t *testing.T) {
	config := cfg.TestConfig()
	const N = 1
	reactors := makeAndConnectReactors(config, N)
	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
	)
	defer func() {
		err := reactor.Stop()
		assert.NoError(t, err)
	}()

	reactor.InitPeer(peer)
	reactor.AddPeer(peer)
	txBytes := make([]byte, 20)
	m := &memproto.MEVTxs{
		Txs:           [][]byte{txBytes},
		DesiredHeight: 1,
		BundleId:      0,
		BundleOrder:   0,
		BundleSize:    1,
	}
	wm := m.Wrap()
	msg, err := proto.Marshal(wm)
	assert.NoError(t, err)
	assert.NotPanics(t, func() {
		reactor.Receive(mempool.SidecarLegacyChannel, peer, msg)
		reactor.Receive(mempool.SidecarChannel, peer, msg)
		waitForSidecarTxsOnReactor(t, []types.Tx{txBytes}, reactor, 0)
	})
}

func TestReactorReceiveSidecarMEVTxs(t *testing.T) {
	config := cfg.TestConfig()
	const N = 1
	reactors := makeAndConnectReactors(config, N)
	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
	)
	defer func() {
		err := reactor.Stop()
		assert.NoError(t, err)
	}()

	reactor.InitPeer(peer)
	reactor.AddPeer(peer)
	txBytes := make([]byte, 20)
	m := &memproto.MEVTxs{
		Txs:           [][]byte{txBytes},
		DesiredHeight: 1,
		BundleId:      0,
		BundleOrder:   0,
		BundleSize:    1,
	}
	assert.NotPanics(t, func() {
		reactor.ReceiveEnvelope(p2p.Envelope{
			ChannelID: mempool.SidecarChannel,
			Src:       peer,
			Message:   m,
		})
		reactor.ReceiveEnvelope(p2p.Envelope{
			ChannelID: mempool.SidecarLegacyChannel,
			Src:       peer,
			Message:   m,
		})
		waitForSidecarTxsOnReactor(t, []types.Tx{txBytes}, reactor, 0)
	})
}

func TestReactorReceiveSidecarMEVMessage(t *testing.T) {
	config := cfg.TestConfig()
	const N = 1
	reactors := makeAndConnectReactors(config, N)
	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
	)
	defer func() {
		err := reactor.Stop()
		assert.NoError(t, err)
	}()

	reactor.InitPeer(peer)
	reactor.AddPeer(peer)
	txBytes := make([]byte, 20)
	msg := &memproto.MEVMessage{
		Sum: &memproto.MEVMessage_Txs{
			Txs: &memproto.Txs{Txs: [][]byte{txBytes}},
		},
		DesiredHeight: 1,
		BundleId:      0,
		BundleOrder:   0,
		BundleSize:    1,
	}

	assert.NotPanics(t, func() {
		reactor.ReceiveEnvelope(p2p.Envelope{
			ChannelID: mempool.SidecarChannel,
			Src:       peer,
			Message:   msg,
		})
		reactor.ReceiveEnvelope(p2p.Envelope{
			ChannelID: mempool.SidecarLegacyChannel,
			Src:       peer,
			Message:   msg,
		})
		waitForSidecarTxsOnReactor(t, []types.Tx{txBytes}, reactor, 0)
	})
}

func TestLegacyReactorReceiveSidecarMEVMessage(t *testing.T) {
	config := cfg.TestConfig()
	const N = 1
	reactors := makeAndConnectReactors(config, N)
	var (
		reactor = reactors[0]
		peer    = mock.NewPeer(nil)
	)
	defer func() {
		err := reactor.Stop()
		assert.NoError(t, err)
	}()

	reactor.InitPeer(peer)
	reactor.AddPeer(peer)
	txBytes := make([]byte, 20)
	msg := &memproto.MEVMessage{
		Sum: &memproto.MEVMessage_Txs{
			Txs: &memproto.Txs{Txs: [][]byte{txBytes}},
		},
		DesiredHeight: 1,
		BundleId:      0,
		BundleOrder:   0,
		BundleSize:    1,
	}

	mm, err := proto.Marshal(msg)
	assert.NoError(t, err)
	assert.NotPanics(t, func() {
		reactor.Receive(mempool.SidecarLegacyChannel, peer, mm)
		reactor.Receive(mempool.SidecarChannel, peer, mm)
		fmt.Println(reactor.sidecar.Size())
		waitForSidecarTxsOnReactor(t, []types.Tx{txBytes}, reactor, 0)
	})
}

func makeAndConnectReactors(config *cfg.Config, n int) []*Reactor {
	reactors := make([]*Reactor, n)
	logger := mempoolLogger()
	for i := 0; i < n; i++ {
		app := kvstore.NewApplication()
		cc := proxy.NewLocalClientCreator(app)
		sidecar := mempool.NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())
		mempool, cleanup := newMempoolWithApp(cc)
		defer cleanup()

		reactors[i] = NewReactor(config.Mempool, mempool, sidecar) // so we dont start the consensus states
		reactors[i].SetLogger(logger.With("validator", i))
	}

	p2p.MakeConnectedSwitches(config.P2P, n, func(i int, s *p2p.Switch) *p2p.Switch {
		s.AddReactor("MEMPOOL", reactors[i])
		return s

	}, p2p.Connect2Switches)
	return reactors
}

// mempoolLogger is a TestingLogger which uses a different
// color for each validator ("validator" key must exist).
func mempoolLogger() log.Logger {
	return log.TestingLoggerWithColorFn(func(keyvals ...interface{}) term.FgBgColor {
		for i := 0; i < len(keyvals)-1; i += 2 {
			if keyvals[i] == "validator" {
				return term.FgBgColor{Fg: term.Color(uint8(keyvals[i+1].(int) + 1))}
			}
		}
		return term.FgBgColor{}
	})
}

func newMempoolWithApp(cc proxy.ClientCreator) (*TxMempool, func()) {
	conf := cfg.ResetTestRoot("mempool_test")

	mp, cu := newMempoolWithAppAndConfig(cc, conf)
	return mp, cu
}

func newMempoolWithAppAndConfig(cc proxy.ClientCreator, conf *cfg.Config) (*TxMempool, func()) {
	appConnMem, _ := cc.NewABCIClient()
	appConnMem.SetLogger(log.TestingLogger().With("module", "abci-client", "connection", "mempool"))
	err := appConnMem.Start()
	if err != nil {
		panic(err)
	}

	mp := NewTxMempool(log.TestingLogger(), conf.Mempool, appConnMem, 0)

	return mp, func() { os.RemoveAll(conf.RootDir) }
}

func waitForTxsOnReactors(t *testing.T, txs types.Txs, reactors []*Reactor, useSidecar bool) {
	// wait for the txs in all mempools
	wg := new(sync.WaitGroup)
	for i, reactor := range reactors {
		wg.Add(1)
		go func(r *Reactor, reactorIndex int) {
			defer wg.Done()
			if useSidecar {
				waitForSidecarTxsOnReactor(t, txs, r, reactorIndex)
			} else {
				waitForTxsOnReactor(t, txs, r, reactorIndex)
			}
		}(reactor, i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.After(timeout)
	select {
	case <-timer:
		t.Fatal("Timed out waiting for txs")
	case <-done:
	}
}

func waitForTxsOnReactor(t *testing.T, txs types.Txs, reactor *Reactor, reactorIndex int) {
	mempool := reactor.mempool
	for mempool.Size() < len(txs) {
		time.Sleep(time.Millisecond * 100)
	}

	reapedTxs := mempool.ReapMaxTxs(len(txs))
	for i, tx := range txs {
		assert.Equalf(t, tx, reapedTxs[i],
			"txs at index %d on reactor %d don't match: %v vs %v", i, reactorIndex, tx, reapedTxs[i])
	}
}

func waitForSidecarTxsOnReactor(t *testing.T, txs types.Txs, reactor *Reactor, reactorIndex int) {
	sidecar := reactor.sidecar
	for sidecar.Size() < len(txs) {
		time.Sleep(time.Millisecond * 100)
	}

	reapedTxs := sidecar.ReapMaxTxs()
	var i int
	for _, scTx := range reapedTxs.Txs {
		assert.Equalf(t, txs[i], scTx,
			"txs at index %d on reactor %d don't match: %s vs %s", i, reactorIndex, txs[i], scTx)
		i++
	}
}

func TestReactorBroadcastSidecarOnly(t *testing.T) {
	config := cfg.TestConfig()
	const N = 8
	reactors := makeAndConnectReactorsEvensSidecar(config, N)
	defer func() {
		for _, r := range reactors {
			if err := r.Stop(); err != nil {
				assert.NoError(t, err)
			}
		}
	}()
	for _, r := range reactors {
		for _, peer := range r.Switch.Peers().List() {
			peer.Set(types.PeerStateKey, peerState{1})
		}
	}
	txs := addNumBundlesToSidecar(t, reactors[0].sidecar, 5, 10, mempool.UnknownPeerID)
	time.Sleep(2000)
	reactors[0].sidecar.PrettyPrintBundles()
	waitForTxsOnReactors(t, txs, reactors[2:3], true)
	waitForTxsOnReactors(t, txs, reactors[4:5], true)
	waitForTxsOnReactors(t, txs, reactors[6:7], true)
	assert.Equal(t, 0, reactors[1].sidecar.Size())
	assert.Equal(t, 0, reactors[5].sidecar.Size())
	assert.Equal(t, 0, reactors[7].sidecar.Size())
	assert.Equal(t, 0, reactors[3].sidecar.Size())
}

// Send a bunch of txs to the first reactor's sidecar and wait for them all to
// be received in the others, IN THE RIGHT ORDER
func TestReactorBroadcastSidecarTxsMessage(t *testing.T) {
	config := cfg.TestConfig()
	const N = 2
	reactors := makeAndConnectReactors(config, N)
	defer func() {
		for _, r := range reactors {
			if err := r.Stop(); err != nil {
				assert.NoError(t, err)
			}
		}
	}()
	for _, r := range reactors {
		for _, peer := range r.Switch.Peers().List() {
			peer.Set(types.PeerStateKey, peerState{1})
		}
	}
	txs := addNumBundlesToSidecar(t, reactors[0].sidecar, 5, 10, mempool.UnknownPeerID)
	time.Sleep(2000)
	reactors[0].sidecar.PrettyPrintBundles()
	waitForTxsOnReactors(t, txs, reactors, true)
	reactors[1].sidecar.PrettyPrintBundles()
}

func TestReactorInsertOutOfOrderThenReap(t *testing.T) {
	config := cfg.TestConfig()
	const N = 2
	reactors := makeAndConnectReactors(config, N)
	defer func() {
		for _, r := range reactors {
			if err := r.Stop(); err != nil {
				assert.NoError(t, err)
			}
		}
	}()
	for _, r := range reactors {
		for _, peer := range r.Switch.Peers().List() {
			peer.Set(types.PeerStateKey, peerState{1})
		}
	}
	txs := addNumBundlesToSidecar(t, reactors[0].sidecar, 5, 10, mempool.UnknownPeerID)
	time.Sleep(2000)
	reactors[0].sidecar.PrettyPrintBundles()
	waitForTxsOnReactors(t, txs, reactors, true)
	reactors[1].sidecar.PrettyPrintBundles()
}

// connect N mempool reactors through N switches
// can add additional logic to set which ones should be treated as sidecar
// peers in p2p.Connect2Switches, including based on index
func makeAndConnectReactorsEvensSidecar(config *cfg.Config, n int) []*Reactor {
	reactors := make([]*Reactor, n)
	logger := mempoolLogger()
	for i := 0; i < n; i++ {
		app := kvstore.NewApplication()
		cc := proxy.NewLocalClientCreator(app)
		sidecar := mempool.NewCListSidecar(0, log.NewNopLogger(), mev.NopMetrics())
		mempool, cleanup := newMempoolWithApp(cc)
		defer cleanup()

		reactors[i] = NewReactor(config.Mempool, mempool, sidecar) // so we dont start the consensus states
		reactors[i].SetLogger(logger.With("validator", i))
	}

	p2p.MakeConnectedSwitches(config.P2P, n, func(i int, s *p2p.Switch) *p2p.Switch {
		s.AddReactor("MEMPOOL", reactors[i])
		return s

	}, p2p.Connect2SwitchesEvensSidecar)
	return reactors
}

// Sidecar testing utils

type testBundleInfo struct {
	BundleSize    int64
	DesiredHeight int64
	BundleID      int64
	PeerID        uint16
}

func addNumBundlesToSidecar(t *testing.T, sidecar mempool.PriorityTxSidecar, numBundles int, bundleSize int64, peerID uint16) types.Txs {
	totalTxsCount := 0
	txs := make(types.Txs, 0)
	for i := 0; i < numBundles; i++ {
		totalTxsCount += int(bundleSize)
		newTxs := createSidecarBundleAndTxs(t, sidecar, testBundleInfo{BundleSize: bundleSize,
			PeerID: mempool.UnknownPeerID, DesiredHeight: sidecar.HeightForFiringAuction(), BundleID: int64(i)})
		txs = append(txs, newTxs...)
	}
	return txs
}

func createSidecarBundleAndTxs(t *testing.T, sidecar mempool.PriorityTxSidecar, bInfo testBundleInfo) types.Txs {
	txs := make(types.Txs, bInfo.BundleSize)
	for i := 0; i < int(bInfo.BundleSize); i++ {
		txBytes := addTxToSidecar(t, sidecar, bInfo, int64(i))
		txs[i] = txBytes
	}
	return txs
}

func addTxToSidecar(t *testing.T, sidecar mempool.PriorityTxSidecar, bInfo testBundleInfo, bundleOrder int64) types.Tx {
	txInfo := mempool.TxInfo{SenderID: bInfo.PeerID, BundleSize: bInfo.BundleSize,
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
