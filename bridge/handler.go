package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// TokenResolver maps a bridge token to a player id. The store implements it.
type TokenResolver interface {
	PlayerIDForBridgeToken(ctx context.Context, token string) (string, error)
}

// Handler serves the two bridge routes. Mount at the game server root.
func Handler(h *Hub, tokens TokenResolver) http.Handler {
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" || tok == r.Header.Get("Authorization") {
			http.Error(w, "missing bridge token", http.StatusUnauthorized)
			return "", false
		}
		pid, err := tokens.PlayerIDForBridgeToken(r.Context(), tok)
		if err != nil || pid == "" {
			http.Error(w, "invalid bridge token", http.StatusUnauthorized)
			return "", false
		}
		return pid, true
	}
	mux.HandleFunc("GET /bridge/poll", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := auth(w, r)
		if !ok {
			return
		}
		req, ok := h.Poll(r.Context(), pid)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(req)
	})
	mux.HandleFunc("POST /bridge/result/{id}", func(w http.ResponseWriter, r *http.Request) {
		pid, ok := auth(w, r)
		if !ok {
			return
		}
		var res Result
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&res); err != nil {
			http.Error(w, "bad result body", http.StatusBadRequest)
			return
		}
		if err := h.Submit(pid, r.PathValue("id"), res); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
