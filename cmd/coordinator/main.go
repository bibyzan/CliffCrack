// Command coordinator runs the online lobby: room listing and WebRTC
// signalling for CliffCrack's multiplayer (see package coordinator).
//
//	go run ./cmd/coordinator                # listens on :8080
//	go run ./cmd/coordinator -addr :9000
//
// On fly.io it listens on $PORT if set.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"CliffCrack/coordinator"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on ($PORT overrides the port)")
	flag.Parse()
	if port := os.Getenv("PORT"); port != "" {
		*addr = ":" + port
	}
	srv := &http.Server{Addr: *addr, Handler: coordinator.New().Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("coordinator: listening on %s", *addr)
	for _, a := range lanAddrs() {
		log.Printf("coordinator: on this network, point games at  -server ws://%s%s", a, portOf(*addr))
	}
	log.Fatal(srv.ListenAndServe())
}

// lanAddrs are this machine's IPv4 addresses on its local networks.
func lanAddrs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() && ip.IP.IsPrivate() {
			out = append(out, ip.IP.String())
		}
	}
	return out
}

func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return ":" + port
	}
	return ""
}
