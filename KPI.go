/*
File Name:  KPI.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner

Collects the data for KPIs related to files shared in Peernet.
It uses the global blockchain cache as source.
*/

package main

import (
	"sync"

	"github.com/PeernetOfficial/core"
	"github.com/PeernetOfficial/core/blockchain"
)

var globalBlockchainStats struct {
	sync.RWMutex
	blockchain.BlockchainStats
}

func startKPIs(backend *core.Backend) {
	// Go through all blockchains to initialize the stats
	backend.GlobalBlockchainCache.Store.IterateBlockchains(func(header *blockchain.MultiBlockchainHeader) {
		if header.Stats.CountFileRecords == 0 && header.Stats.SizeAllFiles == 0 {
			return
		}

		globalBlockchainStats.Lock()
		defer globalBlockchainStats.Unlock()

		globalBlockchainStats.CountFileRecords += header.Stats.CountFileRecords
		globalBlockchainStats.SizeAllFiles += header.Stats.SizeAllFiles
	})
}

// Called whenever a statistic changes for a blockchain in the global blockchain cache.
func filterGlobalBlockchainCacheStatistic(multi *blockchain.MultiStore, header *blockchain.MultiBlockchainHeader, statsOld blockchain.BlockchainStats) {
	globalBlockchainStats.Lock()
	defer globalBlockchainStats.Unlock()

	globalBlockchainStats.CountFileRecords += header.Stats.CountFileRecords - statsOld.CountFileRecords
	globalBlockchainStats.SizeAllFiles += header.Stats.SizeAllFiles - statsOld.SizeAllFiles
}

// Called after a blockchain is deleted from the global blockchain cache.
func filterGlobalBlockchainCacheDelete(multi *blockchain.MultiStore, header *blockchain.MultiBlockchainHeader) {
	globalBlockchainStats.Lock()
	defer globalBlockchainStats.Unlock()

	globalBlockchainStats.CountFileRecords -= header.Stats.CountFileRecords
	globalBlockchainStats.SizeAllFiles -= header.Stats.SizeAllFiles
}
