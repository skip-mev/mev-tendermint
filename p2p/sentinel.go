package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tendermint/tendermint/libs/log"
)

func RegisterWithSentinel(logger log.Logger, APIKey, validatorAddrHex, peerID, sentinel string) {
	logger.Info("[p2p.sentinel]: Registering with sentinel", APIKey, validatorAddrHex, peerID, sentinel)

	params := [3]string{APIKey, validatorAddrHex, peerID}
	data := map[string]interface{}{
		"method": "register_peer",
		"params": params,
		"id":     1,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("[p2p.sentinel]: Err marshalling json data:", err)
		return
	}

	go func() {
		resp, err := http.Post(sentinel, "application/json", bytes.NewBuffer(jsonData)) //nolint:gosec
		if err != nil {
			logger.Info("[p2p.sentinel]: Err making post request to sentinel:", err.Error())
		} else {
			logger.Info("[p2p.sentinel]: SUCCESSFULLY REGISTERED WITH SENTINEL!", resp)
			defer resp.Body.Close()
		}
	}()
}
