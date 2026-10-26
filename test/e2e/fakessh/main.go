// Command fakessh is a minimal SSH server for the E2E tests: it presents an
// ed25519 host key (so Lor can scan it) and rejects every login.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:0", "listen address")
	flag.Parse()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		log.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, errors.New("fakessh accepts no logins")
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("FAKESSH_ADDR=%s\n", ln.Addr())
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			ssh.NewServerConn(c, cfg)
			c.Close()
		}()
	}
}
