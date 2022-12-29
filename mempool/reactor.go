package mempool

import (
	"errors"
	"fmt"
	"math"
	"time"

	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/clist"
	"github.com/tendermint/tendermint/libs/log"
	tmsync "github.com/tendermint/tendermint/libs/sync"
	"github.com/tendermint/tendermint/p2p"
	protomem "github.com/tendermint/tendermint/proto/tendermint/mempool"
	"github.com/tendermint/tendermint/types"
)

const (
	MempoolChannel = byte(0x30)

	SidecarChannel = byte(0x80)

	peerCatchupSleepIntervalMS = 100 // If peer is behind, sleep this amount

	// UnknownPeerID is the peer ID to use when running CheckTx when there is
	// no peer (e.g. RPC)
	UnknownPeerID uint16 = 0

	maxActiveIDs = math.MaxUint16
)

// Reactor handles mempool tx broadcasting amongst peers.
// It maintains a map from peer ID to counter, to prevent gossiping txs to the
// peers you received it from.
type Reactor struct {
	p2p.BaseReactor
	config  *cfg.MempoolConfig
	mempool *CListMempool
	ids     *mempoolIDs
	sidecar *CListPriorityTxSidecar
}

type mempoolIDs struct {
	mtx       tmsync.RWMutex
	peerMap   map[p2p.ID]uint16
	nextID    uint16              // assumes that a node will never have over 65536 active peers
	activeIDs map[uint16]struct{} // used to check if a given peerID key is used, the value doesn't matter
}

// Reserve searches for the next unused ID and assigns it to the
// peer.
func (ids *mempoolIDs) ReserveForPeer(peer p2p.Peer) {
	ids.mtx.Lock()
	defer ids.mtx.Unlock()

	curID := ids.nextPeerID()
	ids.peerMap[peer.ID()] = curID
	ids.activeIDs[curID] = struct{}{}
}

// nextPeerID returns the next unused peer ID to use.
// This assumes that ids's mutex is already locked.
func (ids *mempoolIDs) nextPeerID() uint16 {
	if len(ids.activeIDs) == maxActiveIDs {
		panic(fmt.Sprintf("node has maximum %d active IDs and wanted to get one more", maxActiveIDs))
	}

	_, idExists := ids.activeIDs[ids.nextID]
	for idExists {
		ids.nextID++
		_, idExists = ids.activeIDs[ids.nextID]
	}
	curID := ids.nextID
	ids.nextID++
	return curID
}

// Reclaim returns the ID reserved for the peer back to unused pool.
func (ids *mempoolIDs) Reclaim(peer p2p.Peer) {
	ids.mtx.Lock()
	defer ids.mtx.Unlock()

	removedID, ok := ids.peerMap[peer.ID()]
	if ok {
		delete(ids.activeIDs, removedID)
		delete(ids.peerMap, peer.ID())
	}
}

// GetForPeer returns an ID reserved for the peer.
func (ids *mempoolIDs) GetForPeer(peer p2p.Peer) uint16 {
	ids.mtx.RLock()
	defer ids.mtx.RUnlock()

	return ids.peerMap[peer.ID()]
}

func newMempoolIDs() *mempoolIDs {
	return &mempoolIDs{
		peerMap:   make(map[p2p.ID]uint16),
		activeIDs: map[uint16]struct{}{0: {}},
		nextID:    1, // reserve unknownPeerID(0) for mempoolReactor.BroadcastTx
	}
}

// NewReactor returns a new Reactor with the given config and mempool.
func NewReactor(config *cfg.MempoolConfig, mempool *CListMempool, sidecar *CListPriorityTxSidecar) *Reactor {
	memR := &Reactor{
		config:  config,
		mempool: mempool,
		sidecar: sidecar,
		ids:     newMempoolIDs(),
	}
	memR.BaseReactor = *p2p.NewBaseReactor("Mempool", memR)
	return memR
}

