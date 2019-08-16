// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"

	. "github.com/asdvxgxasjab/miner"
	"github.com/cruzbit/cruzbit"
	"golang.org/x/crypto/ed25519"
)

func main() {
	// flags
	pubKeyPtr := flag.String("pubkey", "", "A public key which receives newly mined block rewards")
	peerPtr := flag.String("peer", "", "Address of a peer to connect to for receiving work")
	keyFilePtr := flag.String("keyfile", "", "Path to a file containing public keys to use when mining")
	flag.Parse()

	if len(*peerPtr) == 0 {
		log.Fatal("-peer parameter required")
	}

	// add default port, if one was not supplied
	if i := strings.LastIndex(*peerPtr, ":"); i < 0 {
		*peerPtr = *peerPtr + ":" + strconv.Itoa(cruzbit.DEFAULT_CRUZBIT_PORT)
	}

	var pubKeys []ed25519.PublicKey
	if len(*pubKeyPtr) == 0 && len(*keyFilePtr) == 0 {
		log.Fatal("-pubkey or -keyfile argument required to receive newly mined block rewards")
	}
	if len(*pubKeyPtr) != 0 && len(*keyFilePtr) != 0 {
		log.Fatal("Specify only one of -pubkey or -keyfile but not both")
	}
	var err error
	pubKeys, err = loadPublicKeys(*pubKeyPtr, *keyFilePtr)
	if err != nil {
		log.Fatal(err)
	}

	// initialize OpenCL devices
	var deviceCount int
	if OPENCL_ENABLED {
		deviceCount = OpenCLInit()
		if deviceCount == -1 {
			log.Fatal("Error initializing devices")
		}
		log.Printf("OpenCL initialized with %d device(s)\n", deviceCount)
	}

	// load genesis block
	genesisBlock := new(cruzbit.Block)
	if err := json.Unmarshal([]byte(cruzbit.GenesisBlockJson), genesisBlock); err != nil {
		log.Fatal(err)
	}

	genesisID, err := genesisBlock.ID()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Starting up...")
	log.Printf("Genesis block ID: %s\n", genesisID)

	// create and run miners
	var miners []*Miner
	for i := 0; i < deviceCount; i++ {
		miner := NewMiner(genesisID, *peerPtr, pubKeys, i)
		miners = append(miners, miner)
		miner.Run()
	}

	// shutdown on ctrl-c
	c := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(c, os.Interrupt)

	go func() {
		defer close(done)
		<-c

		log.Println("Shutting down...")
		for _, miner := range miners {
			miner.Shutdown()
		}
	}()

	log.Println("Miner started")
	<-done
	log.Println("Exiting")
}

func loadPublicKeys(pubKeyEncoded, keyFile string) ([]ed25519.PublicKey, error) {
	var pubKeysEncoded []string
	var pubKeys []ed25519.PublicKey

	if len(pubKeyEncoded) != 0 {
		pubKeysEncoded = append(pubKeysEncoded, pubKeyEncoded)
	} else {
		file, err := os.Open(keyFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			pubKeysEncoded = append(pubKeysEncoded, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		if len(pubKeysEncoded) == 0 {
			return nil, fmt.Errorf("No public keys found in '%s'", keyFile)
		}
	}

	for _, pubKeyEncoded = range pubKeysEncoded {
		pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyEncoded)
		if len(pubKeyBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("Invalid public key: %s\n", pubKeyEncoded)
		}
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, ed25519.PublicKey(pubKeyBytes))
	}
	return pubKeys, nil
}
