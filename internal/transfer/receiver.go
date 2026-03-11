package transfer

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bivek7xdd/p2p-share/internal/webrtcs"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

func StartReceiver(pin string, serverURL string, cb Callbacks) {
	ws, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}
	defer ws.Close()

	ws.WriteMessage(websocket.TextMessage, []byte("RECEIVE"))
	ws.WriteMessage(websocket.TextMessage, []byte(pin))

	config := webrtcs.GetConfig()
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		if cb.OnError != nil {
			cb.OnError(err)
		}
		return
	}
	defer peerConnection.Close()

	var file *os.File
	var fileSize int64
	var receivedBytes int64

	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		if cb.OnConnected != nil {
			cb.OnConnected()
		}

		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			if msg.IsString {
				text := string(msg.Data)
				if text == "EOF" {
					if file != nil {
						file.Close()
					}
					if cb.OnProgress != nil {
						cb.OnProgress(1.0)
					}
					if cb.OnSuccess != nil {
						cb.OnSuccess()
					}
					return
				}

				var meta map[string]interface{}
				if err := json.Unmarshal(msg.Data, &meta); err == nil && meta["type"] == "metadata" {
					fileName := meta["name"].(string)
					fileSize = int64(meta["size"].(float64))

					file, err = os.Create(fileName)
					if err != nil && cb.OnError != nil {
						cb.OnError(err)
					}
				}
				return
			}

			if file != nil {
				n, err := file.Write(msg.Data)
				if err != nil {
					if cb.OnError != nil {
						cb.OnError(err)
					}
					return
				}
				receivedBytes += int64(n)
				if fileSize > 0 && cb.OnProgress != nil {
					cb.OnProgress(float64(receivedBytes) / float64(fileSize))
				}
			}
		})
	})

	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidateJSON, _ := json.Marshal(c.ToJSON())
		msg, _ := json.Marshal(SignalMessage{Event: "candidate", Data: string(candidateJSON)})
		ws.WriteMessage(websocket.TextMessage, msg)
	})

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if cb.OnError != nil {
				cb.OnError(fmt.Errorf("Signaling server disconnected or error: %v", err))
			}
			break
		}

		// Check if it's the specific "invalid pin" string from the server
		if string(msg) == "invalid pin" {
			if cb.OnError != nil {
				cb.OnError(fmt.Errorf("Invalid PIN code entered!"))
			}
			break
		}
		if string(msg) == "connected to sender" {
			continue // ignore plain text from server
		}

		var sig SignalMessage
		json.Unmarshal(msg, &sig)

		switch sig.Event {
		case "offer":
			var offer webrtc.SessionDescription
			json.Unmarshal([]byte(sig.Data), &offer)
			peerConnection.SetRemoteDescription(offer)

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil && cb.OnError != nil {
				cb.OnError(err)
			}
			peerConnection.SetLocalDescription(answer)

			answerJSON, _ := json.Marshal(answer)
			msg, _ := json.Marshal(SignalMessage{Event: "answer", Data: string(answerJSON)})
			ws.WriteMessage(websocket.TextMessage, msg)
		case "candidate":
			var candidate webrtc.ICECandidateInit
			json.Unmarshal([]byte(sig.Data), &candidate)
			peerConnection.AddICECandidate(candidate)
		}
	}
}
