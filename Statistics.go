/*
File Name:  Statistics.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner

Code to collect the necessary data for creating the KPIs:
* Daily active peers
* Full log of new peers

Every 10 second it will write the statistics file. This gives incoming peers some time to connect to both IPv4 and IPv6.
*/

package main

import (
	"log"
	"path"
	"sync"
	"time"

	"github.com/PeernetOfficial/core"
	"github.com/PeernetOfficial/core/btcec"
	"github.com/PeernetOfficial/core/protocol"
	"github.com/robfig/cron/v3"
)

const filenameDailySummary = "Daily Active Peers.csv"

type peerStat struct {
	added         time.Time                            // Added to the list of stats
	peer          *core.PeerInfo                       // Full peer details
	peerID        [btcec.PubKeyBytesLenCompressed]byte // Peer ID compressed form
	isRootPeer    bool                                 // Whether the peer is a trusted root peer.
	isNAT         bool                                 // Whether the peer is behind a NAT.
	isPortForward bool                                 // Whether the peer uses a forwarded port.
	isFirewall    bool                                 // Reported to be behind a firewall
	connection4   *core.Connection                     // IPv4 connection
	connection6   *core.Connection                     // IPv4 connection
}

// timeStat is the collection of statistics for a given timeframe (like per day, week, etc.)
type timeStat struct {
	countActive      uint64 // Count of active peers
	countRoot        uint64 // Count of root peers
	countNAT         uint64 // Count of peers behind a NAT
	countPortForward uint64 // Count of peers with port forwarding enabled
	countFirewall    uint64 // Count of peers reported behind a firewall
}

// Wait time (for IPv4/IPv6 connections) before writing full peer details into log file.
const peerWaitTime = 10 // seconds

// Map of all known peer IDs today for deduplication. Resets at midnight.
var todayPeers map[[btcec.PubKeyBytesLenCompressed]byte]struct{}
var todayPeersMutex sync.Mutex

// dailyStat is todays current statistics
var dailyStat timeStat

// summaryDaily contains all daily records
var summaryDaily []recordSummaryDaily