// InitPeer implements Reactor by creating a state for the peer.
func (memR *Reactor) InitPeer(peer p2p.Peer) p2p.Peer {
	memR.ids.ReserveForPeer(peer)
	return peer
}

// SetLogger sets the Logger on the reactor and the underlying mempool.
func (memR *Reactor) SetLogger(l log.Logger) {
	memR.Logger = l
	memR.mempool.SetLogger(l)
}

// OnStart implements p2p.BaseReactor.
func (memR *Reactor) OnStart() error {
	if !memR.config.Broadcast {
		memR.Logger.Info("Tx broadcasting is disabled")
	}
	return nil
}

// GetChannels implements Reactor by returning the list of channels for this
// reactor.
func (memR *Reactor) GetChannels() []*p2p.ChannelDescriptor {
	largestTx := make([]byte, memR.config.MaxTxBytes)
	batchMsg := protomem.Message{
		Sum: &protomem.Message_Txs{
			Txs: &protomem.Txs{Txs: [][]byte{largestTx}},
		},
	}

	return []*p2p.ChannelDescriptor{
		{
			ID:                  MempoolChannel,
			Priority:            5,
			RecvMessageCapacity: batchMsg.Size(),
		},
		{
			ID:                  SidecarChannel,
			Priority:            5,
			RecvMessageCapacity: batchMsg.Size(),
		},
	}
}

// AddPeer implements Reactor.
// It starts a broadcast routine ensuring all txs are forwarded to the given peer.
func (memR *Reactor) AddPeer(peer p2p.Peer) {
	if memR.config.Broadcast {
		go memR.broadcastMempoolTxRoutine(peer)
		if peer.IsSidecarPeer() {
			go memR.broadcastSidecarTxRoutine(peer)
		}
	}
}

// RemovePeer implements Reactor.
func (memR *Reactor) RemovePeer(peer p2p.Peer, reason interface{}) {
	memR.ids.Reclaim(peer)
	// broadcast routine checks if peer is gone and returns
}

// Receive implements Reactor.
// It adds any received transactions to the mempool.
func (memR *Reactor) Receive(chID byte, src p2p.Peer, msgBytes []byte) {
	msg, err := memR.decodeMsg(msgBytes)
	isSidecarPeer := src.IsSidecarPeer()
	if chID == MempoolChannel {
		if err != nil {
			memR.Logger.Error("Error decoding message", "src", src, "chId", chID, "err", err)
			memR.Switch.StopPeerForError(src, err)
			return
		}
		memR.Logger.Debug("Receive", "src", src, "chId", chID, "msg", msg)

		txInfo := TxInfo{SenderID: memR.ids.GetForPeer(src)}
		if src != nil {
			txInfo.SenderP2PID = src.ID()
		}
		for _, tx := range msg.Txs {
			err = memR.mempool.CheckTx(tx, nil, txInfo)
			if err == ErrTxInCache {
				memR.Logger.Debug("Tx already exists in cache", "tx", txID(tx))
			} else if err != nil {
				memR.Logger.Info(
					"Could not check tx",
					"tx", tx.String(),
					"err", err,
				)
			}
		}
		// broadcasting happens from go routines per peer
	} else if chID == SidecarChannel && isSidecarPeer {
		msg, err := memR.decodeBundleMsg(msgBytes)
		if err != nil {
			memR.Logger.Error(
				"Error decoding sidecar message",
				"src", src,
				"chId", chID,
				"err", err,
			)
			memR.Switch.StopPeerForError(src, err)
			return
		}
		txInfo := TxInfo{
			SenderID:      memR.ids.GetForPeer(src),
			DesiredHeight: msg.DesiredHeight,
			BundleID:      msg.BundleID,
			BundleOrder:   msg.BundleOrder,
			BundleSize:    msg.BundleSize,
		}
		if src != nil {
			txInfo.SenderP2PID = src.ID()
		}
		for _, tx := range msg.Txs {
			memR.Logger.Debug(
				"received sidecar tx",
				"tx", tx.Hash(),
				"desired height", msg.DesiredHeight,
				"bundle ID", msg.BundleID,
				"bundle order", msg.BundleOrder,
				"bundle size", msg.BundleSize,
			)

			err = memR.sidecar.AddTx(tx, txInfo)
			if err == ErrTxInCache {
				memR.Logger.Debug("sidecartx already exists in cache!", "tx", tx.Hash())
			} else if err != nil {
				memR.Logger.Info("could not add SidecarTx", "tx", tx.String(), "err", err)
			}
		}
	}
}

