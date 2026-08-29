package main

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"golang.design/x/hotkey"
)

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
			if string(msg) == "KILL_GTA" {
				g.appendLog("\n[!] REMOTE KILL SIGNAL RECEIVED!")
				go func() {
					for client := range g.clients {
						if client != conn {
							_ = client.WriteMessage(websocket.TextMessage, []byte("KILL_GTA"))
						}
					}
				}()

				g.executeKill()
			}
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
			_, msg, err := conn.ReadMessage()
			if err != nil {
				g.appendLog("[!] Lost connection to host. Attempting to reconnect in 3 seconds...")
				conn.Close()
				break
			}
			if string(msg) == "KILL_GTA" {
				g.appendLog("\n[!] REMOTE KILL SIGNAL RECEIVED!")
				g.executeKill()
			}
		}
	}
}

func onHotKeyPress(g *HeistGUI, hk *hotkey.Hotkey) {
	for range hk.Keydown() {
		g.appendLog("\n[!] HOTKEY DETECTED")

		g.executeKill()

		if g.isHost {
			for client := range g.clients {
				_ = client.WriteMessage(websocket.TextMessage, []byte("KILL_GTA"))
			}
		} else {
			if g.server != nil {
				_ = g.server.WriteMessage(websocket.TextMessage, []byte("KILL_GTA"))
			} else {
				g.appendLog("No server. Skipping remote command")
			}
		}
	}
}
