package transfer

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bivek7xdd/p2p-share/internal/webrtcs"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type SignalMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type Callbacks struct {
	OnConnected func()
	OnProgress  func(float64)
	OnSuccess   func()
	OnError     func(error)
}

func StartSender(ws *websocket.Conn, filePath string, cb Callbacks) {
	defer ws.Close()

	config := webrtcs.GetConfig()
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}
	defer peerConnection.Close()

	dataChannel, err := peerConnection.CreateDataChannel("file-transfer", nil)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}

	dataChannel.OnOpen(func() {
		if cb.OnConnected != nil {
			cb.OnConnected()
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if cb.OnError != nil {
				cb.OnError(err)
			}
			return
		}
		fileName := filepath.Base(filePath)
		fileSize := fileInfo.Size()

		metadata := map[string]interface{}{
			"type": "metadata",
			"name": fileName,
			"size": fileSize,
		}
		metaBytes, _ := json.Marshal(metadata)
		dataChannel.SendText(string(metaBytes))

		sendFileStream(dataChannel, filePath, fileSize, cb)
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON, _ := json.Marshal(c.ToJSON())
		sigMsg := SignalMessage{
			Event: "candidate",
			Data:  string(candidateJSON),
		}
		wsMsg, _ := json.Marshal(sigMsg)
		ws.WriteMessage(websocket.TextMessage, wsMsg)
	})

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}
	peerConnection.SetLocalDescription(offer)

	offerJSON, _ := json.Marshal(offer)
	sigMsg := SignalMessage{
		Event: "offer",
		Data:  string(offerJSON),
	}
	wsMsg, _ := json.Marshal(sigMsg)
	ws.WriteMessage(websocket.TextMessage, wsMsg)

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		var sig SignalMessage
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

func sendFileStream(dc *webrtc.DataChannel, filePath string, fileSize int64, cb Callbacks) {
	file, err := os.Open(filePath)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}
	defer file.Close()

	buffer := make([]byte, 16*1024)
	var sentBytes int64

	for {
		for dc.BufferedAmount() > 1024*1024 {
			time.Sleep(5 * time.Millisecond)
		}

		n, err := file.Read(buffer)
		if err == io.EOF {
			for dc.BufferedAmount() > 0 {
				time.Sleep(10 * time.Millisecond)
			}
			dc.SendText("EOF")
			time.Sleep(500 * time.Millisecond)
			dc.Close()
			if cb.OnProgress != nil {
				cb.OnProgress(1.0)
			}
			if cb.OnSuccess != nil {
				cb.OnSuccess()
			}
			return
		}
		if err != nil {
			if cb.OnError != nil {
				cb.OnError(err)
			}
			return
		}

		err = dc.Send(buffer[:n])
		if err != nil {
			if cb.OnError != nil {
				cb.OnError(err)
			}
			return
		}

		sentBytes += int64(n)
		if fileSize > 0 && cb.OnProgress != nil {
			cb.OnProgress(float64(sentBytes) / float64(fileSize))
		}
	}
}
