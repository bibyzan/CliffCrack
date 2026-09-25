package coordinator

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ICEServer is a STUN or TURN server for WebRTC, as players are told of it
// (the same shape as a browser's RTCIceServer).
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// DefaultSTUN lets players behind ordinary home routers learn their public
// address and connect directly. It's free and needs no account.
var DefaultSTUN = []ICEServer{{URLs: []string{"stun:stun.cloudflare.com:3478", "stun:stun.l.google.com:19302"}}}

// ICEProvider gives the servers a player should use. TURN credentials are
// short-lived, so it's asked each time a player connects.
type ICEProvider func(ctx context.Context) ([]ICEServer, error)

// Static always gives the same servers.
func Static(servers []ICEServer) ICEProvider {
	return func(context.Context) ([]ICEServer, error) { return servers, nil }
}

// SharedSecretTURN gives time-limited credentials for a TURN server set up
// with a shared secret (coturn's use-auth-secret, the "TURN REST API"):
// the username is an expiry time, the password its HMAC.
func SharedSecretTURN(urls []string, secret string, ttl time.Duration) ICEProvider {
	return func(context.Context) ([]ICEServer, error) {
		user := fmt.Sprintf("%d:cliffcrack", time.Now().Add(ttl).Unix())
		mac := hmac.New(sha1.New, []byte(secret))
		mac.Write([]byte(user))
		return []ICEServer{{URLs: urls, Username: user, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))}}, nil
	}
}

// CloudflareTURN asks Cloudflare's TURN service (Realtime > TURN in the
// dashboard) for credentials, keeping them until halfway to expiry.
func CloudflareTURN(keyID, apiToken string, ttl time.Duration) ICEProvider {
	var mu sync.Mutex
	var cached []ICEServer
	var until time.Time
	return func(ctx context.Context) ([]ICEServer, error) {
		mu.Lock()
		defer mu.Unlock()
		if time.Now().Before(until) {
			return cached, nil
		}
		body, _ := json.Marshal(map[string]int{"ttl": int(ttl.Seconds())})
		url := "https://rtc.live.cloudflare.com/v1/turn/keys/" + keyID + "/credentials/generate-ice-servers"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+apiToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("cloudflare turn: %s", resp.Status)
		}
		var out struct {
			ICEServers json.RawMessage `json:"iceServers"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		// A list of servers, or (from the older endpoint) just one.
		var servers []ICEServer
		if err := json.Unmarshal(out.ICEServers, &servers); err != nil {
			var one ICEServer
			if err := json.Unmarshal(out.ICEServers, &one); err != nil {
				return nil, fmt.Errorf("cloudflare turn: %w", err)
			}
			servers = []ICEServer{one}
		}
		cached, until = servers, time.Now().Add(ttl/2)
		return servers, nil
	}
}

// ICEFromEnv picks the TURN provider from the environment (fly secrets):
//
//	CF_TURN_KEY_ID, CF_TURN_API_TOKEN     Cloudflare's TURN service
//	TURN_URLS, TURN_SECRET                a TURN server with a shared secret (coturn)
//	TURN_URLS, TURN_USERNAME, TURN_CREDENTIAL   fixed credentials
//
// with TURN_URLS a comma-separated list (turn:host:3478,turns:host:5349).
// It returns nil with none of them set: STUN only.
func ICEFromEnv() (ICEProvider, string) {
	urls := splitList(os.Getenv("TURN_URLS"))
	switch {
	case os.Getenv("CF_TURN_KEY_ID") != "" && os.Getenv("CF_TURN_API_TOKEN") != "":
		return CloudflareTURN(os.Getenv("CF_TURN_KEY_ID"), os.Getenv("CF_TURN_API_TOKEN"), 24*time.Hour), "Cloudflare TURN"
	case len(urls) > 0 && os.Getenv("TURN_SECRET") != "":
		return SharedSecretTURN(urls, os.Getenv("TURN_SECRET"), 24*time.Hour), "TURN (shared secret)"
	case len(urls) > 0:
		return Static([]ICEServer{{URLs: urls, Username: os.Getenv("TURN_USERNAME"), Credential: os.Getenv("TURN_CREDENTIAL")}}), "TURN"
	}
	return nil, "STUN only (no TURN configured)"
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// iceServers are the servers to tell a player of: STUN, and TURN if set up
// (a TURN failure is logged, and the player gets STUN alone).
func (s *Server) iceServers(ctx context.Context) []ICEServer {
	out := append([]ICEServer(nil), DefaultSTUN...)
	if s.TURN == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	turn, err := s.TURN(ctx)
	if err != nil {
		s.logf("coordinator: TURN credentials: %v", err)
		return out
	}
	return append(out, turn...)
}
