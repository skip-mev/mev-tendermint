package p2p

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/version"
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
		if method := body["method"].(string); method != "register_node" {
			t.Errorf("Expected post body to have method=%s, got %s", "register_node", method)
		}
		params := body["params"].([]interface{})
		if peerID := params[0].(string); peerID != "a1b2c3d4" {
			t.Errorf("Expected post body params to have peerID %s, got %s", "a1b2c3d4", peerID)
		}
		if apikey := params[1].(string); apikey != "test-api-key" {
			t.Errorf("Expected post body params to have apikey %s, got %s", "test-api-key", apikey)
		}
		if versionParam := params[2].(string); versionParam != version.MevTMVersion {
			t.Errorf("Expected post body params to have version %s, got %s", version.MevTMVersion, versionParam)
		}
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(`{"jsonrpc": "2.0", "id": 1, "result": "success"}`))
		if err != nil {
			t.Errorf("Error writing response: %s", err)
		}
	}))
	defer server.Close()

	jsonData, _ := makePostRequestData("a1b2c3d4", "test-api-key", version.MevTMVersion)
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

	jsonData, _ := makePostRequestData("a1b2c3d4", "test-api-key", version.MevTMVersion)
	err := attemptRegisterOnce(log.NewNopLogger(), server.URL, jsonData)
	if err == nil {
		t.Errorf("Expected error on fake server")
	}
}

func TestErrorOnJSONRPCError(t *testing.T) {
	// Make a mock http server to receive the register request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"jsonrpc": "2.0", "id": 1, "error": "some error"}`))
		if err != nil {
			t.Errorf("Error writing response: %s", err)
		}
	}))
	defer server.Close()

	jsonData, _ := makePostRequestData("a1b2c3d4", "test-api-key", version.MevTMVersion)
	err := attemptRegisterOnce(log.NewNopLogger(), server.URL, jsonData)
	if err == nil {
		t.Errorf("Expected error on jsonrpc error")
	}
}

func TestErrorOnFailedToRespond(t *testing.T) {
	jsonData, _ := makePostRequestData("a1b2c3d4", "test-api-key", version.MevTMVersion)
	err := attemptRegisterOnce(log.NewNopLogger(), "http://1.2.3.4:6969", jsonData)
	if err == nil {
		t.Errorf("Expected error on fake server")
	}
}
