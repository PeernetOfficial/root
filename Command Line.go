/*
File Name:  Command Line.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PeernetOfficial/core"
	"github.com/PeernetOfficial/core/btcec"
	"github.com/PeernetOfficial/core/dht"
	"github.com/PeernetOfficial/core/protocol"
	"github.com/PeernetOfficial/core/webapi"
)

func showHelp(output io.Writer) {
	fmt.Fprint(output, "Please enter a command:\n"+
		"help                          Show this help\n"+
		"net list                      Lists all network adapters and their IPs\n"+
		"status                        Get current status\n"+
		"chat                          Send text to all peers\n"+
		"peer list                     List current peers\n"+
		"debug key create              Create Public-Private Key pair\n"+
		"debug key self                List current Public-Private Key pair\n"+
		"debug connect                 Attempts to connect to the target peer\n"+
		"debug watch searches          Watch all outgoing DHT searches\n"+
		"debug watch incoming          Watch all incoming information requests\n"+
		"debug watch                   Watch packets and info requests for hash\n"+
		"probe file transfer           Attempts to transfer and validate a remote file against a local file\n"+
		"hash                          Create blake3 hash of input\n"+
		"warehouse get                 Get data from local warehouse by hash\n"+
		"warehouse store               Store data into local warehouse\n"+
		"dht get                       Get data via DHT by hash\n"+
		"dht store                     Store data into DHT\n"+
		"log error                     Set error log output\n"+
		"\n")
}

func userCommands(input io.Reader, output io.Writer, terminateSignal chan struct{}) {
	reader := bufio.NewReader(input)

	fmt.Fprint(output, appName+" "+core.Version+"\n------------------------------\n")
	showHelp(output)

	for {
		command, _, terminate := getUserOptionString(reader, terminateSignal)
		if terminate {
			return
		}

		command = strings.ToLower(command)

		switch command {
		case "help", "?":
			showHelp(output)

		case "net list":
			fmt.Fprint(output, NetworkListOutput())

		case "debug key create":
			privateKey, publicKey, err := core.Secp256k1NewPrivateKey()
			if err != nil {
				fmt.Fprintf(output, "Error: %s\n", err.Error())
				return
			}

			fmt.Fprintf(output, "Private Key: %s\n", hex.EncodeToString(privateKey.Serialize()))
			fmt.Fprintf(output, "Public Key:  %s\n", hex.EncodeToString(publicKey.SerializeCompressed()))

		case "debug key self":
			privateKey, publicKey := core.ExportPrivateKey()
			fmt.Fprintf(output, "Private Key: %s\n", hex.EncodeToString(privateKey.Serialize()))
			fmt.Fprintf(output, "Public Key:  %s\n", hex.EncodeToString(publicKey.SerializeCompressed()))

		case "peer list":
			for _, peer := range GetPeerlistSorted() {
				info := ""
				if peer.IsRootPeer {
					info = " [root peer]"
				}
				if peer.IsBehindNAT() {
					info += " [NAT]"
				}
				userAgent := strings.ToValidUTF8(peer.UserAgent, "?")

				fmt.Fprintf(output, "* Peer ID %s%s\n  Node ID %s\n  User Agent: %s\n\n%s\n  Packets sent:      %d\n  Packets received:  %d\n\n", hex.EncodeToString(peer.PublicKey.SerializeCompressed()), info, hex.EncodeToString(peer.NodeID), userAgent, textPeerConnections(peer), peer.StatsPacketSent, peer.StatsPacketReceived)
			}

		case "chat all", "chat":
			if text, valid, terminate := getUserOptionString(reader, terminateSignal); valid {
				core.SendChatAll(text)
			} else if terminate {
				return
			}

		case "status":
			_, publicKey := core.ExportPrivateKey()
			nodeID := core.SelfNodeID()
			fmt.Fprintf(output, "----------------\nPublic Key: %s\nNode ID:    %s\n\n", hex.EncodeToString(publicKey.SerializeCompressed()), hex.EncodeToString(nodeID))

			features := ""
			featureSupport := core.FeatureSupport()
			if featureSupport&(1<<protocol.FeatureIPv4Listen) > 0 {
				features = "IPv4"
			}
			if featureSupport&(1<<protocol.FeatureIPv6Listen) > 0 {
				if len(features) > 0 {
					features += ", "
				}
				features += "IPv6"
			}
			if featureSupport&(1<<protocol.FeatureFirewall) > 0 {
				if len(features) > 0 {
					features += ", "
				}
				features += "Firewall Reported"
			}

			fmt.Fprintf(output, "User Agent: %s\nFeatures:   %s\n\n", core.SelfUserAgent(), features)

			fmt.Fprintf(output, "Listen Address                                  Multicast IP out                  External Address\n")

			for _, network := range core.GetNetworks(4) {
				address, _, broadcastIPv4, ipExternal, externalPort := network.GetListen()

				broadcastIPsA := ""
				for n, broadcastIP := range broadcastIPv4 {
					if n > 0 {
						broadcastIPsA += ", "
					}
					broadcastIPsA += broadcastIP.String()
				}

				externalAddress := ""

				if ipExternal != nil && !ipExternal.IsUnspecified() || externalPort > 0 {
					externalIPA := "[unknown]"
					externalPortA := ""
					if ipExternal != nil && !ipExternal.IsUnspecified() {
						externalIPA = ipExternal.String()
					}
					if externalPort > 0 {
						externalPortA = strconv.Itoa(int(externalPort))
					}

					externalAddress = net.JoinHostPort(externalIPA, externalPortA)
				}

				fmt.Fprintf(output, "%-46s  %-32s  %s\n", address.String(), broadcastIPsA, externalAddress)
			}
			for _, network := range core.GetNetworks(6) {
				address, multicastIP, _, _, externalPort := network.GetListen()

				externalPortA := ""
				if externalPort > 0 {
					externalPortA = strconv.Itoa(int(externalPort))
				}

				fmt.Fprintf(output, "%-46s  %-31s  %s\n", address.String(), multicastIP.String(), externalPortA)
			}

			fmt.Fprintf(output, "\nPeer ID                                                             Sent      Received  IP                                   Flags   RTT     \n")
			for _, peer := range GetPeerlistSorted() {
				addressA := "N/A"
				rttA := "N/A"
				if connectionsActive := peer.GetConnections(true); len(connectionsActive) > 0 {
					addressA = addressToA(connectionsActive[0].Address)
				}
				if rtt := peer.GetRTT(); rtt > 0 {
					rttA = rtt.Round(time.Millisecond).String()
				}
				flagsA := ""
				if peer.IsRootPeer {
					flagsA = "R"
				}
				if peer.IsBehindNAT() {
					flagsA += "N"
				}
				if peer.IsFirewallReported() {
					flagsA += "F"
				}
				fmt.Fprintf(output, "%-66s  %-8d  %-8d  %-35s  %-6s  %-6s\n", hex.EncodeToString(peer.PublicKey.SerializeCompressed()), peer.StatsPacketSent, peer.StatsPacketReceived, addressA, flagsA, rttA)
			}

			fmt.Fprintf(output, "\n")

		case "hash":
			if text, valid, terminate := getUserOptionString(reader, terminateSignal); valid {
				hash := core.Data2Hash([]byte(text))
				fmt.Fprintf(output, "blake3 hash: %s\n", hex.EncodeToString(hash))
			} else if terminate {
				return
			}

		case "warehouse get":
			if hash, valid, terminate := getUserOptionHash(reader, terminateSignal); valid {
				data, found := core.GetDataLocal(hash)
				if !found {
					fmt.Fprintf(output, "Not found.\n")
				} else {
					fmt.Fprintf(output, "Data hex:    %s\n", hex.EncodeToString(data))
					fmt.Fprintf(output, "Data string: %s\n", string(data))
				}
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid hash. Hex-encoded blake3 hash as input is required.\n")
			}

		case "warehouse store":
			if text, valid, terminate := getUserOptionString(reader, terminateSignal); valid {
				if err := core.StoreDataLocal([]byte(text)); err != nil {
					fmt.Fprintf(output, "Error storing data: %s\n", err.Error())
					break
				}
				fmt.Fprintf(output, "Stored via hash: %s\n", hex.EncodeToString(core.Data2Hash([]byte(text))))
			} else if terminate {
				return
			}

		case "dht store":
			if text, valid, terminate := getUserOptionString(reader, terminateSignal); valid {
				if err := core.StoreDataDHT([]byte(text), 5); err != nil {
					fmt.Fprintf(output, "Error storing data: %s\n", err.Error())
					break
				}
				fmt.Fprintf(output, "Stored via hash: %s\n", hex.EncodeToString(core.Data2Hash([]byte(text))))
			} else if terminate {
				return
			}

		case "dht get":
			if hash, valid, terminate := getUserOptionHash(reader, terminateSignal); valid {
				data, sender, found := core.GetDataDHT(hash)
				if !found {
					fmt.Fprintf(output, "Not found.\n")
				} else {
					fmt.Fprintf(output, "\nSender:      %s\n", hex.EncodeToString(sender))
					fmt.Fprintf(output, "Data hex:    %s\n", hex.EncodeToString(data))
					fmt.Fprintf(output, "Data string: %s\n", string(data))
				}
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid hash. Hex-encoded blake3 hash as input is required.\n")
			}

		case "log error":
			fmt.Fprintf(output, "Please choose the target output of error messages:\n0 = Log file (default)\n1 = Command line\n2 = Log file + command line\n3 = None\n")
			if number, valid, terminate := getUserOptionInt(reader, terminateSignal); valid && number >= 0 && number <= 3 {
				config.ErrorOutput = number
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid option.\n")
			}

		case "debug connect":
			fmt.Fprintf(output, "Please specify the target peer to connect to via DHT lookup, either by peer ID or node ID:\n")
			text, valid, terminate := getUserOptionString(reader, terminateSignal)
			if terminate {
				return
			} else if !valid || (len(text) != 66 && len(text) != 64) {
				fmt.Fprintf(output, "Invalid peer ID or node ID. It must be hex-encoded and 66 (peer ID) or 64 characters (node ID) long.\n")
				break
			}

			// node ID is required
			var nodeID []byte
			var err error

			if len(text) == 66 {
				// Assume peer ID was supplied.
				publicKeyB, err := hex.DecodeString(text)
				if err != nil || len(publicKeyB) != 33 {
					fmt.Fprintf(output, "Invalid peer ID encoding.\n")
					break
				}

				publicKey, err := btcec.ParsePubKey(publicKeyB, btcec.S256())
				if err != nil {
					fmt.Fprintf(output, "Invalid peer ID (public key decoding failed).\n")
					continue
				}

				nodeID = protocol.PublicKey2NodeID(publicKey)
			} else {
				// Node ID was supplied.
				if nodeID, err = hex.DecodeString(text); err != nil || len(nodeID) != 256/8 {
					fmt.Fprintf(output, "Invalid node ID encoding.\n")
					break
				}
			}

			// is self?
			if bytes.Equal(nodeID, core.SelfNodeID()) {
				fmt.Fprintf(output, "Target node is self.\n")
				break
			}

			debugCmdConnect(nodeID)

		case "debug watch searches":
			fmt.Fprintf(output, "Enable (1) or disable (0) watching of all outgoing DHT searches? (current setting: %t)\n", enableMonitorAll)
			if number, valid, terminate := getUserOptionInt(reader, terminateSignal); valid && number >= 0 && number <= 1 {
				enableMonitorAll = number == 1
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid option.\n")
			}

		case "debug watch incoming":
			fmt.Fprintf(output, "Enable (1) or disable (0) watching of all incoming information requests? (current setting: %t)\n", enableWatchIncomingAll)
			if number, valid, terminate := getUserOptionInt(reader, terminateSignal); valid && number >= 0 && number <= 1 {
				enableWatchIncomingAll = number == 1
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid option.\n")
			}

		case "debug bucket refresh":
			fmt.Fprintf(output, "Disable (1) or enable (0) bucket refresh. This can be useful to disable bucket refresh when debugging outgoing DHT searches. (current setting: %t)\n", dht.DisableBucketRefresh)
			if number, valid, terminate := getUserOptionInt(reader, terminateSignal); valid && number >= 0 && number <= 1 {
				dht.DisableBucketRefresh = number == 1
			} else if terminate {
				return
			} else {
				fmt.Fprintf(output, "Invalid option.\n")
			}

		case "debug watch":
			fmt.Fprintf(output, "Enter hash of data or node ID to watch. This monitors info requests and packets. Enter same hash again to remove from list.\n")
			text, _, terminate := getUserOptionString(reader, terminateSignal)
			if terminate {
				return
			}
			var hash []byte
			var err error
			if hash, err = hex.DecodeString(text); err != nil || len(hash) != 256/8 {
				fmt.Fprintf(output, "Invalid hash. Hex-encoded 64 character hash expected.\n")
				break
			}

			added := hashMonitorControl(hash, 2)
			if added {
				fmt.Fprintf(output, "The hash was added to the monitoring list.\n")
			} else {
				fmt.Fprintf(output, "The hash was removed from the monitoring list.\n")
			}

		case "probe file transfer":
			fmt.Fprintf(output, "Enter peer ID or node ID to connect:\n")
			nodeIDA, _, terminate := getUserOptionString(reader, terminateSignal)
			if terminate {
				return
			}
			fmt.Fprintf(output, "Enter file hash:\n")
			fileHashA, _, terminate := getUserOptionString(reader, terminateSignal)
			if terminate {
				return
			}

			fileHash, valid1 := webapi.DecodeBlake3Hash(fileHashA)
			nodeID, valid2 := webapi.DecodeBlake3Hash(nodeIDA)
			publicKey, err3 := core.PublicKeyFromPeerID(nodeIDA)

			if !valid2 && err3 != nil {
				fmt.Fprintf(output, "Invalid peer ID or node ID.\n")
				break
			} else if !valid1 {
				fmt.Fprintf(output, "Invalid file hash.\n")
			}

			var peer *core.PeerInfo
			var err error
			timeout := time.Second * 10

			if valid2 {
				peer, err = webapi.PeerConnectNode(nodeID, timeout)
			} else if err3 == nil {
				peer, err = webapi.PeerConnectPublicKey(publicKey, timeout)
			}
			if err != nil {
				fmt.Fprintf(output, "Could not connect to peer: %s\n", err.Error())
				break
			}

			go transferCompareFile(peer, fileHash)
		}
	}
}

// NetworkListOutput provides a user friendly output
func NetworkListOutput() (text string) {

	interfaceList, err := net.Interfaces()
	if err != nil {
		return "Error " + err.Error()
	}

	// iterate through all interfaces
	for _, ifaceSingle := range interfaceList {
		text += "Interface " + ifaceSingle.Name + ":\n"
		//text += "  MAC:        " + ifaceSingle.HardwareAddr.String() + "\n"

		addresses, err := ifaceSingle.Addrs()
		if err != nil {
			text += "  Error getting addresses: " + err.Error() + "\n\n"
			continue
		}

		// iterate through all IPs of the interfaces
		for _, address := range addresses {
			text += "  IP:         " + address.(*net.IPNet).IP.String() + "\n"
		}

		// Subscribed Multicast IPs of adapters are not really newsworthy
		//addresses2, err := ifaceSingle.MulticastAddrs()
		//if err != nil {
		//	text += "  Error getting multicast addresses: " + err.Error() + "\n\n"
		//	continue
		//}

		//for _, address := range addresses2 {
		//	text += "  Multicast:  " + address.(*net.IPAddr).IP.String() + "\n"
		//}

		text += "\n"
	}

	return text
}

const dateFormat = "2006-01-02 15:04:05"

func textPeerConnections(peer *core.PeerInfo) (text string) {
	connectionsActive := peer.GetConnections(true)
	connectionsInactive := peer.GetConnections(false)

	mapConnectionsA := make(map[string][]*core.Connection)
	mapConnectionsI := make(map[string][]*core.Connection)
	var listAdapters []string

	// for better human readability, sort all connections based on the network name
	for _, c := range connectionsActive {
		adapterName := c.Network.GetAdapterName()

		list, ok := mapConnectionsA[adapterName]
		if ok {
			mapConnectionsA[adapterName] = append(list, c)
		} else {
			mapConnectionsA[adapterName] = []*core.Connection{c}
			listAdapters = append(listAdapters, adapterName)
		}
	}

	for _, c := range connectionsInactive {
		adapterName := c.Network.GetAdapterName()

		_, ok1 := mapConnectionsA[adapterName]
		if !ok1 {
			if _, ok2 := mapConnectionsI[adapterName]; !ok2 {
				listAdapters = append(listAdapters, adapterName)
			}
		}

		list, ok := mapConnectionsI[adapterName]
		if ok {
			mapConnectionsI[adapterName] = append(list, c)
		} else {
			mapConnectionsI[adapterName] = []*core.Connection{c}
		}
	}

	sort.Strings(listAdapters)

	text += "  Status     Local                                               ->  Remote                                              Last Packet In       Last Packet Out      RTT     Ports I/E  \n"

	for _, adapterName := range listAdapters {
		text += "  -- adapter '" + adapterName + "' --\n"

		list, _ := mapConnectionsA[adapterName]
		for _, c := range list {
			listenAddress, _, _, _, _ := c.Network.GetListen()
			rttA := "N/A"
			if c.RoundTripTime > 0 {
				rttA = c.RoundTripTime.Round(time.Millisecond).String()
			}

			portEA := strconv.Itoa(int(c.PortInternal))
			if c.PortExternal > 0 {
				portEA += " / " + strconv.Itoa(int(c.PortExternal))
			}

			text += fmt.Sprintf("  %-9s  %-50s  ->  %-50s  %-19s  %-19s  %-6s  %-9s  \n", connectionStatusToA(c.Status), listenAddress.String(), addressToA(c.Address), c.LastPacketIn.Format(dateFormat), c.LastPacketOut.Format(dateFormat), rttA, portEA)
		}

		list, _ = mapConnectionsI[adapterName]
		for _, c := range list {
			listenAddress, _, _, _, _ := c.Network.GetListen()
			rttA := "N/A"
			if c.RoundTripTime > 0 {
				rttA = c.RoundTripTime.Round(time.Millisecond).String()
			}

			portEA := strconv.Itoa(int(c.PortInternal))
			if c.PortExternal > 0 {
				portEA += " / " + strconv.Itoa(int(c.PortExternal))
			}

			text += fmt.Sprintf("  %-9s  %-50s  ->  %-50s  %-19s  %-19s  %-6s  %-9s  \n", connectionStatusToA(c.Status), listenAddress.String(), addressToA(c.Address), c.LastPacketIn.Format(dateFormat), c.LastPacketOut.Format(dateFormat), rttA, portEA)
		}
	}

	return text
}

// addressToA is UDPAddr.String without IPv6 zone
func addressToA(a *net.UDPAddr) (result string) {
	if a == nil || len(a.IP) == 0 {
		return "<nil>"
	}
	return net.JoinHostPort(a.IP.String(), strconv.Itoa(a.Port))
}

// connectionStatusToA translates the connection status to a readable text
func connectionStatusToA(status int) (result string) {
	switch status {
	case core.ConnectionActive:
		return "active"
	case core.ConnectionInactive:
		return "inactive"
	case core.ConnectionRemoved:
		return "removed"
	case core.ConnectionRedundant:
		return "redundant"
	default:
		return "unknown"
	}
}

func GetPeerlistSorted() (peers []*core.PeerInfo) {
	peers = core.PeerlistGet()
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].IsRootPeer && !peers[j].IsRootPeer {
			return true
		} else if peers[j].IsRootPeer && !peers[i].IsRootPeer {
			return false
		}
		return (string(peers[i].NodeID) > string(peers[j].NodeID))
	})

	return peers
}

// logError handles error messages from core
func logError(function, format string, v ...interface{}) {
	switch config.ErrorOutput {
	case 0:
		core.DefaultLogError(function, format, v...)

	case 1:
		fmt.Printf("["+function+"] "+format, v...)

	case 2:
		core.DefaultLogError(function, format, v...)
		fmt.Printf("["+function+"] "+format, v...)
	}
}

// ---- command-line helper functions ----

// timeRetryUserInput defines how long the code waits for user input from reader before trying again
// The termination signal takes effect once the reader is drained and returns io.EOF.
const timeRetryUserInput = 500 * time.Millisecond

// readUserText reads user text from the buffer. Blocking, unless termination signal is raised!
func readUserText(reader *bufio.Reader, terminateSignal <-chan struct{}) (text string, valid, terminate bool) {
	for {
		if text, err := reader.ReadString('\n'); err == nil {
			return strings.TrimSpace(text), true, false
		}

		// check for termination signal
		select {
		case <-terminateSignal:
			return "", false, true
		default:
		}

		time.Sleep(timeRetryUserInput)
	}
}

func getUserOptionString(reader *bufio.Reader, terminateSignal <-chan struct{}) (response string, valid, terminate bool) {
	return readUserText(reader, terminateSignal)
}

func getUserOptionBool(reader *bufio.Reader, terminateSignal <-chan struct{}) (response bool, valid, terminate bool) {
	responseA, valid, terminate := readUserText(reader, terminateSignal)
	if !valid || terminate {
		return false, valid, terminate
	}

	responseI, err := strconv.Atoi(responseA)
	if err != nil || (responseI != 0 && responseI != 1) {
		return false, false, false
	}

	return responseI == 1, true, false
}

func getUserOptionInt(reader *bufio.Reader, terminateSignal <-chan struct{}) (response int, valid, terminate bool) {
	responseA, valid, terminate := readUserText(reader, terminateSignal)
	if !valid || terminate {
		return 0, valid, terminate
	}

	responseI, err := strconv.Atoi(responseA)
	if err != nil {
		return 0, false, false
	}

	return responseI, true, false
}

func getUserOptionHash(reader *bufio.Reader, terminateSignal <-chan struct{}) (hash []byte, valid, terminate bool) {
	responseA, valid, terminate := readUserText(reader, terminateSignal)
	if !valid || terminate {
		return nil, valid, terminate
	}

	hash, err := hex.DecodeString(responseA)
	if err != nil || len(hash) != 256/8 {
		return nil, false, false
	}

	return hash, true, false
}
