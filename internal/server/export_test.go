package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"testing"
)

// StartFiltered serves a trivial handler behind filterListener (tests).
func StartFiltered(t *testing.T, port int, proxy netip.Addr) (*http.Server, string) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	fl := filterListener{Listener: ln, allow: func(ip netip.Addr) bool { return ip == proxy }}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })}
	go srv.Serve(fl)
	return srv, fmt.Sprintf("http://127.0.0.1:%d/", port)
}
