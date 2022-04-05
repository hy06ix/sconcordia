package service

import (
	"github.com/hy06ix/onet"
	"github.com/hy06ix/onet/log"
	"github.com/hy06ix/onet/network"
	"go.dedis.ch/kyber/v3/pairing/bn256"
)

var Suite = bn256.NewSuite()
var G2 = Suite.G2()
var Name = "sconcordia"

func init() {
	onet.RegisterNewService(Name, NewSConcordiaService)
}

// SConcordia service is either a beacon a notarizer or a block maker
type SConcordia struct {
	*onet.ServiceProcessor
	context       *onet.Context
	c             *Config
	node          *Node
	blockChain    *BlockChain
	backboneChain *BlockChain
}

// NewSConcordiaService
func NewSConcordiaService(c *onet.Context) (onet.Service, error) {
	n := &SConcordia{
		context:          c,
		ServiceProcessor: onet.NewServiceProcessor(c),
	}
	c.RegisterProcessor(n, ConfigType)
	c.RegisterProcessor(n, BootstrapType)
	c.RegisterProcessor(n, BlockProposalType)
	c.RegisterProcessor(n, NotarizedBlockType)

	c.RegisterProcessor(n, TransactionProofType)
	c.RegisterProcessor(n, NotarizedRefBlockType)
	c.RegisterProcessor(n, BlockHeaderType)

	return n, nil
}

func (sc *SConcordia) SetNode(node *Node) {
	sc.node = node
}

func (sc *SConcordia) GetInfo() {
	log.Lvl1(sc.context)
	log.Lvl1(sc.c)
	log.Lvl1(sc.node)
	log.Lvl1(sc.blockChain)
	log.Lvl1(sc.backboneChain)
}

func (sc *SConcordia) SetConfig(c *Config) {
	sc.c = c
	if sc.c.CommunicationMode == 0 {
		sc.node = NewNodeProcess(sc.context, c, sc.broadcast, sc.gossip, sc.send)
	} else if sc.c.CommunicationMode == 1 {
		sc.node = NewNodeProcess(sc.context, c, sc.broadcast, sc.gossip, sc.send)
	} else {
		panic("Invalid communication mode")
	}
}

func (sc *SConcordia) AttachCallback(fn func(int, int)) {
	// attach to something.. haha lol xd
	if sc.node != nil {
		sc.node.AttachCallback(fn)
	} else {
		log.Lvl1("Could not attach callback, node is nil")
	}
}

func (sc *SConcordia) Start() {
	// send a bootstrap message
	if sc.node != nil {
		sc.node.StartConsensus()
	} else {
		panic("that should not happen")
	}
}

// Process
func (sc *SConcordia) Process(e *network.Envelope) {
	switch inner := e.Msg.(type) {
	case *Config:
		sc.SetConfig(inner)
	case *Bootstrap:
		sc.node.Process(e)
	case *BlockProposal:
		sc.node.Process(e)
	case *BlockHeader:
		sc.node.Process(e)
	case *NotarizedRefBlock:
		sc.node.Process(e)
	case *NotarizedBlock:
		sc.node.Process(e)
	case *TransactionProof:
		sc.node.Process(e)
	default:
		log.Lvl1("Received unidentified message")
	}
}

// depreciated
func (sc *SConcordia) getRandomPeers(numPeers int) []*network.ServerIdentity {
	var results []*network.ServerIdentity
	for i := 0; i < numPeers; {
		posPeer := sc.c.Roster.RandomServerIdentity()
		if sc.ServerIdentity().Equal(posPeer) {
			// selected itself
			continue
		}
		results = append(results, posPeer)
		i++
	}
	return results
}

type BroadcastFn func(sis []*network.ServerIdentity, msg interface{})

func (sc *SConcordia) broadcast(sis []*network.ServerIdentity, msg interface{}) {
	for _, si := range sis {
		if sc.ServerIdentity().Equal(si) {
			continue
		}
		log.Lvlf4("Broadcasting from: %s to: %s", sc.ServerIdentity(), si)
		if err := sc.ServiceProcessor.SendRaw(si, msg); err != nil {
			log.Lvl1("Error sending message")
			//panic(err)
		}
	}
}

func (sc *SConcordia) gossip(sis []*network.ServerIdentity, msg interface{}) {
	//targets := n.getRandomPeers(n.c.GossipPeers)
	targets := sc.node.c.Roster.RandomSubset(sc.ServerIdentity(), sc.c.GossipPeers).List
	for k, target := range targets {
		if k == 0 {
			continue
		}
		log.Lvlf4("Gossiping from: %s to: %s", sc.ServerIdentity(), target)
		if err := sc.ServiceProcessor.SendRaw(target, msg); err != nil {
			log.Lvl1("Error sending message")
		}
	}
}

type DirectSendFn func(sis *network.ServerIdentity, msg interface{})

// function for send header to reference shard directly
func (sc *SConcordia) send(si *network.ServerIdentity, msg interface{}) {
	// log.Lvl1(reflect.TypeOf(msg))
	log.Lvlf4("Sending from: %s to: %s", sc.ServerIdentity(), si)
	if err := sc.ServiceProcessor.SendRaw(si, msg); err != nil {
		log.Lvl1("Error sending message")
	}
}
