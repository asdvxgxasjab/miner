// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package miner

import (
	"github.com/cruzbit/cruzbit"
	"golang.org/x/crypto/ed25519"
)

// Protocol is the name of this version of the cruzbit peer protocol.
const Protocol = "cruzbit.1"

// Message is a message frame for all messages in the cruzbit.1 protocol.
type Message struct {
	Type string      `json:"type"`
	Body interface{} `json:"body,omitempty"`
}

// GetWorkMessage is used by a mining peer to request mining work.
// Type: "get_work"
type GetWorkMessage struct {
	PublicKeys []ed25519.PublicKey `json:"public_keys"`
	Memo       string              `json:"memo,omitempty"`
}

// WorkMessage is used by a client to send work to perform to a mining peer.
// The timestamp and nonce in the header can be manipulated by the mining peer.
// It is the mining peer's responsibility to ensure the timestamp is not set below
// the minimum timestamp and that the nonce does not exceed MAX_NUMBER (2^53-1).
// Type: "work"
type WorkMessage struct {
	WorkID  int32                `json:"work_id"`
	Header  *cruzbit.BlockHeader `json:"header"`
	MinTime int64                `json:"min_time"`
	Target  *cruzbit.BlockID     `json:"target,omitempty"` // optional pool target
	Error   string               `json:"error,omitempty"`
}

// SubmitWorkMessage is used by a mining peer to submit a potential solution to the client.
// Type: "submit_work"
type SubmitWorkMessage struct {
	WorkID int32                `json:"work_id"`
	Header *cruzbit.BlockHeader `json:"header"`
}

// SubmitWorkResultMessage is used to inform a mining peer of the result of its work.
// Type: "submit_work_result"
type SubmitWorkResultMessage struct {
	WorkID int32  `json:"work_id"`
	Error  string `json:"error,omitempty"`
}
