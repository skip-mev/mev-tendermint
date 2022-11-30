package v0

import (
	"errors"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"

	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/clist"
	"github.com/tendermint/tendermint/libs/log"
	tmsync "github.com/tendermint/tendermint/libs/sync"
	"github.com/tendermint/tendermint/mempool"
	"github.com/tendermint/tendermint/p2p"
	protomem "github.com/tendermint/tendermint/proto/tendermint/mempool"
	"github.com/tendermint/tendermint/types"
)

// Reactor handles mempool tx broadcasting amongst peers.
// It maintains a map from peer ID to counter, to prevent gossiping txs to the
// peers you received it from.
type Reactor struct {
	p2p.BaseReactor
	config  *cfg.MempoolConfig
	mempool *CListMempool
	sidecar *mempool.CListPriorityTxSidecar
	ids     *mempoolIDs
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
	if len(ids.activeIDs) == mempool.MaxActiveIDs {
		panic(fmt.Sprintf("node has maximum %d active IDs and wanted to get one more", mempool.MaxActiveIDs))
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
func NewReactor(config *cfg.MempoolConfig, mempool *CListMempool, sidecar *mempool.CListPriorityTxSidecar) *Reactor {
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
			ID:                  mempool.MempoolChannel,
			Priority:            5,
			RecvMessageCapacity: batchMsg.Size(),
			MessageType:         &protomem.Message{},
		},
		{
			ID:                  mempool.SidecarLegacyChannel,
			Priority:            5,
			RecvMessageCapacity: batchMsg.Size(),
			MessageType:         &protomem.MEVMessage{},
		},
		{
			ID:                  mempool.SidecarChannel,
			Priority:            5,
			RecvMessageCapacity: batchMsg.Size(),
			MessageType:         &protomem.Message{},
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
func (memR *Reactor) ReceiveEnvelope(e p2p.Envelope) {
	memR.Logger.Debug("Receive", "src", e.Src, "chId", e.ChannelID, "msg", e.Message)
	isSidecarPeer := e.Src.IsSidecarPeer()
	isLegacySidecarChannel := e.ChannelID == mempool.SidecarLegacyChannel
	isSidecarChannel := e.ChannelID == mempool.SidecarChannel
	switch msg := e.Message.(type) {
	case *protomem.Txs:
		protoTxs := msg.GetTxs()
		if len(protoTxs) == 0 {
			memR.Logger.Error("received empty txs from peer", "src", e.Src)
			return
		}
		txInfo := mempool.TxInfo{SenderID: memR.ids.GetForPeer(e.Src)}
		if e.Src != nil {
			txInfo.SenderP2PID = e.Src.ID()
		}

		var err error
		for _, tx := range protoTxs {
			ntx := types.Tx(tx)
			err = memR.mempool.CheckTx(ntx, nil, txInfo)
			if errors.Is(err, mempool.ErrTxInCache) {
				memR.Logger.Debug("Tx already exists in cache", "tx", ntx.String())
			} else if err != nil {
				memR.Logger.Info("Could not check tx", "tx", ntx.String(), "err", err)
			}
		}
	case *protomem.MEVTxs:
		protoTxs := msg.GetTxs()
		if len(protoTxs) == 0 {
			memR.Logger.Error("received empty txs from peer", "src", e.Src)
			return
		}
		if !isSidecarChannel {
			memR.Logger.Error("received mev txs over incorrect channel", "src", e.Src, "chId", e.ChannelID)
			return
		}
		if !isSidecarPeer {
			memR.Logger.Error("received mev txs from non-sidecar peer", "src", e.Src)
			return
		}
		summary := MEVTxSummary{
			Txs:           protoTxs,
			DesiredHeight: msg.DesiredHeight,
			BundleID:      msg.BundleId,
			BundleOrder:   msg.BundleOrder,
			BundleSize:    msg.BundleSize,
			GasWanted:     msg.GasWanted,
		}
		memR.InsertMEVTxInSidecar(summary, e.Src)
	case *protomem.MEVMessage:
		summary, err := memR.decodeLegacyMEVMessage(msg)
		if err != nil {
			memR.Logger.Error("failed to decode legacy mev message", "err", err)
			return
		}
		if !isLegacySidecarChannel {
			memR.Logger.Error("received legacy mev message over incorrect channel", "src", e.Src, "chId", e.ChannelID)
			return
		}
		if !isSidecarPeer {
			memR.Logger.Error("received legacy mev message from non-sidecar peer", "src", e.Src)
			return
		}
		memR.InsertMEVTxInSidecar(summary, e.Src)
	default:
		memR.Logger.Error("unknown message type", "src", e.Src, "chId", e.ChannelID, "msg", e.Message)
		memR.Switch.StopPeerForError(e.Src, fmt.Errorf("mempool cannot handle message of type: %T", e.Message))
		return
	}
}

func (memR *Reactor) Receive(chID byte, peer p2p.Peer, msgBytes []byte) {
	if chID == mempool.MempoolChannel || chID == mempool.SidecarChannel {
		msg := &protomem.Message{}

		err := proto.Unmarshal(msgBytes, msg)
		if err != nil {
			memR.Logger.Error("failed to decode message", "err", err)
			return
		}
		uw, err := msg.Unwrap()
		if err != nil {
			memR.Logger.Error("failed to unwrap message", "err", err)
			return
		}
		memR.ReceiveEnvelope(p2p.Envelope{
			ChannelID: chID,
			Src:       peer,
			Message:   uw,
		})
	} else if chID == mempool.SidecarLegacyChannel {
		msg := &protomem.MEVMessage{}
		err := proto.Unmarshal(msgBytes, msg)
		if err != nil {
			memR.Logger.Error("failed to decode message", "err", err)
			return
		}
		memR.ReceiveEnvelope(p2p.Envelope{
			ChannelID: chID,
			Src:       peer,
			Message:   msg,
		})
	}
}

// Ingress a transaction to the sidecar along with txinfo object
func (memR *Reactor) InsertMEVTxInSidecar(summary MEVTxSummary, peer p2p.Peer) {
	txInfo := mempool.TxInfo{
		SenderID:      memR.ids.GetForPeer(peer),
		DesiredHeight: summary.DesiredHeight,
		BundleID:      summary.BundleID,
		BundleOrder:   summary.BundleOrder,
		BundleSize:    summary.BundleSize,
		GasWanted:     summary.GasWanted,
	}
	if peer != nil {
		txInfo.SenderP2PID = peer.ID()
	}
	for _, tx := range summary.Txs {
		ntx := types.Tx(tx)
		err := memR.sidecar.AddTx(ntx, txInfo)
		if err == mempool.ErrTxInCache {
			memR.Logger.Debug("sidecartx already exists in cache!", "tx", ntx.Hash())
		} else if err != nil {
			memR.Logger.Info("could not add SidecarTx", "tx", ntx.String(), "err", err)
		}
	}
}

// PeerState describes the state of a peer.
type PeerState interface {
	GetHeight() int64
}

// Send new sidecar txs to peer.
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
		// Make sure the peer is up to date.
		peerState, ok := peer.Get(types.PeerStateKey).(PeerState)
		if !ok {
			// Peer does not have a state yet. We set it in the consensus reactor, but
			// when we add peer in Switch, the order we call reactors#AddPeer is
			// different every time due to us using a map. Sometimes other reactors
			// will be initialized before the consensus reactor. We should wait a few
			// milliseconds and retry.
			time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
			continue
		}
		if scTx, okConv := next.Value.(*mempool.SidecarTx); okConv && isSidecarPeer {
			memR.Logger.Debug(
				"broadcasting a sidecarTx to peer",
				"peer", peerID,
				"tx", scTx.Tx.Hash(),
			)
			if peerState.GetHeight() < scTx.DesiredHeight-1 {
				time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
				continue
			}
			if _, ok := scTx.Senders.Load(peerID); !ok {
				// try sending over using envelop protocol over new channel first
				msg := &protomem.MEVTxs{
					Txs:           [][]byte{scTx.Tx},
					DesiredHeight: scTx.DesiredHeight,
					BundleId:      scTx.BundleID,
					BundleOrder:   scTx.BundleOrder,
					BundleSize:    scTx.BundleSize,
					GasWanted:     scTx.GasWanted,
				}
				success := p2p.SendEnvelopeShim(peer, p2p.Envelope{ //nolint: staticcheck
					ChannelID: mempool.SidecarChannel,
					Message:   msg,
				}, memR.Logger)
				if !success {
					// if sending over envelop protocol fails, try sending over legacy protocol
					msg := &protomem.MEVMessage{
						Sum: &protomem.MEVMessage_Txs{
							Txs: &protomem.Txs{Txs: [][]byte{scTx.Tx}},
						},
						DesiredHeight: scTx.DesiredHeight,
						BundleId:      scTx.BundleID,
						BundleOrder:   scTx.BundleOrder,
						BundleSize:    scTx.BundleSize,
						GasWanted:     scTx.GasWanted,
					}

					success := p2p.SendEnvelopeShim(peer, p2p.Envelope{
						ChannelID: mempool.SidecarLegacyChannel,
						Message:   msg,
					}, memR.Logger)
					if !success {
						time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
						continue
					}
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
			time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
			continue
		}

		// Allow for a lag of 1 block.
		memTx := next.Value.(*mempoolTx)
		if peerState.GetHeight() < memTx.height-1 {
			time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
			continue
		}

		// NOTE: Transaction batching was disabled due to
		// https://github.com/tendermint/tendermint/issues/5796

		if _, ok := memTx.senders.Load(peerID); !ok {
			success := p2p.SendEnvelopeShim(peer, p2p.Envelope{ //nolint: staticcheck
				ChannelID: mempool.MempoolChannel,
				Message:   &protomem.Txs{Txs: [][]byte{memTx.tx}},
			}, memR.Logger)
			if !success {
				time.Sleep(mempool.PeerCatchupSleepIntervalMS * time.Millisecond)
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

func (memR *Reactor) decodeLegacyMEVMessage(lm *protomem.MEVMessage) (MEVTxSummary, error) {
	var summary MEVTxSummary

	if i, ok := lm.Sum.(*protomem.MEVMessage_Txs); ok {
		txs := i.Txs.GetTxs()

		if len(txs) == 0 {
			return summary, errors.New("empty TxsMessage")
		}

		decoded := make([]types.Tx, len(txs))
		for j, tx := range txs {
			decoded[j] = types.Tx(tx)
		}

		summary = MEVTxSummary{
			Txs:           txs,
			DesiredHeight: lm.GetDesiredHeight(),
			BundleID:      lm.GetBundleId(),
			BundleOrder:   lm.GetBundleOrder(),
			BundleSize:    lm.GetBundleSize(),
			GasWanted:     lm.GetGasWanted(),
		}
		return summary, nil
	}
	return summary, fmt.Errorf("msg type: %T is not supported", lm)
}

// MEVTxsSummary is a data structure used to abstract MEV Txs
// mempool insertion logic from the protobuf message they are received in
type MEVTxSummary struct {
	Txs           [][]byte
	DesiredHeight int64
	BundleID      int64
	BundleOrder   int64
	BundleSize    int64
	GasWanted     int64
}

//-----------------------------------------------------------------------------
// Messages

// TxsMessage is a Message containing transactions.
type TxsMessage struct {
	Txs []types.Tx
}

// String returns a string representation of the TxsMessage.
func (m *TxsMessage) String() string {
	return fmt.Sprintf("[TxsMessage %v]", m.Txs)
}
