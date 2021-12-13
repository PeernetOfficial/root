/*
File Name:  API.go
Copyright:  2021 Peernet Foundation s.r.o.
Author:     Peter Kleissner
*/

package main

import (
	"bytes"
	"net/http"
	"time"

	"github.com/PeernetOfficial/core"
	"github.com/PeernetOfficial/core/webapi"
	"github.com/gorilla/websocket"
)

func startAPI(backend *core.Backend) {
	if len(config.APIListen) == 0 {
		return
	}

	api := webapi.Start(backend, config.APIListen, config.APIUseSSL, config.APICertificateFile, config.APICertificateKey, parseDuration(config.APITimeoutRead), parseDuration(config.APITimeoutWrite), config.APIKey)

	api.Router.HandleFunc("/console", apiConsole).Methods("GET")
}

/*
apiConsole provides a websocket to send/receive internal commands.

Request:    GET /console
Result:     Upgrade to websocket. The websocket message are texts to read/write.
*/
func apiConsole(w http.ResponseWriter, r *http.Request) {
	c, err := webapi.WSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// May happen if request is simple HTTP request.
		return
	}
	defer c.Close()

	bufferR := bytes.NewBuffer(make([]byte, 0, 4096))
	bufferW := bytes.NewBuffer(make([]byte, 0, 4096))

	terminateSignal := make(chan struct{})
	defer close(terminateSignal)

	// start userCommands which handles the actual commands
	go userCommands(bufferR, bufferW, terminateSignal)

	// go routine to receive output from userCommands and forward to websocket
	go func() {
		bufferW2 := make([]byte, 4096)
		for {
			select {
			case <-terminateSignal:
				return
			default:
			}

			countRead, err := bufferW.Read(bufferW2)
			if err != nil || countRead == 0 {
				time.Sleep(250 * time.Millisecond)
				continue
			}

			c.WriteMessage(websocket.TextMessage, bufferW2[:countRead])
		}
	}()

	// read from websocket loop and forward to the userCommands routine
	for {
		_, message, err := c.ReadMessage()
		if err != nil { // when channel is closed, an error is returned here
			break
		}

		// make sure the message has the \n delimiter which is used to detect a line
		if !bytes.HasSuffix(message, []byte{'\n'}) {
			message = append(message, '\n')
		}

		bufferR.Write(message)
	}
}
