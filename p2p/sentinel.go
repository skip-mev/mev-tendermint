package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func RegisterWithSentinel(APIKey, validatorAddr, peerID, sentinel string) {
	fmt.Println("[p2p.sentinel]: Registering with sentinel", APIKey, validatorAddr, peerID, sentinel)

	params := [3]string{APIKey, validatorAddr, peerID}
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
		_, err := http.Post(sentinel, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			fmt.Println("[p2p.sentinel]: Err making post request to sentinel:", err)
		}
	}()
}
