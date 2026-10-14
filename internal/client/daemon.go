package client

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

func Handler(recorder *Recorder) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /prepare-open", func(w http.ResponseWriter, r *http.Request) {
		value, err := recorder.PrepareOpen()
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(value)
	})
	mux.HandleFunc("POST /scan-binding", func(w http.ResponseWriter, r *http.Request) {
		value, err := recorder.BeginScan()
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(value)
	})
	mux.HandleFunc("GET /binding", func(w http.ResponseWriter, r *http.Request) {
		value, err := recorder.RefreshBinding()
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		json.NewEncoder(w).Encode(value)
	})
	for _, action := range []string{"pair", "unlock"} {
		mux.HandleFunc("POST /"+action, func(w http.ResponseWriter, r *http.Request) {
			var input struct{ ClientID, MAC, DisplayName, Password string }
			if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&input); err != nil {
				http.Error(w, "invalid JSON", 400)
				return
			}
			var value Binding
			var err error
			if action == "pair" {
				value, err = recorder.Pair(input.ClientID, input.MAC, input.DisplayName, input.Password)
			} else {
				value, err = recorder.Unlock(input.Password)
			}
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			json.NewEncoder(w).Encode(value)
		})
	}
	mux.HandleFunc("GET /transcript", func(w http.ResponseWriter, r *http.Request) {
		value, err := recorder.Transcript(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(value)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recorder.Status())
	})
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title   string `json:"title"`
			Context string `json:"context"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if err := recorder.Start(body.Title, body.Context); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(recorder.Status())
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if err := recorder.Stop(); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recorder.Status())
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Browsers cannot issue cross-origin control requests to the device daemon.
		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser control is disabled", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func Serve(recorder *Recorder, address string) (*http.Server, net.Listener, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return nil, nil, errors.New("control server must bind to loopback")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, err
	}
	server := &http.Server{Handler: Handler(recorder), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	return server, listener, nil
}
