/*
File Name:  Statistics JSON.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
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
}

func webStatDailyJSON(w http.ResponseWriter, r *http.Request) {
	var stats jsonStatistics

	for _, record := range summaryDaily {
		stats.Daily = append(stats.Daily, jsonStatsDay{Date: record.Date, Active: record.stats.countActive, Root: record.stats.countRoot, NAT: record.stats.countNAT, PortForward: record.stats.countPortForward})
	}

	stats.Today = jsonStatsDay{Date: time.Now().UTC(), Active: dailyStat.countActive, Root: dailyStat.countRoot, NAT: dailyStat.countNAT, PortForward: dailyStat.countPortForward}

	CacheControlSetHeader(w, true, 60) // 1 minute
	APIEncodeJSON(w, r, stats)
}

// APIEncodeJSON writes JSON data
func APIEncodeJSON(w http.ResponseWriter, r *http.Request, data interface{}) (err error) {
	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Printf("Error writing data for route '%s': %v\n", r.URL.Path, err)
	}

	return err
}
