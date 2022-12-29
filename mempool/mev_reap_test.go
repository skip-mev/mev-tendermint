package mempool

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/types"
)

func TestCombineSidecarAndMempoolTxs_SkipsSidecarTxsInMempoolReap(t *testing.T) {
	tx := makeTxWithNumBytes(t, 20)
	// Put one tx in the mempool
	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{tx},
		GasWanteds: []int64{10},
	}
	// Put the same tx in sidecarTxs
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{tx},
		GasWanteds: []int64{10},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 50, 500)

	// Assert that it only got reaped once
	expectedTxs := types.Txs([]types.Tx{tx})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_SidecarHassMaxBytes(t *testing.T) {
	mplTx := makeTxWithNumBytes(t, 18)
	scTx1 := makeTxWithNumBytes(t, 18)
	scTx2 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx},
		GasWanteds: []int64{10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{scTx1, scTx2},
		GasWanteds: []int64{10, 10},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 40, 50)

	expectedTxs := types.Txs([]types.Tx{scTx1, scTx2})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_SidecarHasMaxGas(t *testing.T) {
	mplTx := makeTxWithNumBytes(t, 18)
	scTx1 := makeTxWithNumBytes(t, 18)
	scTx2 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx},
		GasWanteds: []int64{10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{scTx1, scTx2},
		GasWanteds: []int64{10, 10},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 60, 20)

	expectedTxs := types.Txs([]types.Tx{scTx1, scTx2})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_MempoolHasMaxBytes(t *testing.T) {
	mplTx1 := makeTxWithNumBytes(t, 18)
	mplTx2 := makeTxWithNumBytes(t, 18)
	mplTx3 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx1, mplTx2, mplTx3},
		GasWanteds: []int64{10, 10, 10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{},
		GasWanteds: []int64{},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 40, 40)

	expectedTxs := types.Txs([]types.Tx{mplTx1, mplTx2})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_MempoolHasMaxGas(t *testing.T) {
	mplTx1 := makeTxWithNumBytes(t, 18)
	mplTx2 := makeTxWithNumBytes(t, 18)
	mplTx3 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx1, mplTx2, mplTx3},
		GasWanteds: []int64{10, 10, 10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{},
		GasWanteds: []int64{},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 100, 20)

	expectedTxs := types.Txs([]types.Tx{mplTx1, mplTx2})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_CombinedHasMaxBytes(t *testing.T) {
	mplTx1 := makeTxWithNumBytes(t, 18)
	mplTx2 := makeTxWithNumBytes(t, 18)
	scTx1 := makeTxWithNumBytes(t, 18)
	scTx2 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx1, mplTx2},
		GasWanteds: []int64{10, 10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{scTx1, scTx2},
		GasWanteds: []int64{10, 10},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 60, 60)

	expectedTxs := types.Txs([]types.Tx{scTx1, scTx2, mplTx1})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func TestCombineSidecarAndMempoolTxs_CombinedHasMaxGas(t *testing.T) {
	mplTx1 := makeTxWithNumBytes(t, 18)
	mplTx2 := makeTxWithNumBytes(t, 18)
	scTx1 := makeTxWithNumBytes(t, 18)
	scTx2 := makeTxWithNumBytes(t, 18)

	memplTxs := types.ReapedTxs{
		Txs:        []types.Tx{mplTx1, mplTx2},
		GasWanteds: []int64{10, 10},
	}
	sidecarTxs := types.ReapedTxs{
		Txs:        []types.Tx{scTx1, scTx2},
		GasWanteds: []int64{10, 10},
	}

	// Combine
	reapedTxs := CombineSidecarAndMempoolTxs(memplTxs, sidecarTxs, 100, 30)

	expectedTxs := types.Txs([]types.Tx{scTx1, scTx2, mplTx1})
	assert.Equal(t, expectedTxs, reapedTxs, "Got %s, expected %s", reapedTxs, expectedTxs)
}

func makeTxWithNumBytes(t *testing.T, numBytes int) types.Tx {
	tx := make([]byte, numBytes)
	_, err := rand.Read(tx)
	if err != nil {
		t.Error(err)
	}
	return tx
}
