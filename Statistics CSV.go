/*
File Name:  Statistics CSV.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner

CSV code for storing the statistics on disk.

Header of daily statistics files: Date, Peer ID, IPv4, IPv4 Port, IPv4 Reported Internal, IPv4 Reported External, IPv6, IPv6 Port, IPv6 Reported Internal, IPv6 Reported External, User Agent, Blockchain Height, Blockchain Version, Flags
* The peer ID is the public key in compressed form.
* The reported ports are self-reported by that peer and allow to detect NAT and port forwarding.
*/

package main

import (
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/PeernetOfficial/core/btcec"
)

// ---- daily summary file ----

var csvHeaderSummaryDaily = []string{"Date", "Daily Active Peers", "Root Peers", "NAT", "Port Forward", "Firewall"}

// recordSummaryDaily is a record in the summarty daily CSV file
type recordSummaryDaily struct {
	Date  time.Time
	stats timeStat
}

// statWriteSummary writes a summary file. It should be called at midnight.
func statWriteSummary(filename string, summary timeStat) {
	stats, err := os.Stat(filename)
	header := err != nil && os.IsNotExist(err) || err == nil && stats.Size() == 0

	// open the file for writing
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Error storing summary file '%s'. Active %d, root %d, NAT %d, port forward %d, firewall %d: %s", filename, summary.countActive, summary.countRoot, summary.countNAT, summary.countPortForward, summary.countFirewall, err.Error())
		return
	}
	defer file.Close()

	// create the CSV writer and write the header
	csvWriter := csv.NewWriter(file)
	csvWriter.UseCRLF = true

	if header {
		csvWriter.Write(csvHeaderSummaryDaily)
	}

	// write as CSV record
	todayA := time.Now().UTC().Round(time.Hour * 24).Format(dateFormat)

	csvWriter.Write([]string{todayA, strconv.FormatUint(summary.countActive, 10), strconv.FormatUint(summary.countRoot, 10), strconv.FormatUint(summary.countNAT, 10), strconv.FormatUint(summary.countPortForward, 10), strconv.FormatUint(summary.countFirewall, 10)})
	csvWriter.Flush()
}

// statReadSummary reads the full summary file.
func statReadSummary(filename string) (records []recordSummaryDaily, err error) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return records, err
	}

	csvReader := csv.NewReader(file)
	csvReader.LazyQuotes = true
	csvReader.Comma = ','
	csvReader.FieldsPerRecord = -1 // to allow rows with incorrect number of fields which will be skipped

	for {
		record, err := csvReader.Read()
		if err != nil {
			if err == csv.ErrFieldCount {
			} else if err == io.EOF {
				return records, nil
			} else {
				return records, err
			}
		}

		if len(record) != len(csvHeaderSummaryDaily) { // skip records with unexpected field count
			continue
		}

		var stat recordSummaryDaily

		// parse the fields
		if stat.Date, err = time.Parse(dateFormat, record[0]); err != nil {
			continue
		}
		if stat.stats.countActive, err = strconv.ParseUint(record[1], 10, 0); err != nil {
			continue
		}
		if stat.stats.countRoot, err = strconv.ParseUint(record[2], 10, 0); err != nil {
			continue
		}
		if stat.stats.countNAT, err = strconv.ParseUint(record[3], 10, 0); err != nil {
			continue
		}
		if stat.stats.countPortForward, err = strconv.ParseUint(record[4], 10, 0); err != nil {
			continue
		}
		if stat.stats.countFirewall, err = strconv.ParseUint(record[5], 10, 0); err != nil {
			continue
		}

		records = append(records, stat)
	}
}

// ---- full log ----

var csvHeaderFull = []string{"Date", "Peer ID", "Node ID", "IPv4", "IPv4 Port", "IPv4 Reported Internal", "IPv4 Reported External", "IPv6", "IPv6 Port", "IPv6 Reported Internal", "IPv6 Reported External", "User Agent", "Blockchain Height", "Blockchain Version", "Flags"}

var dailyLogMutex sync.Mutex

