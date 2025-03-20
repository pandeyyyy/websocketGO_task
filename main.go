package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// stores client info for multiple connections
type Client struct {
	ID           string
	Conn         *websocket.Conn
	LastPong     time.Time
	MessageQueue chan []byte
	Quit         chan bool
}

// manages all connected users
type Manager struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mutex      sync.Mutex
}

// upgrader for http to ws connection
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// initialize manager
var manager = Manager{
	clients:    make(map[string]*Client),
	register:   make(chan *Client),
	unregister: make(chan *Client),
	broadcast:  make(chan []byte),
}

// main loop for handling clients
func (manager *Manager) run() {
	for {
		select {
		case client := <-manager.register:
			manager.mutex.Lock()
			manager.clients[client.ID] = client
			manager.mutex.Unlock()
			log.Printf("New client: %s (total: %d)", client.ID, len(manager.clients))

		case client := <-manager.unregister:
			manager.mutex.Lock()
			if _, ok := manager.clients[client.ID]; ok {
				delete(manager.clients, client.ID)
				close(client.MessageQueue)
			}
			manager.mutex.Unlock()
			log.Printf("Client left: %s (total: %d)", client.ID, len(manager.clients))

		case msg := <-manager.broadcast:
			manager.mutex.Lock()
			for id, client := range manager.clients {
				select {
				case client.MessageQueue <- msg:
				default:
					close(client.MessageQueue)
					delete(manager.clients, id)
				}
			}
			manager.mutex.Unlock()
		}
	}
}

// check if clients are alive
func (manager *Manager) pingAll() {
	pingTimer := time.NewTicker(30 * time.Second)
	defer pingTimer.Stop()

	for range pingTimer.C {
		manager.mutex.Lock()
		for id, client := range manager.clients {
			if time.Since(client.LastPong) > 60*time.Second {
				log.Printf("Client %s timed out", id)
				client.Quit <- true
				continue
			}

			if err := client.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				log.Printf("Ping failed for %s: %v", id, err)
				client.Quit <- true
			}
		}
		manager.mutex.Unlock()
	}
}

// reads messages from client
func (client *Client) readMessages() {
	defer func() {
		manager.unregister <- client
		client.Conn.Close()
	}()

	client.Conn.SetReadLimit(4096)
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// set pong handler
	client.Conn.SetPongHandler(func(string) error {
		client.LastPong = time.Now()
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		// log.Printf("Received pong from client") commented out  because it was flooding terminal
		return nil
	})

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Read error from %s: %v", client.ID, err)
			}
			break
		}

		log.Printf("Client %s says: %s", client.ID, string(message))
		client.MessageQueue <- message
	}
}

// sends messages to client
func (client *Client) writeMessages() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.MessageQueue:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := client.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			n := len(client.MessageQueue)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-client.MessageQueue)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-client.Quit:
			return
		}
	}
}

// websocket endpoint
func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Connection upgrade failed:", err)
		return
	}

	client := &Client{
		ID:           uuid.New().String(),
		Conn:         conn,
		LastPong:     time.Now(),
		MessageQueue: make(chan []byte, 256),
		Quit:         make(chan bool),
	}

	manager.register <- client

	welcome := fmt.Sprintf("Welcome! Your ID is: %s", client.ID)
	client.MessageQueue <- []byte(welcome)

	go client.writeMessages()
	go client.readMessages()
}

// sends message to all clients
func sendToAll(message []byte) {
	manager.broadcast <- message
}

func main() {
	go manager.run()
	go manager.pingAll()

	http.HandleFunc("/ws", wsHandler)

	http.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			message := r.FormValue("message")
			if message != "" {
				sendToAll([]byte(message))
				w.Write([]byte("Message sent to all clients"))
				return
			}
			w.Write([]byte("Empty message"))
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	log.Println("WebSocket server running on ws://localhost:8080/ws")
	log.Println("Broadcast endpoint: http://localhost:8080/broadcast")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server error:", err)
	}
}
