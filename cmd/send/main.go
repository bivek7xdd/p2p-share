package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bivek7xdd/p2p-share/internal/webrtcs"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/schollz/progressbar/v3"
)

type SignalMeassage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/send/main.go <file-to-send>")
		os.Exit(1)
	}
	filePath := os.Args[1]

	// Create a new RTCPeerConnection

	//1. Connect to signaling server and get pin
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatal("Error connecting to signaling server:", err)
	}
	defer ws.Close()

	// Tell server we want to send
	ws.WriteMessage(websocket.TextMessage, []byte("SEND"))

	// Wait for pin
	_, msg, err := ws.ReadMessage()
	if err != nil {
		log.Fatal("Error reading pin from server:", err)
	}
	response := string(msg)
	if strings.HasPrefix(response, "PIN:") {
		fmt.Printf("🎯 Ready! Tell the receiver to use PIN: %s\n", strings.Split(response, ":")[1])
	}

	//2. set up webRTC
	config := webrtcs.GetConfig()
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Fatal(err)
	}
	defer peerConnection.Close()

	//3. Create a data channel for file transfer
	dataChannel, err := peerConnection.CreateDataChannel("file-transfer", nil)
	if err != nil {
		log.Fatal(err)
	}

	//4. Handle incoming WebRTC signals from the server
	dataChannel.OnOpen(func() {
		fmt.Println("✅ Peer connected! Sending metadata...")

		// 1. Get the file size and name
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			log.Fatal("Could not get file info:", err)
		}
		fileName := filepath.Base(filePath)
		fileSize := fileInfo.Size()

		// 2. Send the name AND size in the metadata
		metadata := map[string]interface{}{
			"type": "metadata",
			"name": fileName,
			"size": fileSize,
		}
		metaBytes, _ := json.Marshal(metadata)
		dataChannel.SendText(string(metaBytes))

		fmt.Println("🚀 Sending file data...")
		sendFile(dataChannel, filePath)
	})

	dataChannel.OnClose(func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	})

	// 5. Handle ICE Candidates (Finding the best network path)
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON, _ := json.Marshal(c.ToJSON())
		sigMsg := SignalMeassage{
			Event: "candidate",
			Data:  string(candidateJSON),
		}
		wsMsg, _ := json.Marshal(sigMsg)
		ws.WriteMessage(websocket.TextMessage, wsMsg)
	})

	//6. Create the webRTC offer and send it to the server
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatal(err)
	}
	peerConnection.SetLocalDescription(offer)

	offerJSON, _ := json.Marshal(offer)
	sigMsg := SignalMeassage{
		Event: "offer",
		Data:  string(offerJSON),
	}
	wsMsg, _ := json.Marshal(sigMsg)
	ws.WriteMessage(websocket.TextMessage, wsMsg)

	//7. Listen for the receiver's answer through the websocket
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			log.Fatal("Error reading message from server:", err)
		}
		var sig SignalMeassage
		json.Unmarshal(msg, &sig)

		switch sig.Event {
		case "answer":
			var answer webrtc.SessionDescription
			json.Unmarshal([]byte(sig.Data), &answer)
			peerConnection.SetRemoteDescription(answer)

		case "candidate":
			var candidate webrtc.ICECandidateInit
			json.Unmarshal([]byte(sig.Data), &candidate)
			peerConnection.AddICECandidate(candidate)
		}

	}
}

// sendFile handles slicing the file into chuncks so we dont crash the ram
func sendFile(dc *webrtc.DataChannel, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("Error getting file info:", err)
		return
	}

	bar := progressbar.DefaultBytes(
		fileInfo.Size(),
		"Sending "+filepath.Base(filePath),
	)

	//define chunk size (16KB chunks are safer and more optimal for WebRTC)
	buffer := make([]byte, 16*1024)

	for {
		// Backpressure: wait until the buffer drains
		// WebRTC default buffer can handle a bit more, but 1MB is safe.
		for dc.BufferedAmount() > 1024*1024 {
			time.Sleep(5 * time.Millisecond)
		}

		n, err := file.Read(buffer)
		if err == io.EOF {
			// Wait for buffer to drain before sending EOF
			for dc.BufferedAmount() > 0 {
				time.Sleep(10 * time.Millisecond)
			}

			//Tell receiver we are done
			dc.SendText("EOF")

			// The receiver exits abruptly immediately after getting EOF.
			// Give it a brief moment to transmit over the network before closing.
			time.Sleep(500 * time.Millisecond)

			fmt.Println("\n✅ File sent successfully!")

			// Close the data channel
			dc.Close()

			// Force exit after successful send since OnClose may not reliably fire
			// when the receiver disconnects abruptly.
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)
			return
		}
		if err != nil {
			log.Fatal(err)
		}

		err = dc.Send(buffer[:n])
		if err != nil {
			log.Fatal("Send error: ", err)
		}

		bar.Add(n)
	}
}
