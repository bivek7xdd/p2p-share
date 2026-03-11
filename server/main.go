package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	rooms    = make(map[string]*Room)
	roomsMu  sync.Mutex
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type Room struct {
	Sender   *websocket.Conn
	Receiver *websocket.Conn
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "p2p-share signaling server is running")
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	http.HandleFunc("/ws", handleConnections)

	fmt.Printf("Server started on :%s\n", port)
	http.ListenAndServe(":"+port, nil)
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, _ := upgrader.Upgrade(w, r, nil)

	//check if they are send or receive
	_, msg, _ := conn.ReadMessage()
	//TODO: need to handle error

	action := string(msg)
	switch action {
	case "SEND":
		//create a new room
		pin := fmt.Sprintf("%04d", rand.Intn(10000))
		roomsMu.Lock()
		rooms[pin] = &Room{Sender: conn}
		roomsMu.Unlock()

		conn.WriteMessage(websocket.TextMessage, []byte("PIN:"+pin))
		fmt.Printf("New sender connected with pin: %s\n", pin)

	case "RECEIVE":
		//wait for pin
		_, pinMsg, _ := conn.ReadMessage()
		pin := string(pinMsg)

		roomsMu.Lock()
		room, exists := rooms[pin]
		if exists {
			room.Receiver = conn
			conn.WriteMessage(websocket.TextMessage, []byte("connected to sender"))
			go startSignaling(room)
		} else {
			conn.WriteMessage(websocket.TextMessage, []byte("invalid pin"))
			conn.Close()
		}
		roomsMu.Unlock()

	default:
		conn.WriteMessage(websocket.TextMessage, []byte("unknown action"))
		conn.Close()
	}
}

func startSignaling(room *Room) {
	// bridge the two connections
	go bridge(room.Sender, room.Receiver)
	bridge(room.Receiver, room.Sender)
}

func bridge(from, to *websocket.Conn) {
	for {
		mt, msg, err := from.ReadMessage()
		if err != nil {
			break
		}
		to.WriteMessage(mt, msg)
	}
}
