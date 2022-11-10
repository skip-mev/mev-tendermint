package mempool

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tendermint/tendermint/types"
)

func TestCombineSidecarAndMempoolTxs_SkipsSidecarTxsInMempoolReap(t *testing.T) {
	tx := make([]byte, 20)
	_, err := rand.Read(tx)
	if err != nil {
		t.Error(err)
	}
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
