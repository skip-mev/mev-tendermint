package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tendermint/tendermint/libs/log"
)

func RegisterWithSentinel(logger log.Logger, APIKey, peerID, sentinel string) {
	logger.Info(
		"[p2p.sentinel]: Registering with sentinel (first try)",
		"API Key", APIKey,
		"peerID", peerID,
		"sentinel string", sentinel,
	)

	jsonData, err := makePostRequestData(peerID, APIKey)
	if err != nil {
		logger.Info("[p2p.sentinel]: Err marshalling json data:", err)
		return
	}

	go postRequestRoutine(logger, sentinel, jsonData)
}

func postRequestRoutine(logger log.Logger, sentinel string, jsonData []byte) {
	resp, err := http.Post(sentinel, "application/json", bytes.NewBuffer(jsonData)) //nolint:gosec
	logger.Info("response was", "resp", resp)
	logger.Info("error was", "err", err)
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
				if resp != nil {
					logger.Info("[p2p.sentinel]: SUCCESSFULLY REGISTERED WITH SENTINEL!",
						"responseStatusCode", resp.StatusCode,
						"responseStatus", resp.Status,
					)
					if resp.Body != nil {
						defer resp.Body.Close()
					}
				}
				return
			}
			time.Sleep(30 * time.Second)
			tries++
		}
	} else {
		if resp != nil && resp.Body != nil {
			logger.Info("[p2p.sentinel]: SUCCESSFULLY REGISTERED WITH SENTINEL", resp)
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Println("[p2p.sentinel]: Error unmarshalling body", err)
			}
			bodyString := string(bodyBytes)
			logger.Info("[p2p.sentinel]:", bodyString)
		}
	}
}

func makePostRequestData(peerID, APIKey string) ([]byte, error) {
	params := [2]string{peerID, APIKey}
	data := map[string]interface{}{
		"method": "register_node_api",
		"params": params,
		"id":     1,
	}

	return json.Marshal(data)
}