// createDailyLog creates the daily log file which contains records of all new peers.
// If the file already exists, it will read it to parse the peer IDs. This means that the serivce can be stopped and started anytime.
func createDailyLog(directory string, records <-chan *peerStat) (filename string, readStats timeStat, err error) {
	dailyLogMutex.Lock()
	defer dailyLogMutex.Unlock()

	today := time.Now().UTC()
	filename = path.Join(directory, fmt.Sprintf("%d_%02d_%02d.csv", today.Year(), today.Month(), today.Day()))

	stats, err := os.Stat(filename)

	if err == nil && stats.Size() > 0 {
		todayPeersMutex.Lock()

		// read existing file
		readDailyFile(filename, func(record []string) {
			if peerID, flags, err := parseDailyLogRecord(record); err == nil {
				todayPeers[peerID] = struct{}{}

				readStats.countActive++

				for _, char := range flags {
					switch char {
					case 'R':
						readStats.countRoot++
					case 'N':
						readStats.countNAT++
					case 'P':
						readStats.countPortForward++
					case 'F':
						readStats.countFirewall++
					}
				}
			}
		})

		todayPeersMutex.Unlock()
	}

	header := err != nil && os.IsNotExist(err) || err == nil && stats.Size() == 0

	// open the file for writing
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return filename, readStats, err
	}

	// create the CSV writer and write the header
	csvWriter := csv.NewWriter(file)
	csvWriter.UseCRLF = true

	if header {
		csvWriter.Write(csvHeaderFull)
		csvWriter.Flush()
	}

	go func() {
		for stat := range records {
			userAgent := stat.peer.UserAgent
			blockchainHeightA := strconv.FormatUint(uint64(stat.peer.BlockchainHeight), 10)
			blockchainVersionA := strconv.FormatUint(uint64(stat.peer.BlockchainVersion), 10)
			flags := stat.Flags()

			var ipv4A, ipv4PortA, ipv4ReportedInternalA, ipv4ReportedExternalA, ipv6A, ipv6PortA, ipv6ReportedInternalA, ipv6ReportedExternalA string
			if stat.connection4 != nil {
				ipv4A = stat.connection4.Address.IP.String()
				ipv4PortA = strconv.Itoa(stat.connection4.Address.Port)
				if stat.connection4.PortInternal > 0 {
					ipv4ReportedInternalA = strconv.Itoa(int(stat.connection4.PortInternal))
				}
				if stat.connection4.PortExternal > 0 {
					ipv4ReportedExternalA = strconv.Itoa(int(stat.connection4.PortExternal))
				}
			}
			if stat.connection6 != nil {
				ipv6A = stat.connection6.Address.IP.String()
				ipv6PortA = strconv.Itoa(stat.connection6.Address.Port)
				if stat.connection6.PortInternal > 0 {
					ipv6ReportedInternalA = strconv.Itoa(int(stat.connection6.PortInternal))
				}
				if stat.connection6.PortExternal > 0 {
					ipv6ReportedExternalA = strconv.Itoa(int(stat.connection6.PortExternal))
				}
			}

			csvWriter.Write([]string{stat.added.Format(dateFormat), hex.EncodeToString(stat.peerID[:]), hex.EncodeToString(stat.peer.NodeID), ipv4A, ipv4PortA, ipv4ReportedInternalA, ipv4ReportedExternalA, ipv6A, ipv6PortA, ipv6ReportedInternalA, ipv6ReportedExternalA, userAgent, blockchainHeightA, blockchainVersionA, flags})
			csvWriter.Flush()
		}
		file.Close()
	}()

	return filename, readStats, nil
}

func parseDailyLogRecord(record []string) (peerID [btcec.PubKeyBytesLenCompressed]byte, flags string, err error) {
	if len(record) != len(csvHeaderFull) { // skip records with unexpected field count
		return peerID, "", errors.New("invalid length")
	}

	peerIDh, err := hex.DecodeString(record[1])
	if err != nil || len(peerIDh) != btcec.PubKeyBytesLenCompressed {
		return peerID, "", errors.New("invalid peer ID")
	}

	copy(peerID[:], peerIDh)

	flags = record[14]

	return peerID, flags, nil
}

// readDailyFile reads the daily file and calls the callback with each record
func readDailyFile(filename string, callback func(record []string)) (err error) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}

	csvReader := csv.NewReader(file)
	csvReader.LazyQuotes = true
	csvReader.Comma = ','
	csvReader.FieldsPerRecord = -1 // to allow rows with incorrect number of fields which will be skipped

	for {
		record, err := csvReader.Read()
		if err != nil {
			if err == csv.ErrFieldCount {
			} else if err == io.EOF {
				return nil
			} else {
				return err
			}
		}

		if len(record) != len(csvHeaderFull) { // skip records with unexpected field count
			continue
		}

		callback(record)
	}
}

// ---- files served via web server ----

func webStatDailyActive(w http.ResponseWriter, r *http.Request) {
	CacheControlSetHeader(w, true, 10*60) // 10 minutes

	csvWriter := csv.NewWriter(w)
	csvWriter.UseCRLF = true

	csvWriter.Write(csvHeaderSummaryDaily)

	for _, record := range summaryDaily {
		csvWriter.Write([]string{record.Date.Format(dateFormat), strconv.FormatUint(record.stats.countActive, 10), strconv.FormatUint(record.stats.countRoot, 10), strconv.FormatUint(record.stats.countNAT, 10), strconv.FormatUint(record.stats.countPortForward, 10), strconv.FormatUint(record.stats.countFirewall, 10)})
	}

	csvWriter.Flush()
}
