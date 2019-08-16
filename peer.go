// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package miner

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/cruzbit/cruzbit"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ed25519"
)

// Peer is a client in the cruzbit network providing us with mining work.
type Peer struct {
	conn       *websocket.Conn
	genesisID  cruzbit.BlockID
	workChan   chan<- WorkMessage
	submitChan <-chan SubmitWorkMessage
	wg         sync.WaitGroup
}

// NewPeer returns a new instance of a peer.
func NewPeer(genesisID cruzbit.BlockID, workChan chan<- WorkMessage, submitChan <-chan SubmitWorkMessage) *Peer {
	return &Peer{genesisID: genesisID, workChan: workChan, submitChan: submitChan}
}

var tlsClientConfig *tls.Config = &tls.Config{
	RootCAs:                  nil,
	InsecureSkipVerify:       true,
	MinVersion:               tls.VersionTLS12,
	CurvePreferences:         []tls.CurveID{tls.CurveP256, tls.CurveP384, tls.CurveP521},
	PreferServerCipherSuites: false,
	CipherSuites: []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	},
}

var peerDialer *websocket.Dialer = &websocket.Dialer{
	Proxy:            http.ProxyFromEnvironment,
	HandshakeTimeout: 15 * time.Second,
	Subprotocols:     []string{Protocol}, // set in protocol.go
	TLSClientConfig:  tlsClientConfig,
}

// Connect connects outbound to a peer.
func (p *Peer) Connect(ctx context.Context, addr string, pubKeys []ed25519.PublicKey) (int, error) {
	u := url.URL{Scheme: "wss", Host: addr, Path: "/" + p.genesisID.String()}
	log.Printf("Connecting to %s", u.String())

	// specify timeout via context. if the parent context is cancelled
	// we'll also abort the connection.
	dialCtx, cancel := context.WithTimeout(ctx, connectWait)
	defer cancel()

	var statusCode int
	conn, resp, err := peerDialer.DialContext(dialCtx, u.String(), nil)
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		return statusCode, err
	}

	p.conn = conn

	// send get_work
	gw := Message{Type: "get_work", Body: GetWorkMessage{PublicKeys: pubKeys}}
	p.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := p.conn.WriteJSON(gw); err != nil {
		return 0, err
	}

	return statusCode, nil
}

// Shutdown is called to shutdown the underlying WebSocket synchronously.
func (p *Peer) Shutdown() {
	var addr string
	if p.conn != nil {
		addr = p.conn.RemoteAddr().String()
		p.conn.Close()
	}
	p.wg.Wait()
	if len(addr) != 0 {
		log.Printf("Closed connection with %s\n", addr)
	}
}

const (
	// Time allowed to wait for WebSocket connection
	connectWait = 10 * time.Second

	// Time allowed to write a message to the peer
	writeWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 120 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = pongWait / 2
)

// Run executes the peer's main loop in its own goroutine.
// It manages reading and writing to the peer's WebSocket and facilitating the protocol.
func (p *Peer) Run() {
	p.wg.Add(1)
	go p.run()
}

func (p *Peer) run() {
	defer p.wg.Done()
	defer p.conn.Close()

	// writer goroutine loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// send the peer pings
		tickerPing := time.NewTicker(pingPeriod)
		defer tickerPing.Stop()

		for {
			select {
			case sw, ok := <-p.submitChan:
				if !ok {
					// miner is exiting
					return
				}

				// send outgoing submit_work message to peer
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteJSON(sw); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}

			case <-tickerPing.C:
				p.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := p.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("Write error: %s, to: %s\n", err, p.conn.RemoteAddr())
					p.conn.Close()
				}
			}
		}
	}()

	// handle pongs
	p.conn.SetPongHandler(func(string) error {
//		p.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// set initial read deadline
//	p.conn.SetReadDeadline(time.Now().Add(pongWait))

	// reader loop
	for {
		// new message from peer
		messageType, message, err := p.conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %s, from: %s\n", err, p.conn.RemoteAddr())
			break
		}

		switch messageType {
		case websocket.TextMessage:
			var body json.RawMessage
			m := Message{Body: &body}
			if err := json.Unmarshal([]byte(message), &m); err != nil {
				log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
				return
			}

			switch m.Type {
			case "work":
				var work WorkMessage
				if err := json.Unmarshal(body, &work); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				p.workChan <- work

			case "submit_work_result":
				var swr SubmitWorkResultMessage
				if err := json.Unmarshal(body, &swr); err != nil {
					log.Printf("Error: %s, from: %s\n", err, p.conn.RemoteAddr())
					return
				}
				log.Printf("Received submit_work_result message, from: %s\n", p.conn.RemoteAddr())
				if len(swr.Error) != 0 {
					log.Printf("Error: %s\n", swr.Error)
					// XXX todo reconnect
				} else {
					log.Println("Share accepted")
				}
			}

		case websocket.CloseMessage:
			log.Printf("Received close message from: %s\n", p.conn.RemoteAddr())
			break
		}
	}
}
