package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func RegisterWithSentinel(APIKey, validatorAddr, peer, sentinel string) {
	fmt.Println("[p2p.sentinel]: Registering with sentinel", APIKey, validatorAddr, peer)

	params := [3]string{APIKey, validatorAddr, peer}
	data := map[string]interface{} {
		"method": "register_peer",
		"params": params,
		"id": 1,
	}

	json_data, err := json.Marshal(data)
	if err != nil {
		fmt.Println("[p2p.sentinel]: Err marshalling json data:", err)
		return
	}

	_, err = http.Post(sentinel, "application/json", bytes.NewBuffer(json_data))
	if err != nil {
		fmt.Println("[p2p.sentinel]: Err making post request to sentinel:", err)
	}
}
