/*
File Name:  Statistics JSON.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"net/http"
	"time"

	"github.com/PeernetOfficial/core"
	"github.com/PeernetOfficial/core/webapi"
)

type jsonStatistics struct {
	Today jsonStatsDay   `json:"today"` // Current statistics of today
	Daily []jsonStatsDay `json:"daily"` // Daily records
}

type jsonStatsDay struct {
	Date        time.Time `json:"date"`        // Date
	Active      uint64    `json:"active"`      // Count of active peers
	Root        uint64    `json:"root"`        // Count of root peers
	NAT         uint64    `json:"nat"`         // Count of peers behind a NAT
	PortForward uint64    `json:"portforward"` // Count of peers with port forwarding enabled
	Firewall    uint64    `json:"firewall"`    // Count of peers reported behind a firewall
}

func webStatDailyJSON(backend *core.Backend) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var stats jsonStatistics

		for _, record := range summaryDaily {
			stats.Daily = append(stats.Daily, jsonStatsDay{Date: record.Date, Active: record.stats.countActive, Root: record.stats.countRoot, NAT: record.stats.countNAT, PortForward: record.stats.countPortForward, Firewall: record.stats.countFirewall})
		}

		stats.Today = jsonStatsDay{Date: time.Now().UTC(), Active: dailyStat.countActive, Root: dailyStat.countRoot, NAT: dailyStat.countNAT, PortForward: dailyStat.countPortForward, Firewall: dailyStat.countFirewall}

		CacheControlSetHeader(w, true, 60) // 1 minute
		webapi.EncodeJSON(backend, w, r, stats)
	}
}

type jsonStatsToday struct {
	Date time.Time `json:"date"` // Date
	// Peer Counts
	Active      uint64 `json:"active"`      // Count of active peers
	Root        uint64 `json:"root"`        // Count of root peers
	NAT         uint64 `json:"nat"`         // Count of peers behind a NAT
	PortForward uint64 `json:"portforward"` // Count of peers with port forwarding enabled
	Firewall    uint64 `json:"firewall"`    // Count of peers reported behind a firewall
	// File Statistics
	FilesShared uint64 `json:"filesshared"` // Count of files shared across all blockchains
	ContentSize uint64 `json:"contentsize"` // Total size of shared content in bytes across all blockchains
}

func webStatTodayJSON(backend *core.Backend) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := jsonStatsToday{Date: time.Now().UTC(), Active: dailyStat.countActive, Root: dailyStat.countRoot, NAT: dailyStat.countNAT, PortForward: dailyStat.countPortForward, Firewall: dailyStat.countFirewall}

		globalBlockchainStats.Lock()

		stats.FilesShared = globalBlockchainStats.CountFileRecords
		stats.ContentSize = globalBlockchainStats.SizeAllFiles

		globalBlockchainStats.Unlock()

		CacheControlSetHeader(w, true, 60) // 1 minute
		webapi.EncodeJSON(backend, w, r, stats)
	}
}