func initStatistics(backend *core.Backend) {
	if config.DatabaseFolder == "" {
		return
	}

	todayPeers = make(map[[btcec.PubKeyBytesLenCompressed]byte]struct{})

	// All new peers waiting to be added to the CSV list after the wait time.
	// Waiting makes sure that both IPv4 and IPv6 connections are recorded.
	statQueue := make(map[[btcec.PubKeyBytesLenCompressed]byte]*peerStat)
	var statQueueMutex sync.Mutex

	newRecordsChan := make(chan *peerStat)
	var newRecordsChanMutex sync.Mutex

	var err error
	var filename string

	filename, dailyStat, err = createDailyLog(config.DatabaseFolder, newRecordsChan)
	if err != nil {
		log.Printf("Error opening daily statistics file '%s': %s\n", filename, err.Error())
		return
	}

	// Read the daily summary file.
	summaryDailyFilename := path.Join(config.DatabaseFolder, filenameDailySummary)
	summaryDaily, err = statReadSummary(summaryDailyFilename)

	// Every midnight create a new database file.
	c := cron.New(cron.WithLocation(time.UTC))
	c.AddFunc("0 0 * * *", func() {
		// write last day into summary file "Daily Active Peers.csv"
		summaryDaily = append(summaryDaily, recordSummaryDaily{Date: time.Now().UTC().Round(time.Hour * 24), stats: dailyStat})
		statWriteSummary(summaryDailyFilename, dailyStat)

		// reset daily peer list and counter
		todayPeersMutex.Lock()
		todayPeers = make(map[[btcec.PubKeyBytesLenCompressed]byte]struct{})
		todayPeersMutex.Unlock()

		dailyStat.countActive = 0
		dailyStat.countNAT = 0
		dailyStat.countPortForward = 0
		dailyStat.countRoot = 0
		dailyStat.countFirewall = 0

		// close daily log and create new one
		newRecordsChanMutex.Lock()
		close(newRecordsChan)
		newRecordsChan = make(chan *peerStat)
		newRecordsChanMutex.Unlock()

		if filename, dailyStat, err = createDailyLog(config.DatabaseFolder, newRecordsChan); err != nil {
			log.Printf("Error opening daily statistics file '%s' at midnight: %s\n", filename, err.Error())
		}

		// Process all current connected peers
		statQueueCurrentPeers(backend)
	})
	c.Start()

	// register the filter to be called each time a new peer is discovered
	backend.Filters.NewPeer = func(peer *core.PeerInfo, connection *core.Connection) {
		// New peers are added to the wait list, and after 10 seconds written into the file.
		// This gives peers a little bit of time to connect both via IPv4 and IPv6.
		peerID := publicKey2Compressed(peer.PublicKey)
		todayPeersMutex.Lock()
		_, ok := todayPeers[peerID]
		todayPeersMutex.Unlock()
		if ok {
			return
		}

		stat := &peerStat{
			added:      time.Now(),
			peerID:     peerID,
			isRootPeer: peer.IsRootPeer,
			peer:       peer,
		}

		todayPeers[peerID] = struct{}{}
		statQueue[peerID] = stat
	}

	// filter for each new peer connection
	backend.Filters.NewPeerConnection = func(peer *core.PeerInfo, connection *core.Connection) {
		// process the new connection only if the peers is in queue
		peerID := publicKey2Compressed(peer.PublicKey)
		statQueueMutex.Lock()
		stat, ok := statQueue[peerID]
		statQueueMutex.Unlock()
		if !ok {
			return
		}

		// match IPv4/IPV6
		if connection.IsIPv4() && stat.connection4 == nil {
			stat.connection4 = connection
		} else if connection.IsIPv6() && stat.connection6 == nil {
			stat.connection6 = connection
		}
	}

	// process the queue every 10 seconds for writeout
	go func() {
		for {
			time.Sleep(time.Second * peerWaitTime)

			threshold := time.Now().Add(-time.Second * peerWaitTime)
			statQueueMutex.Lock()

			for id, stat := range statQueue {
				if stat.added.After(threshold) {
					continue
				}
				delete(statQueue, id)

				// process
				stat.isNAT = (stat.connection4 != nil && stat.connection4.IsBehindNAT()) || (stat.connection6 != nil && stat.connection6.IsBehindNAT())
				stat.isPortForward = (stat.connection4 != nil && stat.connection4.IsPortForward()) || (stat.connection6 != nil && stat.connection6.IsPortForward())
				stat.isFirewall = stat.peer.IsFirewallReported()

				// register the counts
				dailyStat.countActive++

				if stat.isRootPeer {
					dailyStat.countRoot++
				}
				if stat.isNAT {
					dailyStat.countNAT++
				}
				if stat.isPortForward {
					dailyStat.countPortForward++
				}
				if stat.isFirewall {
					dailyStat.countFirewall++
				}

				// send as record
				newRecordsChanMutex.Lock()
				newRecordsChan <- stat
				newRecordsChanMutex.Unlock()
			}

			statQueueMutex.Unlock()
		}
	}()
}

func publicKey2Compressed(publicKey *btcec.PublicKey) [btcec.PubKeyBytesLenCompressed]byte {
	var key [btcec.PubKeyBytesLenCompressed]byte
	copy(key[:], publicKey.SerializeCompressed())
	return key
}

func (stat *peerStat) Flags() (flags string) {
	if stat.isRootPeer {
		flags += "R"
	}

	if stat.isNAT {
		flags += "N"
	}

	if stat.isPortForward {
		flags += "P"
	}

	if stat.peer.Features&(1<<protocol.FeatureIPv4Listen) > 0 {
		flags += "4"
	}

	if stat.peer.Features&(1<<protocol.FeatureIPv6Listen) > 0 {
		flags += "6"
	}

	if stat.peer.Features&(1<<protocol.FeatureFirewall) > 0 {
		flags += "F"
	}

	return flags
}

// statQueueCurrentPeers records all currently connected peers for statistics
func statQueueCurrentPeers(backend *core.Backend) {
	if backend.Filters.NewPeer == nil || backend.Filters.NewPeerConnection == nil {
		return
	}

	for _, peer := range backend.PeerlistGet() {
		backend.Filters.NewPeer(peer, nil)

		connections := peer.GetConnections(true)
		connections = append(connections, peer.GetConnections(false)...)

		for _, connection := range connections {
			backend.Filters.NewPeerConnection(peer, connection)
		}
	}
}
