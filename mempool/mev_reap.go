package mempool

import (
	"fmt"

	"github.com/tendermint/tendermint/types"
)

func CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs types.ReapedTxs, maxBytes, maxGas int64) types.Txs {
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
			return txs
		}
		runningSize += dataSize

		newTotalGas := totalGas + sidecarTxs.GasWanteds[i]
		if maxGas > -1 && newTotalGas > maxGas {
			return txs
		}
		totalGas = newTotalGas
		txs = append(txs, sidecarTx)
		sidecarTxsMap[sidecarTx.Key()] = struct{}{}
		fmt.Printf("[mev-tendermint]: reaped sidecar mev transaction %s with gasWanted %d\n",
			getLastNumBytesFromTx(sidecarTx, 20), sidecarTxs.GasWanteds[i])
	}

	for i, memplTx := range memplTxs.Txs {
		if _, ok := sidecarTxsMap[memplTx.Key()]; ok {
			// SKIP THIS TRANSACTION, ALREADY SEEN IN SIDECAR
			fmt.Println("[mev-tendermint]: skipped mempool tx, already found in sidecar", getLastNumBytesFromTx(memplTx, 20))
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