// PeerState describes the state of a peer.
type PeerState interface {
	GetHeight() int64
}

// Send new mempool txs to peer.
func (memR *Reactor) broadcastSidecarTxRoutine(peer p2p.Peer) {
	peerID := memR.ids.GetForPeer(peer)
	isSidecarPeer := peer.IsSidecarPeer()
	var next *clist.CElement

	for {
		// In case of both next.NextWaitChan() and peer.Quit() are variable at the same time
		if !memR.IsRunning() || !peer.IsRunning() {
			return
		}
		// This happens because the CElement we were looking at got garbage
		// collected (removed). That is, .NextWait() returned nil. Go ahead and
		// start from the beginning.
		if next == nil {
			select {
			case <-memR.sidecar.TxsWaitChan(): // Wait until a tx is available in sidecar
				// if a tx is available on sidecar, if fire is set too, then fire
				if next = memR.sidecar.TxsFront(); next == nil {
					continue
				}
			case <-peer.Quit():
				return
			case <-memR.Quit():
				return
			}
		}

		if scTx, okConv := next.Value.(*SidecarTx); okConv && isSidecarPeer {
			memR.Logger.Debug(
				"broadcasting a sidecarTx to peer",
				"peer", peerID,
				"tx", scTx.Tx.Hash(),
			)
			if _, ok := scTx.Senders.Load(peerID); !ok {
				msg := protomem.MEVMessage{
					Sum: &protomem.MEVMessage_Txs{
						Txs: &protomem.Txs{Txs: [][]byte{scTx.Tx}},
					},
					DesiredHeight: scTx.DesiredHeight,
					BundleID:      scTx.BundleID,
					BundleOrder:   scTx.BundleOrder,
					BundleSize:    scTx.BundleSize,
				}
				bz, err := msg.Marshal()
				if err != nil {
					memR.Logger.Info("not sending a sidecarTx for peer", peerID, "failed to marshal", msg)
					continue
				}
				success := peer.Send(SidecarChannel, bz)
				if !success {
					time.Sleep(peerCatchupSleepIntervalMS * time.Millisecond)
					continue
				}
			} else {
				memR.Logger.Info(
					"broadcasting sidecarTx to peer failed",
					"peer", peerID,
					"was considered sidecarPeer", isSidecarPeer,
					"was converted to sidecarTx", okConv,
					"tx", scTx.Tx.Hash(),
				)
			}
		}

		select {
		case <-next.NextWaitChan():
			// see the start of the for loop for nil check
			next = next.Next()
		case <-peer.Quit():
			return
		case <-memR.Quit():
			return
		}
	}
}

