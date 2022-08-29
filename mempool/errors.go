package mempool

import (
	"errors"
	"fmt"
)

var (
	// ErrTxInCache is returned to the client if we saw tx earlier
	ErrTxInCache = errors.New("tx already exists in cache")
)

// ErrWrongHeight means the tx is asking to be in a height that doesn't match the current auction
type ErrWrongHeight struct {
	desiredHeight        int
	currentAuctionHeight int
}

func (e ErrWrongHeight) Error() string {
	return fmt.Sprintf("Tx submitted for wrong height, asked for %d, but current auction height is %d", e.desiredHeight, e.currentAuctionHeight)
}

// ErrBundleFull means the tx is trying to enter a bundle that has already reached its limit
type ErrBundleFull struct {
	bundleId     int64
	bundleHeight int64
}

func (e ErrBundleFull) Error() string {
	return fmt.Sprintf("Tx submitted but bundle is full, for bundleId %d with bundle size %d", e.bundleId, e.bundleHeight)
}

// ErrTxMalformedForBundle is a general malformed error for specific cases
type ErrTxMalformedForBundle struct {
	bundleId     int64
	bundleSize   int64
	bundleHeight int64
	bundleOrder  int64
}

func (e ErrTxMalformedForBundle) Error() string {
	return fmt.Sprintf("Tx submitted but malformed with respect to bundling, for bundleId %d, at height %d, with bundleSize %d, and bundleOrder %d", e.bundleId, e.bundleHeight, e.bundleSize, e.bundleOrder)
}

// ErrTxTooLarge means the tx is too big to be sent in a message to other peers
type ErrTxTooLarge struct {
	max    int
	actual int
}

func (e ErrTxTooLarge) Error() string {
	return fmt.Sprintf("Tx too large. Max size is %d, but got %d", e.max, e.actual)
}

// ErrMempoolIsFull means Tendermint & an application can't handle that much load
type ErrMempoolIsFull struct {
	numTxs int
	maxTxs int

	txsBytes    int64
	maxTxsBytes int64
}

func (e ErrMempoolIsFull) Error() string {
	return fmt.Sprintf(
		"mempool is full: number of txs %d (max: %d), total txs bytes %d (max: %d)",
		e.numTxs, e.maxTxs,
		e.txsBytes, e.maxTxsBytes)
}

// ErrPreCheck is returned when tx is too big
type ErrPreCheck struct {
	Reason error
}

func (e ErrPreCheck) Error() string {
	return e.Reason.Error()
}

// IsPreCheckError returns true if err is due to pre check failure.
func IsPreCheckError(err error) bool {
	_, ok := err.(ErrPreCheck)
	return ok
}
