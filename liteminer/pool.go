/*
 *  Brown University, CS138, Spring 2020
 *
 *  Purpose: a LiteMiner mining pool.
 */

package liteminer

import (
	"encoding/gob"
	"io"
	"math"
	"net"
	"sync"

	"go.uber.org/atomic"
)

// HeartbeatTimeout is the time duration at which a pool considers a miner 'dead'
const HeartbeatTimeout = 3 * HeartbeatFreq

//MinerHandle a wrap around the Mine Struct that contains the data of the miner
type MinerHandle struct {
	Conn         MiningConn
	Speed        int
	NumProcessed uint64
	Free         *atomic.Bool
	Job          Interval
	Address      net.Addr
}

//SendMineRequestMessage ....
func (m *MinerHandle) SendMineRequestMessage(pool *Pool) {
	m.Free.Store(false)
	msg := MineRequestMsg(pool.Data, m.Job.Lower, m.Job.Upper)
	SendMsg(m.Conn, msg)
}

// Pool represents a LiteMiner mining pool
type Pool struct {
	Addr net.Addr

	Miners    map[net.Addr]MiningConn // Currently connected miners
	minersMtx sync.Mutex              // Mutex for concurrent access to miners

	Client    MiningConn // The current client
	clientMtx sync.Mutex // Mutex for concurrent access to Client

	busy *atomic.Bool // True when processing a transaction

	jobs        chan Interval
	intervals   []Interval
	intervalMtx sync.Mutex

	//Keep track of which miners are available for processing
	freeMiners chan *MinerHandle
	// freeMiners    map[net.Addr]MinerHandle
	// freeMinersMtx sync.Mutex

	//Transcations To Be Handled
	transactions chan Message
	Data         string

	//Hash and Nonce to be returned
	hash    uint64
	nonce   uint64
	hashMtx sync.Mutex

	//Finisehd jobs
	finished  map[Interval]bool
	finishMtx sync.Mutex

	//Done Checker
	check *sync.Cond
}

// CreatePool creates a new pool at the specified port.
func CreatePool(port string) (*Pool, error) {
	p := &Pool{}
	p.Miners = make(map[net.Addr]MiningConn)
	p.freeMiners = make(chan *MinerHandle, 100)
	p.finished = make(map[Interval]bool)
	p.busy = atomic.NewBool(false)
	p.transactions = make(chan Message, 10000)
	p.jobs = make(chan Interval, 30000)
	p.hash = uint64(math.Inf(1))
	p.check = sync.NewCond(&p.finishMtx)
	err := p.startListener(port)
	if err != nil {
		Err.Fatalf("Error has occured while creating the pool\n%v\n", err)
		return nil, err
	}
	return p, nil
}

// startListener starts listening for new connections.
func (p *Pool) startListener(port string) error {
	listener, portID, err := OpenListener(port)
	if err != nil {
		return err
	}

	Out.Printf("Listening on port %v\n", portID)

	p.Addr = listener.Addr()

	// Listen for and accept connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				Err.Printf("Received error %v when listening for connections\n", err)
				continue
			}

			go p.handleConnection(conn)
		}
	}()

	return nil
}

// handleConnection handles an incoming connection and delegates to
// handleMinerConnection or handleClientConnection.
func (p *Pool) handleConnection(nc net.Conn) {
	// Set up connection
	conn := MiningConn{}
	conn.Conn = nc
	conn.Enc = gob.NewEncoder(nc)
	conn.Dec = gob.NewDecoder(nc)

	// Wait for Hello message
	msg, err := RecvMsg(conn)
	if err != nil {
		Err.Printf(
			"Received error %v when processing Hello message from %v\n",
			err,
			conn.Conn.RemoteAddr(),
		)
		conn.Conn.Close() // Close the connection
		return
	}

	switch msg.Type {
	case MinerHello:
		p.handleMinerConnection(conn)
	case ClientHello:
		p.handleClientConnection(conn)
	default:
		Err.Printf("Pool received unexpcted message type %v (msg=%v)", msg.Type, msg)
		SendMsg(conn, ErrorMsg("Unexpected message type"))
	}
}

// handleClientConnection handles a connection from a client.
func (p *Pool) handleClientConnection(conn MiningConn) {
	Debug.Printf("Received client connection from %v", conn.Conn.RemoteAddr())

	p.clientMtx.Lock()
	if p.Client.Conn != nil {
		Debug.Printf(
			"Busy with client %v, sending BusyPool message to client %v",
			p.Client.Conn.RemoteAddr(),
			conn.Conn.RemoteAddr(),
		)
		SendMsg(conn, BusyPoolMsg())
		p.clientMtx.Unlock()
		return
	}
	p.clientMtx.Unlock()

	if p.busy.Load() {
		Debug.Printf(
			"Busy with previous transaction, sending BusyPool message to client %v",
			conn.Conn.RemoteAddr(),
		)
		SendMsg(conn, BusyPoolMsg())
		return
	}
	p.clientMtx.Lock()
	p.Client = conn
	p.clientMtx.Unlock()

	go p.handleTransaction()
	go p.handleJobs()
	go p.checkForDone()

	// Listen for and handle incoming messages
	for {
		Debug.Print("Waiting for Mine Request From Client")
		msg, err := RecvMsg(conn)
		if err != nil {
			if err == io.EOF {
				Out.Printf("Client %v disconnected\n", conn.Conn.RemoteAddr())

				conn.Conn.Close() // Close the connection

				p.clientMtx.Lock()
				p.Client.Conn = nil
				p.clientMtx.Unlock()

				return
			}
			Err.Printf(
				"Received error %v when processing message from client %v\n",
				err,
				conn.Conn.RemoteAddr(),
			)
			return
		}

		if msg.Type != Transaction {
			SendMsg(conn, ErrorMsg("Expected Transaction message"))
			continue
		}

		Debug.Printf(
			"Received transaction from client %v with data %v and upper bound %v",
			conn.Conn.RemoteAddr(),
			msg.Data,
			msg.Upper,
		)

		p.minersMtx.Lock()
		if len(p.Miners) == 0 {
			SendMsg(conn, ErrorMsg("No miners connected"))
			p.minersMtx.Unlock()
			continue
		}
		p.minersMtx.Unlock()
		p.busy.Store(true)
		p.transactions <- msg
	}
}

