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

	// Log settings
	ErrorOutput int `yaml:"ErrorOutput"` // 0 = Log file (default),  1 = Command line, 2 = Log file + command line, 3 = None
}

func init() {
	if status, err := core.LoadConfigOut(configFile, &config); err != nil {
		switch status {
		case 0:
			fmt.Printf("Unknown error accessing config file '%s': %s\n", configFile, err.Error())
		case 1:
			fmt.Printf("Error reading config file '%s': %s\n", configFile, err.Error())
		case 2:
			fmt.Printf("Error parsing config file '%s' (make sure it is valid YAML format): %s\n", configFile, err.Error())
		case 3:
			fmt.Printf("Unknown error loading config file '%s': %s\n", configFile, err.Error())
		}
		os.Exit(1)
	}

	if err := core.InitLog(); err != nil {
		fmt.Printf("Error opening log file: %s\n", err.Error())
		os.Exit(1)
	}

	monitorKeys = make(map[string]struct{})

	core.UserAgent = appName + "/" + core.Version
	core.Filters.LogError = logError
	core.Filters.DHTSearchStatus = filterSearchStatus
	core.Filters.IncomingRequest = filterIncomingRequest

	core.Init()
}

func main() {
	initStatistics()
	startStatisticsWebServer()

	core.Connect()

	userCommands()
}
