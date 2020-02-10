package liteminer

import (
	"encoding/gob"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/atomic"
)

func TestMineOfMiner(t *testing.T) {
	SetDebug(true)
	m := &Miner{
		NumProcessed: atomic.NewUint64(0),
		IsShutdown:   atomic.NewBool(false),
		Mining:       atomic.NewBool(false),
	}
	nonce := m.Mine("msg", 0, 3)
	var expected uint64 = 1

	if nonce != expected {
		t.Errorf("Expected nonce to be %v but it retured %v", expected, nonce)
	}

	t.Log("Success!")
}

func TestHeartBeatOfMiner(t *testing.T) {
	SetDebug(true)
	miner := &Miner{
		NumProcessed: atomic.NewUint64(0),
		IsShutdown:   atomic.NewBool(false),
		Mining:       atomic.NewBool(true),
	}

	dec, enc := net.Pipe()
	defer dec.Close()
	defer enc.Close()

	conn := MiningConn{
		Enc:  gob.NewEncoder(enc),
		Dec:  gob.NewDecoder(dec),
		Conn: enc,
	}

	go miner.sendHeartBeats(conn)
	time.Sleep(3 * HeartbeatFreq)
	msg := Message{}
	err := conn.Dec.Decode(&msg)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	if msg.Type == StatusUpdate {
		t.Log("Success!")
	} else {
		t.Errorf("Message Type should be %v but is %v", StatusUpdate, msg.Type)
	}
}
func TestHeartBeatOfMinerWhenShutdown(t *testing.T) {
	SetDebug(true)
	miner := &Miner{
		NumProcessed: atomic.NewUint64(0),
		IsShutdown:   atomic.NewBool(true),
		Mining:       atomic.NewBool(false),
	}

	dec, enc := net.Pipe()
	defer dec.Close()
	defer enc.Close()

	conn := MiningConn{
		Enc:  gob.NewEncoder(enc),
		Dec:  gob.NewDecoder(dec),
		Conn: enc,
	}

	go miner.sendHeartBeats(conn)
	time.Sleep(3 * HeartbeatFreq)
	msg := Message{}
	err := conn.Dec.Decode(&msg)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
}