func (p *Pool) handleTransaction() {
	for {
		t := <-p.transactions
		Debug.Printf("Handling Transaction of Data %v with Upper Bound %v", t.Data, t.Upper)
		p.Data = t.Data
		p.intervalMtx.Lock()
		p.intervals = GenerateIntervalsBySize(t.Upper, 1000)
		p.intervalMtx.Unlock()
		for _, interval := range p.intervals {
			p.jobs <- interval
		}
	}
}

func (p *Pool) handleJobs() {
	for {
		job := <-p.jobs
		miner := <-p.freeMiners
		miner.Job = job
		miner.SendMineRequestMessage(p)
	}
}

func (p *Pool) clear() {
	p.busy.Store(false)
	p.hash = uint64(math.Inf(1))
	p.intervals = p.intervals[:0]
	p.finished = make(map[Interval]bool)

}

func (p *Pool) checkForDone() {
	p.check.L.Lock()
	if (len(p.intervals) == 0 && len(p.finished) == 0) || len(p.finished) != len(p.intervals) || !p.busy.Load() {
		p.check.Wait()
	}
	msg := ProofOfWorkMsg(p.Data, p.nonce, p.hash)
	SendMsg(p.Client, msg)
	p.clear()
	p.check.L.Unlock()
}

// handleMinerConnection handles a connection from a miner.
func (p *Pool) handleMinerConnection(conn MiningConn) {
	Debug.Printf("Received miner connection from %v", conn.Conn.RemoteAddr())

	p.minersMtx.Lock()
	p.Miners[conn.Conn.RemoteAddr()] = conn
	miner := &MinerHandle{
		Address: conn.Conn.RemoteAddr(),
		Free:    atomic.NewBool(true),
		Conn:    conn,
	}
	p.minersMtx.Unlock()

	p.freeMiners <- miner

	msgChan := make(chan Message)
	go p.receiveFromMiner(conn, msgChan)

	for {
		msg := <-msgChan
		if msg.Type == StatusUpdate {
			//TODO: Add Heartbeat Timer
			Debug.Printf("Recieved a Status Update Message, Num Processed %v from Miner %v", msg.NumProcessed, conn.Conn.RemoteAddr())
			continue
		}

		if msg.Type == ProofOfWork {
			Debug.Printf("Recieved A Proof of Work Miner %v found nonce to be %v", conn.Conn.RemoteAddr(), msg.Nonce)
			p.finishMtx.Lock()
			p.finished[miner.Job] = true
			p.hashMtx.Lock()
			if p.hash > msg.Hash {
				p.hash = msg.Hash
				p.nonce = msg.Nonce
			}
			p.hashMtx.Unlock()
			Debug.Printf("Freeing Miner %v", miner.Address)
			miner.Free.Store(true)
			p.freeMiners <- miner
			p.check.Broadcast()
			p.finishMtx.Unlock()
			Debug.Printf("Miner %v freed", miner.Address)
			continue
		}

		Err.Printf("Recieved an unkown type of message %v", msg.Type)

	}
}

// receiveFromMiner waits for messages from the miner specified by conn and
// forwards them over msgChan.
func (p *Pool) receiveFromMiner(conn MiningConn, msgChan chan Message) {
	for {
		msg, err := RecvMsg(conn)
		if err != nil {
			if _, ok := err.(*net.OpError); ok || err == io.EOF {
				Out.Printf("Miner %v disconnected\n", conn.Conn.RemoteAddr())

				p.minersMtx.Lock()
				delete(p.Miners, conn.Conn.RemoteAddr())
				p.minersMtx.Unlock()

				conn.Conn.Close() // Close the connection

				return
			}
			Err.Printf(
				"Received error %v when processing message from miner %v\n",
				err,
				conn.Conn.RemoteAddr(),
			)
			continue
		}
		msgChan <- msg
	}
}

// GetMiners returns the addresses of any connected miners.
func (p *Pool) GetMiners() []net.Addr {
	p.minersMtx.Lock()
	defer p.minersMtx.Unlock()

	miners := []net.Addr{}
	for _, m := range p.Miners {
		miners = append(miners, m.Conn.RemoteAddr())
	}
	return miners
}

// GetClient returns the address of the current client or nil if there is no
// current client.
func (p *Pool) GetClient() net.Addr {
	p.clientMtx.Lock()
	defer p.clientMtx.Unlock()

	if p.Client.Conn == nil {
		return nil
	}
	return p.Client.Conn.RemoteAddr()
}
