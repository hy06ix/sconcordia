package simulation

import (
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hy06ix/onet"
	"github.com/hy06ix/onet/log"
	"github.com/hy06ix/onet/network"
	"github.com/hy06ix/onet/simul/monitor"
	sconcordia "github.com/hy06ix/sconcordia/service"
	"github.com/jinzhu/copier"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
)

// Name is the name of the simulation
var Name = "sconcordia"

func init() {
	onet.SimulationRegister(Name, NewSimulation)
}

// config being passed down to all nodes, each one takes the relevant
// information
type config struct {
	Seed              int64
	Threshold         int // threshold of the threshold sharing scheme
	BlockSize         int // the size of the block in bytes
	BlockTime         int // blocktime in seconds
	GossipTime        int
	GossipPeers       int // number of neighbours that each node will gossip messages to
	CommunicationMode int // 0 for broadcast, 1 for gossip
	MaxRoundLoops     int // maximum times a node can loop on a round before alerting
	ShardNum          int // Number of shards
}

type Simulation struct {
	onet.SimulationBFTree
	config
	BlockSize int // in bytes
}

func NewSimulation(config string) (onet.Simulation, error) {
	s := &Simulation{}
	_, err := toml.Decode(config, s)
	return s, err
}

func (s *Simulation) Setup(dir string, hosts []string) (*onet.SimulationConfig, error) {
	sim := new(onet.SimulationConfig)
	s.CreateRoster(sim, hosts, 2000)
	s.CreateTree(sim)
	// create the shares manually
	return sim, nil
}

func (s *Simulation) DistributeConfig(config *onet.SimulationConfig, sconcordias []*sconcordia.SConcordia) {
	shardSize := len(config.Roster.List) / s.ShardNum

	rosters := make([]*onet.Roster, s.ShardNum)
	shareList := make([][]*share.PriShare, s.ShardNum)
	publicList := make([]*share.PubPoly, s.ShardNum)
	commitList := make([][]kyber.Point, s.ShardNum)
	interShard := make([]*network.ServerIdentity, s.ShardNum)

	for i := 0; i < s.ShardNum; i++ {
		rosters[i] = onet.NewRoster(config.Roster.List[i*shardSize : (i+1)*shardSize])
		shareList[i], publicList[i] = dkg(s.Threshold, shardSize)
		_, commitList[i] = publicList[i].Info()
	}

	sconfigs := make([]*onet.SimulationConfig, s.ShardNum)
	sconfigs[0] = config

	for i, si := range config.Roster.List {
		shardID := i / shardSize
		shardIndex := i % shardSize

		c := &sconcordia.Config{
			Roster:            rosters[shardID],
			Index:             shardIndex,
			N:                 shardSize,
			Threshold:         s.Threshold,
			CommunicationMode: s.CommunicationMode,
			GossipTime:        s.GossipTime,
			GossipPeers:       s.GossipPeers,
			Public:            commitList[shardID],
			Share:             shareList[shardID][shardIndex],
			BlockSize:         s.BlockSize,
			MaxRoundLoops:     s.MaxRoundLoops,
			RoundsToSimulate:  s.Rounds,
			ShardID:           shardID,
			InterShard:        interShard,
		}

		if i == 0 {
			c.InterShard[shardID] = config.Roster.List[i]

			sconcordias[0] = config.GetService(sconcordia.Name).(*sconcordia.SConcordia)
			sconcordias[0].SetConfig(c)
			//sconcordias[0].GetInfo()
		} else if i%shardSize == 0 {
			port := 8080 + shardID
			address := "127.0.0.1:" + strconv.Itoa(port)
			server := onet.NewServerTCP(network.NewServerIdentity(commitList[shardID][0], network.NewAddress("tcp", address)), sconcordia.Suite)

			rosters[shardID].List[0] = server.ServerIdentity

			sconfig := new(onet.SimulationConfig)
			copier.Copy(&sconfig, &config)
			sconfig.Server = server
			sconfig.Roster = rosters[shardID]
			sconfig.Tree = onet.NewTree(rosters[shardID], onet.NewTreeNode(0, rosters[shardID].List[0]))
			sconfig.Overlay = onet.NewOverlay(server)

			sconfigs[shardID] = sconfig

			c.InterShard[shardID] = server.ServerIdentity

			sconcordias[shardID] = sconfigs[shardID].GetService(sconcordia.Name).(*sconcordia.SConcordia)
			sconcordias[shardID].SetConfig(c)
			// sconcordias[shardID] = server.Service(Name).(*sconcordia.SConcordia)
			// sconcordias[shardID].SetConfig(c)
			//sconcordias[shardID].GetInfo()
		} else {
			// config.Server.Send(si, c)
			// server.Send(si, c)
			sconfigs[shardID].Server.Send(si, c)
		}
	}
}

func (s *Simulation) Run(config *onet.SimulationConfig) error {
	sconcordias := make([]*sconcordia.SConcordia, s.ShardNum)

	log.Lvl1("distributing config to all nodes...")
	s.DistributeConfig(config, sconcordias)
	log.Lvl1("Sleeping for the config to dispatch correctly")
	time.Sleep(3 * time.Second)
	log.Lvl1("Starting concordia simulation")

	var roundDone int
	done := make(chan bool)
	var fullRound *monitor.TimeMeasure
	newRoundCb := func(round int, shardID int) {
		fullRound.Record()
		roundDone++
		log.Lvl1("Simulation Round #", round, "finished")
		if roundDone > s.Rounds {
			done <- true
		} else {
			fullRound = monitor.NewTimeMeasure("fullRound")
		}
	}

	// shardSize := len(config.Roster.List) / s.ShardNum
	for i := 0; i < s.ShardNum; i++ {
		sconcordias[i].AttachCallback(newRoundCb)
	}

	fullTime := monitor.NewTimeMeasure("finalizing")
	fullRound = monitor.NewTimeMeasure("fullRound")

	log.Lvl1(sconcordias)

	for i := 0; i < s.ShardNum; i++ {
		go sconcordias[i].Start()
	}

	for i := 0; i <= (s.ShardNum-1)*11; i++ {
		<-done
	}

	fullTime.Record()
	monitor.RecordSingleMeasure("blocks", float64(roundDone))
	monitor.RecordSingleMeasure("avgRound", fullTime.Wall.Value/float64(s.Rounds))
	log.Lvl1(" ---------------------------")
	log.Lvl1("End of simulation => ", roundDone, " rounds done")
	log.Lvl1("Last full round = ", fullRound.Wall.Value)
	log.Lvl1("Total time = ", fullTime.Wall.Value)
	log.Lvl1("Avg round = ", fullTime.Wall.Value/float64(s.Rounds))
	log.Lvl1(" ---------------------------")
	return nil
}
