package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/bivek7xdd/p2p-share/internal/webrtcs"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/schollz/progressbar/v3"
)

type SignalMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func main() {
	//1. Get PIN from command line
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/receive/main.go <PIN>")
		os.Exit(1)
	}
	pin := os.Args[1]

	//2.  Connect to signaling server
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		fmt.Println("Error connecting to signaling server:", err)
		os.Exit(1)
	}
	defer ws.Close()

	ws.WriteMessage(websocket.TextMessage, []byte("RECEIVE"))
	ws.WriteMessage(websocket.TextMessage, []byte(pin))

	//3. set up webRtc using our shared configuration
	config := webrtcs.GetConfig()
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		fmt.Println("Error creating peer connection:", err)
		os.Exit(1)
	}
	defer peerConnection.Close()

	// Create variables to hold our file pointer AND our progress bar
	var file *os.File
	var bar *progressbar.ProgressBar

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Println("✅ Peer connected! Waiting for file info...")

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			// A. Is it a text message (Metadata or EOF)?
			if msg.IsString {
				text := string(msg.Data)

				if text == "EOF" {
					if file != nil {
						file.Close()
					}
					fmt.Println("\n🚀 Download Complete!")
					os.Exit(0)
				}

				// Parse the JSON metadata to get name AND size
				var meta map[string]interface{}
				if err := json.Unmarshal(msg.Data, &meta); err == nil && meta["type"] == "metadata" {
					fileName := meta["name"].(string)

					// JSON numbers are parsed as float64 by default in Go
					fileSize := int64(meta["size"].(float64))

					// Initialize the beautiful progress bar!
					bar = progressbar.DefaultBytes(
						fileSize,
						"Downloading "+fileName,
					)

					// Create the file in the current working directory
					file, err = os.Create(fileName)
					if err != nil {
						log.Fatal("Could not create file:", err)
					}
				}
				return
			}

			// B. It's binary data! Write it to disk and update the progress bar.
			if file != nil {
				bytesWritten, err := file.Write(msg.Data)
				if err != nil {
					log.Fatal("Error writing to file:", err)
				}

				// Tell the progress bar how many bytes we just saved
				bar.Add(bytesWritten)
			}
		})
	})
	//5. Handle out ICE candidates and send them to the server
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON, _ := json.Marshal(c.ToJSON())
		sendWS(ws, "candidate", string(candidateJSON))
	})
	//6. Main loop: listen for the sender's offer and candidates
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			log.Fatal("Error reading from WebSocket:", err)
		}
		var sig SignalMessage
		json.Unmarshal(msg, &sig)

		switch sig.Event {
		case "offer":
			var offer webrtc.SessionDescription
			json.Unmarshal([]byte(sig.Data), &offer)
			peerConnection.SetRemoteDescription(offer)

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Fatal("Error creating answer:", err)
			}
			peerConnection.SetLocalDescription(answer)

			answerJSON, _ := json.Marshal(answer)
			sendWS(ws, "answer", string(answerJSON))
		case "candidate":
			var candidate webrtc.ICECandidateInit
			json.Unmarshal([]byte(sig.Data), &candidate)
			peerConnection.AddICECandidate(candidate)
		}
	}

}
func sendWS(ws *websocket.Conn, event string, data string) {
	msg, _ := json.Marshal(SignalMessage{Event: event, Data: data})
	ws.WriteMessage(websocket.TextMessage, msg)
}
