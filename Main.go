/*
File Name:  Main.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"fmt"
	"os"

	"github.com/PeernetOfficial/core"
	"github.com/google/uuid"
)

const configFile = "Config.yaml"
const appName = "Peernet Root"

var config struct {
	// Statistics web server settings
	WebListen       []string `yaml:"WebListen"`       // WebListen is in format IP:Port and declares where the web-interface should listen on. IP can also be ommitted to listen on any.
	UseSSL          bool     `yaml:"UseSSL"`          // Enables SSL.
	CertificateFile string   `yaml:"CertificateFile"` // This is the certificate received from the CA. This can also include the intermediate certificate from the CA.
	CertificateKey  string   `yaml:"CertificateKey"`  // This is the private key.
	HTTPAccessAllow string   `yaml:"HTTPAccessAllow"` // Sets the Access-Control-Allow-Origin HTTP header required for cross domain access. Specify * for all or URL.

	// HTTP Server Timeouts. Valid units are ms, s, m, h.
	HTTPTimeoutRead  string `yaml:"HTTPTimeoutRead"`  // The maximum duration for reading the entire request, including the body.
	HTTPTimeoutWrite string `yaml:"HTTPTimeoutWrite"` // The maximum duration before timing out writes of the response. This includes processing time and is therefore the max time any HTTP function may take.

	// WebFiles is the directory holding all HTML and other files to be served by the server
	WebFiles string `yaml:"WebFiles"`

	// DatabaseFolder defines where all the database files are stored. Currently they are uncompressed unencrypted CSV files.
	DatabaseFolder string `yaml:"DatabaseFolder"`

	// API settings
	APIListen          []string  `yaml:"APIListen"`          // WebListen is in format IP:Port and declares where the web-interface should listen on. IP can also be ommitted to listen on any.
	APIUseSSL          bool      `yaml:"APIUseSSL"`          // Enables SSL.
	APICertificateFile string    `yaml:"APICertificateFile"` // This is the certificate received from the CA. This can also include the intermediate certificate from the CA.
	APICertificateKey  string    `yaml:"APICertificateKey"`  // This is the private key.
	APITimeoutRead     string    `yaml:"APITimeoutRead"`     // The maximum duration for reading the entire request, including the body.
	APITimeoutWrite    string    `yaml:"APITimeoutWrite"`    // The maximum duration before timing out writes of the response. This includes processing time and is therefore the max time any HTTP function may take.
	APIKey             uuid.UUID `yaml:"APIKey"`             // API key. Empty UUID 00000000-0000-0000-0000-000000000000 = not used.
}

func main() {
	userAgent := appName + "/" + core.Version

	filters := &core.Filters{
		DHTSearchStatus:                filterSearchStatus,
		IncomingRequest:                filterIncomingRequest,
		MessageIn:                      filterMessageIn,
		MessageOutAnnouncement:         filterMessageOutAnnouncement,
		MessageOutResponse:             filterMessageOutResponse,
		MessageOutTraverse:             filterMessageOutTraverse,
		MessageOutPing:                 filterMessageOutPing,
		MessageOutPong:                 filterMessageOutPong,
		GlobalBlockchainCacheStatistic: filterGlobalBlockchainCacheStatistic,
		GlobalBlockchainCacheDelete:    filterGlobalBlockchainCacheDelete,
	}

	backend, status, err := core.Init(userAgent, configFile, filters, &config)

	if status != core.ExitSuccess {
		switch status {
		case core.ExitErrorConfigAccess:
			fmt.Printf("Unknown error accessing config file '%s': %s\n", configFile, err.Error())
		case core.ExitErrorConfigRead:
			fmt.Printf("Error reading config file '%s': %s\n", configFile, err.Error())
		case core.ExitErrorConfigParse:
			fmt.Printf("Error parsing config file '%s' (make sure it is valid YAML format): %s\n", configFile, err.Error())
		case core.ExitErrorLogInit:
			fmt.Printf("Error opening log file '%s': %s\n", backend.Config.LogFile, err.Error())
		default:
			fmt.Printf("Unknown error %d initializing backend: %s\n", status, err.Error())
		}
		os.Exit(status)
	}

	backend.Stdout.Subscribe(os.Stdout)

	initStatistics(backend)
	startStatisticsWebServer(backend)
	go startKPIs(backend)

	startAPI(backend)

	backend.Connect()

	userCommands(backend, os.Stdin, os.Stdout, nil)
}
