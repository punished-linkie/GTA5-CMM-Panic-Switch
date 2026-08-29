package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"golang.design/x/hotkey"
)

type Message struct {
	Timestamp int64
	Content   string
}

func (g *HeistGUI) startServer() {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		g.mu.Lock()
		g.clients[conn] = true
		g.mu.Unlock()
		g.appendLog("[+] Crew member connected successfully!")

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				g.appendLog("[!] Lost connection to client.")
				conn.Close()
				break
			}

			message := Message{}
			_ = json.Unmarshal(msg, &message)

			if message.Content == "KILL_GTA" {
				g.appendLog("\n[!] REMOTE KILL SIGNAL RECEIVED!")
				go func() {
					for client := range g.clients {
						if client != conn {
							_ = client.WriteMessage(websocket.TextMessage, []byte(msg))
						}
					}
				}()

				g.executeKill()
			}
			stats.appendMessage(&message)
		}
	})
	_ = http.ListenAndServe(":8080", nil)
}

func (g *HeistGUI) startClient() {
	for {
		conn, _, err := websocket.DefaultDialer.Dial(g.Addr, nil)
		g.server = conn
		if err != nil {
			g.appendLog("[!] Connection failed. Retrying in 3s...")
			time.Sleep(3 * time.Second)
			continue
		}
		g.appendLog("[+] Connected to Host successfully over WAN!")

		for {
			message := Message{}
			_, msg, err := conn.ReadMessage()
			_ = json.Unmarshal(msg, &message)
			if err != nil {
				g.appendLog("[!] Lost connection to host. Attempting to reconnect in 3 seconds...")
				conn.Close()
				break
			}
			if message.Content == "KILL_GTA" {
				g.appendLog("\n[!] REMOTE KILL SIGNAL RECEIVED!")
				g.executeKill()
			}
			stats.appendMessage(&message)
		}
	}
}

func onHotKeyPress(g *HeistGUI, hk *hotkey.Hotkey) {
	for range hk.Keydown() {
		g.appendLog("\n[!] HOTKEY DETECTED")

		g.executeKill()

		kill_message, _ := json.Marshal(Message{
			Timestamp: time.Now().UnixMilli(),
			Content:   "KILL_GTA",
		})

		if g.isHost {
			for client := range g.clients {
				_ = client.WriteMessage(websocket.TextMessage, []byte(kill_message))
			}
		} else {
			if g.server != nil {
				_ = g.server.WriteMessage(websocket.TextMessage, []byte(kill_message))
			} else {
				g.appendLog("No server. Skipping remote command")
			}
		}
	}
}
