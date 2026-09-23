// Package bridge is a generic reverse tunnel between the game server and
// HTTP services that only the player's own machine can reach: a local
// model (Ollama on a laptop, a GPU box on a home LAN), a plugin they are
// developing, anything that speaks HTTP.
//
// The player runs the separate founder-sim-bridge binary with a list of
// named services. It dials OUT to the game server and long-polls for
// requests, forwards each to the named local service, and posts the
// response back. Nothing listens on the player's machine; nothing is
// exposed to the internet. On the server, any component that would use an
// HTTP base URL can instead use "bridge://<service>" and gets an
// http.RoundTripper that goes through the tunnel. The LLM client is the
// first such component; remote plugins can be the next.
//
// Wire protocol (stdlib HTTP only, so a bridge can be written in anything):
//
//	GET  /bridge/poll                 Authorization: Bearer <bridge token>
//	     -> 200 Request (one pending request), or 204 if none arrived
//	        within the long-poll window. Poll again either way.
//	POST /bridge/result/{id}          Authorization: Bearer <bridge token>
//	     body: Result                 -> 204
//
// The bridge token is per player, shown in Settings, and is the only
// credential. Rotate it there.
package bridge

// Request is one HTTP call the server wants the bridge to make on its
// behalf. Method/Path/Body are relative to the named service's base URL;
// the server never learns where the service actually lives.
type Request struct {
	ID          string `json:"id"`
	Service     string `json:"service"` // e.g. "llm"
	Method      string `json:"method"`
	Path        string `json:"path"` // e.g. "/chat/completions"
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"` // base64 in JSON
}

// Result is what came back from the local service.
type Result struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
	// Error is set when the bridge could not reach the service at all, or
	// does not expose a service by that name.
	Error string `json:"error,omitempty"`
}

// PollWindowSeconds is how long the server holds a poll open before 204.
const PollWindowSeconds = 25

// Scheme is the base-URL scheme that routes a component through the bridge:
// "bridge://llm" means "the service the player's bridge calls llm".
const Scheme = "bridge"
