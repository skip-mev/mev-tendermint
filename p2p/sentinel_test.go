package p2p

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tendermint/tendermint/libs/log"
)

func TestPostRequestRoutine(t *testing.T) {
	// Make a mock http server to receive the register request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path != "/" {
			t.Errorf("Expected to request '/', got: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
		}

		bodyBytes := make([]byte, 100)
		n, _ := r.Body.Read(bodyBytes)

		var body map[string]interface{}
		err := json.Unmarshal(bodyBytes[:n], &body)
		if err != nil {
			t.Errorf("Erred while unmarshalling body: %s", err)
		}

		if id := body["id"].(float64); id != float64(1) {
			t.Errorf("Expected post body to have id=%f, got %f", float64(1), id)
		}
		if method := body["method"].(string); method != "register_peer" {
			t.Errorf("Expected post body to have method=%s, got %s", "register_peer", method)
		}
		params := body["params"].([]interface{})
		if apikey := params[0].(string); apikey != "test-api-key" {
			t.Errorf("Expected post body params to have apikey %s, got %s", "test-api-key", apikey)
		}
		if valhex := params[1].(string); valhex != "ABCD1234" {
			t.Errorf("Expected post body params to have valhex %s, got %s", "ABCD1234", valhex)
		}
		if peerID := params[2].(string); peerID != "a1b2c3d4" {
			t.Errorf("Expected post body params to have peerID %s, got %s", "a1b2c3d4", peerID)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	jsonData, _ := makePostRequestData("test-api-key", "ABCD1234", "a1b2c3d4")
	postRequestRoutine(log.NewNopLogger(), server.URL, jsonData)
}

func TestPostRequestRoutine_DoesNotPanicOn500(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Unexpected panic from postRequestRoutine")
		}
	}()

	// Make a mock http server to receive the register request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	jsonData, _ := makePostRequestData("test-api-key", "ABCD1234", "a1b2c3d4")
	postRequestRoutine(log.NewNopLogger(), server.URL, jsonData)
}
