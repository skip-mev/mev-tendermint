package p2p

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/tendermint/tendermint/libs/log"
)

func RegisterWithSentinel(logger log.Logger, APIKey, validatorAddrHex, peerID, sentinel string) {
	logger.Info("[p2p.sentinel]: Registering with sentinel", APIKey, validatorAddrHex, peerID, sentinel)

	jsonData, err := makePostRequestData(APIKey, validatorAddrHex, peerID)
	if err != nil {
		logger.Info("[p2p.sentinel]: Err marshalling json data:", err)
		return
	}

	go postRequestRoutine(logger, sentinel, jsonData)
}

func postRequestRoutine(logger log.Logger, sentinel string, jsonData []byte) {
	resp, err := http.Post(sentinel, "application/json", bytes.NewBuffer(jsonData)) //nolint:gosec
	if err != nil {
		logger.Info("[p2p.sentinel]: Err making post request to sentinel:", err)
	} else {
		logger.Info("[p2p.sentinel]: Successfully registered with sentinel", resp)
		defer resp.Body.Close()
	}
}

func makePostRequestData(APIKey, validatorAddrHex, peerID string) ([]byte, error) {
	params := [3]string{APIKey, validatorAddrHex, peerID}
	data := map[string]interface{}{
		"method": "register_peer",
		"params": params,
		"id":     1,
	}

	return json.Marshal(data)
}
