package core

import (
	"strings"
	"time"

	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/p2p"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	rpctypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	"github.com/tendermint/tendermint/types"
)

// Status returns Tendermint status including node info, pubkey, latest block
// hash, app hash, block height and time.
// More: https://docs.tendermint.com/v0.34/rpc/#/Info/status
func Status(ctx *rpctypes.Context) (*ctypes.ResultStatus, error) {
	var (
		earliestBlockHeight   int64
		earliestBlockHash     tmbytes.HexBytes
		earliestAppHash       tmbytes.HexBytes
		earliestBlockTimeNano int64
	)

	if earliestBlockMeta := env.BlockStore.LoadBaseMeta(); earliestBlockMeta != nil {
		earliestBlockHeight = earliestBlockMeta.Header.Height
		earliestAppHash = earliestBlockMeta.Header.AppHash
		earliestBlockHash = earliestBlockMeta.BlockID.Hash
		earliestBlockTimeNano = earliestBlockMeta.Header.Time.UnixNano()
	}

	var (
		latestBlockHash     tmbytes.HexBytes
		latestAppHash       tmbytes.HexBytes
		latestBlockTimeNano int64

		latestHeight = env.BlockStore.Height()
	)

	if latestHeight != 0 {
		if latestBlockMeta := env.BlockStore.LoadBlockMeta(latestHeight); latestBlockMeta != nil {
			latestBlockHash = latestBlockMeta.BlockID.Hash
			latestAppHash = latestBlockMeta.Header.AppHash
			latestBlockTimeNano = latestBlockMeta.Header.Time.UnixNano()
		}
	}

	// Return the very last voting power, not the voting power of this validator
	// during the last block.
	var votingPower int64
	if val := validatorAtHeight(latestUncommittedHeight()); val != nil {
		votingPower = val.VotingPower
	}

	var (
		isPeeredWithSentinel     bool
		lastReceivedBundleHeight int64
	)

	// Temporarily support both SentinelPeerString and RelayerPeerString
	sentinel := env.SidecarConfig.SentinelPeerString
	if len(sentinel) == 0 {
		sentinel = env.SidecarConfig.RelayerPeerString
	}
	if sentinel != "" {
		isPeeredWithSentinel = env.P2PPeers.Peers().Has(p2p.ID(strings.Split(sentinel, "@")[0]))
	}

	if env.Sidecar != nil {
		lastReceivedBundleHeight = env.Sidecar.GetLastBundleHeight()
	}

	result := &ctypes.ResultStatus{
		NodeInfo: env.P2PTransport.NodeInfo().(p2p.DefaultNodeInfo),
		SyncInfo: ctypes.SyncInfo{
			LatestBlockHash:     latestBlockHash,
			LatestAppHash:       latestAppHash,
			LatestBlockHeight:   latestHeight,
			LatestBlockTime:     time.Unix(0, latestBlockTimeNano),
			EarliestBlockHash:   earliestBlockHash,
			EarliestAppHash:     earliestAppHash,
			EarliestBlockHeight: earliestBlockHeight,
			EarliestBlockTime:   time.Unix(0, earliestBlockTimeNano),
			CatchingUp:          env.ConsensusReactor.WaitSync(),
		},
		ValidatorInfo: ctypes.ValidatorInfo{
			Address:     env.PubKey.Address(),
			PubKey:      env.PubKey,
			VotingPower: votingPower,
		},
		MevInfo: ctypes.MevInfo{
			IsPeeredWithSentinel:     isPeeredWithSentinel,
			LastReceivedBundleHeight: lastReceivedBundleHeight,
		},
	}

	return result, nil
}

func validatorAtHeight(h int64) *types.Validator {
	vals, err := env.StateStore.LoadValidators(h)
	if err != nil {
		return nil
	}
	privValAddress := env.PubKey.Address()
	_, val := vals.GetByAddress(privValAddress)
	return val
}