// Send new mempool txs to peer.
func (memR *Reactor) broadcastMempoolTxRoutine(peer p2p.Peer) {
	peerID := memR.ids.GetForPeer(peer)
	var next *clist.CElement

	for {
		// In case of both next.NextWaitChan() and peer.Quit() are variable at the same time
		if !memR.IsRunning() || !peer.IsRunning() {
			return
		}
		// This happens because the CElement we were looking at got garbage
		// collected (removed). That is, .NextWait() returned nil. Go ahead and
		// start from the beginning.
		if next == nil {
			select {
			case <-memR.mempool.TxsWaitChan(): // Wait until a tx is available
				if next = memR.mempool.TxsFront(); next == nil {
					continue
				}
			case <-peer.Quit():
				return
			case <-memR.Quit():
				return
			}
		}

		// Make sure the peer is up to date.
		peerState, ok := peer.Get(types.PeerStateKey).(PeerState)
		if !ok {
			// Peer does not have a state yet. We set it in the consensus reactor, but
			// when we add peer in Switch, the order we call reactors#AddPeer is
			// different every time due to us using a map. Sometimes other reactors
			// will be initialized before the consensus reactor. We should wait a few
			// milliseconds and retry.
			time.Sleep(peerCatchupSleepIntervalMS * time.Millisecond)
			continue
		}

		// Allow for a lag of 1 block.
		memTx := next.Value.(*mempoolTx)
		if peerState.GetHeight() < memTx.height-1 {
			time.Sleep(peerCatchupSleepIntervalMS * time.Millisecond)
			continue
		}

		// NOTE: Transaction batching was disabled due to
		// https://github.com/tendermint/tendermint/issues/5796

		if _, ok := memTx.senders.Load(peerID); !ok {
			msg := protomem.Message{
				Sum: &protomem.Message_Txs{
					Txs: &protomem.Txs{Txs: [][]byte{memTx.tx}},
				},
			}
			bz, err := msg.Marshal()
			if err != nil {
				panic(err)
			}
			success := peer.Send(MempoolChannel, bz)
			if !success {
				time.Sleep(peerCatchupSleepIntervalMS * time.Millisecond)
				continue
			}
		}

		select {
		case <-next.NextWaitChan():
			// see the start of the for loop for nil check
			next = next.Next()
		case <-peer.Quit():
			return
		case <-memR.Quit():
			return
		}
	}
}

//-----------------------------------------------------------------------------
// Messages

func (memR *Reactor) decodeMsg(bz []byte) (TxsMessage, error) {
	msg := protomem.Message{}
	err := msg.Unmarshal(bz)
	if err != nil {
		return TxsMessage{}, err
	}

	var message TxsMessage

	if i, ok := msg.Sum.(*protomem.Message_Txs); ok {
		txs := i.Txs.GetTxs()

		if len(txs) == 0 {
			return message, errors.New("empty TxsMessage")
		}

		decoded := make([]types.Tx, len(txs))
		for j, tx := range txs {
			decoded[j] = types.Tx(tx)
		}

		message = TxsMessage{
			Txs: decoded,
		}
		return message, nil
	}
	return message, fmt.Errorf("msg type: %T is not supported", msg)
}

func (memR *Reactor) decodeBundleMsg(bz []byte) (MEVTxsMessage, error) {
	msg := protomem.MEVMessage{}
	err := msg.Unmarshal(bz)
	if err != nil {
		return MEVTxsMessage{}, err
	}

	var message MEVTxsMessage

	if i, ok := msg.Sum.(*protomem.MEVMessage_Txs); ok {
		txs := i.Txs.GetTxs()

		if len(txs) == 0 {
			return message, errors.New("empty TxsMessage")
		}

		decoded := make([]types.Tx, len(txs))
		for j, tx := range txs {
			decoded[j] = types.Tx(tx)
		}

		message = MEVTxsMessage{
			Txs:           decoded,
			DesiredHeight: msg.GetDesiredHeight(),
			BundleID:      msg.GetBundleID(),
			BundleOrder:   msg.GetBundleOrder(),
			BundleSize:    msg.GetBundleSize(),
		}
		return message, nil
	}
	return message, fmt.Errorf("msg type: %T is not supported", msg)
}

//-------------------------------------

// TxsMessage is a Message containing transactions.
type MEVTxsMessage struct {
	Txs           []types.Tx
	DesiredHeight int64
	BundleID      int64
	BundleOrder   int64
	BundleSize    int64
}

// TxsMessage is a Message containing transactions.
type TxsMessage struct {
	Txs []types.Tx
}

// String returns a string representation of the TxsMessage.
func (m *TxsMessage) String() string {
	return fmt.Sprintf("[TxsMessage %v]", m.Txs)
}
