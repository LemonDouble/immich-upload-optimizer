package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fake immich that only accepts the "valid" API key and records which paths were called.
// Like the real one, it fails with 500 on an empty asset list.
func newFakeImmich(t *testing.T, calledPaths *[]string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calledPaths = append(*calledPaths, r.Method+" "+r.URL.Path)
		if r.Header.Get("x-api-key") != "valid" {
			http.Error(w, `{"message":"Invalid API key"}`, http.StatusUnauthorized)
			return
		}
		var checkRequest bulkUploadCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&checkRequest); err != nil || len(checkRequest.Assets) == 0 {
			http.Error(w, `{"message":"Internal server error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(server.Close)
	previousUpstreamURL, previousDevMITMproxy := upstreamURL, DevMITMproxy
	upstreamURL, DevMITMproxy = server.URL, false
	t.Cleanup(func() { upstreamURL, DevMITMproxy = previousUpstreamURL, previousDevMITMproxy })
}

func newUploadRequest(t *testing.T, apiKey string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("deviceAssetId", "asset-1")
	part, _ := writer.CreateFormFile("assetData", "photo.heic")
	_, _ = part.Write([]byte("not really a heic"))
	_ = writer.Close()
	request := httptest.NewRequest("POST", "/api/assets", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("x-api-key", apiKey)
	return request
}

// An unauthenticated upload must get immich's rejection without the file ever reaching a task.
func TestUnauthenticatedUploadIsRejectedBeforeProcessing(t *testing.T) {
	var calledPaths []string
	newFakeImmich(t, &calledPaths)
	recorder := httptest.NewRecorder()

	err := newJob(newUploadRequest(t, "invalid"), recorder, newCustomLogger(log.New(io.Discard, "", 0), ""))

	if err == nil {
		t.Fatal("expected an error for an unauthenticated upload")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status is %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if len(calledPaths) != 1 || calledPaths[0] != "POST /api/assets/bulk-upload-check" {
		t.Errorf("immich was called with %v, want only the auth check", calledPaths)
	}
}

func TestAuthenticatedUploadPassesPermissionCheck(t *testing.T) {
	var calledPaths []string
	newFakeImmich(t, &calledPaths)
	recorder := httptest.NewRecorder()

	if err := requireUploadPermission(recorder, newUploadRequest(t, "valid")); err != nil {
		t.Fatalf("expected the upload to be allowed, got %v", err)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("nothing should be written to the client yet, got %q", recorder.Body.String())
	}
}
