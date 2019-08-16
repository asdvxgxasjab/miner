// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package miner

import (
	"context"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/cruzbit/cruzbit"
	"golang.org/x/crypto/ed25519"
)

// Miner tries to mine a new tip block.
type Miner struct {
	genesisID    cruzbit.BlockID
	peerAddr     string
	pubKeys      []ed25519.PublicKey // receipients of any block rewards we mine
	num          int
	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// NewMiner returns a new Miner instance.
func NewMiner(genesisID cruzbit.BlockID, peerAddr string, pubKeys []ed25519.PublicKey, num int) *Miner {
	return &Miner{
		genesisID:    genesisID,
		peerAddr:     peerAddr,
		pubKeys:      pubKeys,
		num:          num,
		shutdownChan: make(chan struct{}),
	}
}

// Run executes the miner's main loop in its own goroutine.
func (m *Miner) Run() {
	m.wg.Add(1)
	go m.run()
}

func (m *Miner) run() {
	defer m.wg.Done()

	workChan := make(chan WorkMessage, 1)
	submitChan := make(chan SubmitWorkMessage, 1)

	peer := NewPeer(m.genesisID, workChan, submitChan)
	_, err := peer.Connect(context.Background(), m.peerAddr, m.pubKeys)
	if err != nil {
		// XXX TODO: handle errors and reconnects
		panic(err)
	}
	peer.Run()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var hashes, medianTimestamp int64
	var work WorkMessage
	var hasher *BlockHeaderHasher
	var target cruzbit.BlockID
	var targetInt *big.Int

	work.WorkID = -1

	// main mining loop
	for {
		select {
		case work = <-workChan:
			log.Printf("Miner %d received new work, ID: %d\n", m.num, work.WorkID)
			if work.Target != nil {
				target = *work.Target
			} else {
				target = work.Header.Target
			}
			targetInt = target.GetBigInt()
			medianTimestamp = work.MinTime
			hasher = NewBlockHeaderHasher()

		case _, ok := <-m.shutdownChan:
			if !ok {
				close(submitChan)
				peer.Shutdown()
				log.Printf("Miner %d shutting down...\n", m.num)
				return
			}

		case <-ticker.C:
			// update block time every so often
			now := time.Now().Unix()
			if now > medianTimestamp {
				work.Header.Time = now
			}

		default:
			if work.WorkID == -1 {
				time.Sleep(100 * time.Microsecond)
				continue
			}

			// hash the block and check the proof-of-work
			idInt, attempts := hasher.Update(m.num, work.Header, target)
			hashes += attempts
			if idInt.Cmp(targetInt) <= 0 {
				// found a solution
				id := new(cruzbit.BlockID).SetBigInt(idInt)
				log.Printf("Miner %d calculated a share %s, submitting work, ID: %d\n",
					m.num, *id, work.WorkID)
				submitChan <- SubmitWorkMessage{WorkID: work.WorkID, Header: work.Header}
				work.WorkID = -1
			} else {
				// no solution yet
				work.Header.Nonce += attempts
				if work.Header.Nonce > cruzbit.MAX_NUMBER {
					work.Header.Nonce = 0
				}
			}
		}
	}
}

// Shutdown stops the miner synchronously.
func (m *Miner) Shutdown() {
	close(m.shutdownChan)
	m.wg.Wait()
	log.Printf("Miner %d shutdown\n", m.num)
}
