package mempool

import (
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/types"
)

// CombineSidecarAndMempoolTxs takes the txs reaped from the mempool and sidecar
// and combines them into one output Txs.
//
// param memplTxs - the ReapedTxs resulting from ReapMaxBytesMaxGas on the mempool
// param sidecarTxs - the ReapedTxs resulting from ReapMaxBytes on the sidecar
// param maxBytes - max allowed bytes of output txs
// param maxGas - max allows gas of output txs
// param logger - a logger
//
// returns a types.Txs of the sidecar txs followed by as many mempool txs as possible
func CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs types.ReapedTxs, maxBytes, maxGas int64, logger log.Logger) types.Txs {
	var (
		totalGas    int64
		runningSize int64
	)

	txs := make([]types.Tx, 0, (len(memplTxs.Txs) + len(sidecarTxs.Txs)))
	sidecarTxsMap := make(map[types.TxKey]struct{})

	for i, sidecarTx := range sidecarTxs.Txs {
		dataSize := types.ComputeProtoSizeForTxs([]types.Tx{sidecarTx})

		// Check total size requirement
		if maxBytes > -1 && runningSize+dataSize > maxBytes {
			logger.Info("[mev-tendermint]: Sidecar txs exceeded block maxBytes, falling back to mempool txs")
			return memplTxs.Txs
		}
		runningSize += dataSize

		newTotalGas := totalGas + sidecarTxs.GasWanteds[i]
		if maxGas > -1 && newTotalGas > maxGas {
			logger.Info("[mev-tendermint]: Sidecar txs exceeded block maxGas, falling back to mempool txs")
			return memplTxs.Txs
		}
		totalGas = newTotalGas
		txs = append(txs, sidecarTx)
		sidecarTxsMap[sidecarTx.Key()] = struct{}{}
		logger.Info(
			"[mev-tendermint]: reaped sidecar",
			"mev transaction", getLastNumBytesFromTx(sidecarTx, 20),
			"gasWanted", sidecarTxs.GasWanteds[i],
		)
	}

	for i, memplTx := range memplTxs.Txs {
		if _, ok := sidecarTxsMap[memplTx.Key()]; ok {
			// SKIP THIS TRANSACTION, ALREADY SEEN IN SIDECAR
			logger.Info("[mev-tendermint]: skipped mempool tx, already found in sidecar", getLastNumBytesFromTx(memplTx, 20))
			continue
		}

		dataSize := types.ComputeProtoSizeForTxs([]types.Tx{memplTx})

		// Check total size requirement
		if maxBytes > -1 && runningSize+dataSize > maxBytes {
			return txs
		}
		runningSize += dataSize

		// Check total gas requirement.
		// If maxGas is negative, skip this check.
		// Since newTotalGas < masGas, which
		// must be non-negative, it follows that this won't overflow.
		newTotalGas := totalGas + memplTxs.GasWanteds[i]
		if maxGas > -1 && newTotalGas > maxGas {
			return txs
		}
		totalGas = newTotalGas
		txs = append(txs, memplTx)
	}
	return txs
}

func getLastNumBytesFromTx(tx types.Tx, numBytes int) string {
	if len(tx) == 0 {
		return ""
	} else if len(tx) < 20 {
		return tx.String()
	} else {
		return tx.String()[len(tx.String())-20:]
	}
}
