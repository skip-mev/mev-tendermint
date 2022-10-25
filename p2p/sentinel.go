package p2p

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tendermint/tendermint/libs/log"
)

func RegisterWithSentinel(logger log.Logger, APIKey, validatorAddrHex, peerID, sentinel string) {
	logger.Info(
		"[p2p.sentinel]: Registering with sentinel (first try)",
		"API Key", APIKey,
		"validatorAddrHex", validatorAddrHex,
		"peerID", peerID,
		"sentinel string", sentinel,
	)

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
		tries := 1
		for {
			logger.Info("[p2p.sentinel]: Attempt to reregister via Sentinel API",
				"try #", tries,
			)
			resp, err := http.Post(sentinel, "application/json", bytes.NewBuffer(jsonData)) //nolint:gosec
			if err != nil || (resp == nil) || (resp != nil && resp.StatusCode != http.StatusOK) {
				if resp != nil {
					logger.Info("[p2p.sentinel]: reregister with Sentinel API failed",
						"status code", resp.StatusCode,
					)
				}
				if err != nil {
					logger.Info("[p2p.sentinel]: error was",
						"err:", err,
					)
				}
			} else {
				logger.Info("[p2p.sentinel]: SUCCESSFULLY REGISTERED WITH SENTINEL!",
					"responseStatusCode", resp.StatusCode,
					"responseStatus", resp.Status,
				)
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
				return
			}
			time.Sleep(30 * time.Second)
			tries++
		}
	} else {
		logger.Info("[p2p.sentinel]: SUCCESSFULLY REGISTERED WITH SENTINEL", resp)
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}
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
