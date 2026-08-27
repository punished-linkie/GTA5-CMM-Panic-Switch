package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gorilla/websocket"
	"golang.design/x/hotkey"
)

type customTheme struct {
	fyne.Theme
}

func (m *customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameDisabled {
		return color.NRGBA{R: 240, G: 240, B: 240, A: 255}
	}
	if name == theme.ColorNameBackground || name == theme.ColorNameInputBackground {
		return color.NRGBA{R: 25, G: 25, B: 30, A: 255}
	}
	return m.Theme.Color(name, variant)
}

type HeistGUI struct {
	isHost      bool
	Addr        string
	DuckDomain  string
	DuckToken   string
	server      *websocket.Conn
	clients     map[*websocket.Conn]bool
	mu          sync.Mutex
	logArea     *widget.Entry
	isRecording bool
}

const settings_file = "cmm.json"

func main() {

	if _, err := os.Stat(settings_file); os.IsNotExist(err) {
		_ = os.WriteFile(settings_file,
			[]byte(`{
        "addr": "ws://myduckdnsssubdomain.duckdns.org:8080/ws",
        "duckDomain": "myduckdnsssubdomain",
        "duckToken": "my-duck-dns-subdomain-token"
    }`), 0644)
		fmt.Printf("Created %s with default config\n", settings_file)
	} else {
		fmt.Printf("%s already exists\n", settings_file)
	}

	myApp := app.New()
	myApp.Settings().SetTheme(&customTheme{Theme: theme.DefaultTheme()})

	myWindow := myApp.NewWindow("GTA V CMM Panic Switch")
	myWindow.Resize(fyne.NewSize(500, 600))

	g := &HeistGUI{
		clients: make(map[*websocket.Conn]bool),
	}

	{
		settings, errr := os.ReadFile(settings_file)
		if errr != nil {
			println("Error reading config file", errr)
		} else {
			_ = json.Unmarshal(settings, &g)
		}
	}

	addrInput := widget.NewEntry()
	addrInput.SetText(g.Addr)

	domainInput := widget.NewEntry()
	domainInput.SetText(g.DuckDomain)

	tokenInput := widget.NewPasswordEntry()
	tokenInput.SetText(g.DuckToken)

	clientContainer := container.NewVBox(
		widget.NewLabel("Client: Host Address (ws://...):"),
		addrInput,
	)

	hostContainer := container.NewVBox(
		widget.NewLabel("Host Only: DuckDNS Subdomain & Token:"),
		container.NewGridWithColumns(2, domainInput, tokenInput),
	)

	hostContainer.Hide()

	roleSelect := widget.NewSelect([]string{"Client (Join Crew)", "Host (Server Leader)"}, func(s string) {
		g.isHost = (s == "Host (Server Leader)")
		if g.isHost {
			clientContainer.Hide()
			hostContainer.Show()
		} else {
			hostContainer.Hide()
			clientContainer.Show()
		}
	})
	roleSelect.SetSelected("Client (Join Crew)")

	var pending *binding // last recorded combination, awaiting registration
	var current *hotkey.Hotkey

	status := widget.NewLabel("Record a shortcut, then click Register.")
	recorder := newShortcutRecorder(func(b binding) {
		pending = &b
		status.SetText("Recorded " + b.display + ". Click Register to activate it.")
	})
	recorder.canvasRef = myWindow.Canvas()

	myWindow.SetOnClosed(func() {
		if current != nil {
			_ = current.Unregister()
		}
	})

	registerBtn := widget.NewButton("Register", func() {
		// TODO
		if pending == nil {
			status.SetText("Record a shortcut first (click the box above and press keys).")
			return
		}

		// Replace any previously registered hotkey. Unregistering closes its
		// Keydown channel, so the old listener goroutine exits on its own.
		if current != nil {
			_ = current.Unregister()
			current = nil
		}

		hk := hotkey.New(pending.mods, pending.key)
		if err := hk.Register(); err != nil {
			status.SetText("Failed to register " + pending.display + ": " + err.Error())
			return
		}
		current = hk
		display := pending.display
		status.SetText("Registered " + display + ". Press it anywhere to trigger.")
		g.appendLog("[i] Registered shortcut: " + display)

		go func() {
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
		}()
	})

	g.logArea = widget.NewMultiLineEntry()
	g.logArea.SetText("[i] Ready. Configure your settings and click Initialize.")
	g.logArea.Disable()

	initBtn := widget.NewButton("Initialize Network", func() {
		g.Addr = addrInput.Text
		g.DuckDomain = domainInput.Text
		g.DuckToken = tokenInput.Text

		if g.isHost {
			go g.startServer()
			go g.startDuckDNSUpdater()
			g.appendLog("[+] Host server & IPv6 DuckDNS auto-updater initialized...")
		} else {
			go g.startClient()
			g.appendLog(fmt.Sprintf("[*] Connecting to host at %s...", g.Addr))
		}

		{
			config, _ := json.MarshalIndent(g, "    ", "")
			_ = os.WriteFile(settings_file, config, 0644)
		}
	})
	initBtn.Importance = widget.HighImportance

	topContainer := container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabel("Operating Mode:"),
		roleSelect,
		clientContainer,
		hostContainer,
		widget.NewLabel("Panic Shortcut Combo:"),
		container.NewGridWithColumns(2, recorder, registerBtn),
		initBtn,
		widget.NewSeparator(),
		widget.NewLabel("Live Status Log:"),
	)

	content := container.NewBorder(topContainer, nil, nil, nil, g.logArea)
	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func (g *HeistGUI) appendLog(msg string) {
	fyne.Do(func() {
		println("[" + msg + "]")
		current := g.logArea.Text
		g.logArea.SetText(current + "\n" + msg)
		g.logArea.CursorRow = len(g.logArea.Text)
	})
}

func (g *HeistGUI) startDuckDNSUpdater() {
	for {
		if g.DuckDomain != "" && g.DuckToken != "" && g.DuckDomain != "yourname" {
			ipResp, err := http.Get("https://api6.ipify.org")
			if err != nil {
				time.Sleep(5 * time.Minute)
				continue
			}
			ipv6Bytes, _ := io.ReadAll(ipResp.Body)
			ipResp.Body.Close()
			myIPv6 := strings.TrimSpace(string(ipv6Bytes))

			url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&ipv6=%s&verbose=true", g.DuckDomain, g.DuckToken, myIPv6)
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
			}
		}
		time.Sleep(5 * time.Minute)
	}
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

func (g *HeistGUI) executeKill() {
	cmd := exec.Command("taskkill", "/F", "/IM", "GTA5_Enhanced.exe")
	if err := cmd.Run(); err != nil {
		g.appendLog(fmt.Sprintf("[X] Failed to kill process: %v", err))
	} else {
		g.appendLog("[✓] GTA5 process terminated instantly!")
	}
}

// ---- Key/modifier mapping between Fyne and the hotkey library ----
type modInfo struct {
	mod   hotkey.Modifier
	label string
	order int
}

// modifierKeys maps Fyne modifier key names to hotkey modifiers.
var modifierKeys = map[fyne.KeyName]modInfo{
	desktop.KeyControlLeft:  {hotkey.ModCtrl, "Ctrl", 0},
	desktop.KeyControlRight: {hotkey.ModCtrl, "Ctrl", 0},
	desktop.KeyShiftLeft:    {hotkey.ModShift, "Shift", 1},
	desktop.KeyShiftRight:   {hotkey.ModShift, "Shift", 1},
	desktop.KeyAltLeft:      {hotkey.ModAlt, "Alt", 2},
	desktop.KeyAltRight:     {hotkey.ModAlt, "Alt", 2},
	desktop.KeySuperLeft:    {hotkey.ModWin, "Win", 3},
	desktop.KeySuperRight:   {hotkey.ModWin, "Win", 3},
}

// mainKeys maps Fyne (non-modifier) key names to hotkey keys.
var mainKeys = map[fyne.KeyName]hotkey.Key{
	fyne.KeyA: hotkey.KeyA, fyne.KeyB: hotkey.KeyB, fyne.KeyC: hotkey.KeyC,
	fyne.KeyD: hotkey.KeyD, fyne.KeyE: hotkey.KeyE, fyne.KeyF: hotkey.KeyF,
	fyne.KeyG: hotkey.KeyG, fyne.KeyH: hotkey.KeyH, fyne.KeyI: hotkey.KeyI,
	fyne.KeyJ: hotkey.KeyJ, fyne.KeyK: hotkey.KeyK, fyne.KeyL: hotkey.KeyL,
	fyne.KeyM: hotkey.KeyM, fyne.KeyN: hotkey.KeyN, fyne.KeyO: hotkey.KeyO,
	fyne.KeyP: hotkey.KeyP, fyne.KeyQ: hotkey.KeyQ, fyne.KeyR: hotkey.KeyR,
	fyne.KeyS: hotkey.KeyS, fyne.KeyT: hotkey.KeyT, fyne.KeyU: hotkey.KeyU,
	fyne.KeyV: hotkey.KeyV, fyne.KeyW: hotkey.KeyW, fyne.KeyX: hotkey.KeyX,
	fyne.KeyY: hotkey.KeyY, fyne.KeyZ: hotkey.KeyZ,

	fyne.Key0: hotkey.Key0, fyne.Key1: hotkey.Key1, fyne.Key2: hotkey.Key2,
	fyne.Key3: hotkey.Key3, fyne.Key4: hotkey.Key4, fyne.Key5: hotkey.Key5,
	fyne.Key6: hotkey.Key6, fyne.Key7: hotkey.Key7, fyne.Key8: hotkey.Key8,
	fyne.Key9: hotkey.Key9,

	fyne.KeySpace:  hotkey.KeySpace,
	fyne.KeyReturn: hotkey.KeyReturn,
	fyne.KeyEnter:  hotkey.KeyReturn,
	fyne.KeyEscape: hotkey.KeyEscape,
	fyne.KeyTab:    hotkey.KeyTab,
	fyne.KeyDelete: hotkey.KeyDelete,

	fyne.KeyUp: hotkey.KeyUp, fyne.KeyDown: hotkey.KeyDown,
	fyne.KeyLeft: hotkey.KeyLeft, fyne.KeyRight: hotkey.KeyRight,

	fyne.KeyF1: hotkey.KeyF1, fyne.KeyF2: hotkey.KeyF2, fyne.KeyF3: hotkey.KeyF3,
	fyne.KeyF4: hotkey.KeyF4, fyne.KeyF5: hotkey.KeyF5, fyne.KeyF6: hotkey.KeyF6,
	fyne.KeyF7: hotkey.KeyF7, fyne.KeyF8: hotkey.KeyF8, fyne.KeyF9: hotkey.KeyF9,
	fyne.KeyF10: hotkey.KeyF10, fyne.KeyF11: hotkey.KeyF11, fyne.KeyF12: hotkey.KeyF12,
}

// binding is a fully captured shortcut ready to hand to the hotkey library.
type binding struct {
	mods    []hotkey.Modifier
	key     hotkey.Key
	display string
}

// ---- Shortcut recorder widget ----

// shortcutRecorder is a focusable widget that records the next key combination
// (any number of modifiers plus a single main key, or just a single key).
type shortcutRecorder struct {
	widget.BaseWidget

	label     *widget.Label
	canvasRef fyne.Canvas

	capturing bool
	heldMods  map[fyne.KeyName]bool

	// onCaptured fires when a complete combination has been recorded.
	onCaptured func(binding)
}

// Compile-time guarantees that the recorder receives focus, key, and tap events.
var (
	_ desktop.Keyable = (*shortcutRecorder)(nil)
	_ fyne.Tappable   = (*shortcutRecorder)(nil)
)

func newShortcutRecorder(onCaptured func(binding)) *shortcutRecorder {
	r := &shortcutRecorder{
		label:      widget.NewLabel("Click here, then press your shortcut"),
		heldMods:   map[fyne.KeyName]bool{},
		onCaptured: onCaptured,
	}
	r.label.Alignment = fyne.TextAlignCenter
	r.ExtendBaseWidget(r)
	return r
}

func (r *shortcutRecorder) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.StrokeColor = theme.Color(theme.ColorNamePrimary)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.InputRadiusSize()
	return widget.NewSimpleRenderer(container.NewStack(bg, r.label))
}

func (r *shortcutRecorder) MinSize() fyne.Size {
	return fyne.NewSize(320, 56)
}

// start puts the recorder into capture mode and grabs keyboard focus.
func (r *shortcutRecorder) start() {
	r.capturing = true
	r.heldMods = map[fyne.KeyName]bool{}
	r.label.SetText("Press your shortcut now...")
	if r.canvasRef != nil {
		r.canvasRef.Focus(r)
	}
}

// Tapped lets the user click the box to begin recording.
func (r *shortcutRecorder) Tapped(_ *fyne.PointEvent) { r.start() }

// --- fyne.Focusable ---

func (r *shortcutRecorder) FocusGained() {}
func (r *shortcutRecorder) FocusLost() {
	if r.capturing {
		r.capturing = false
		r.label.SetText("Recording cancelled. Click to try again.")
	}
}
func (r *shortcutRecorder) TypedRune(_ rune)          {}
func (r *shortcutRecorder) TypedKey(_ *fyne.KeyEvent) {}

// --- desktop.Keyable ---

func (r *shortcutRecorder) KeyDown(e *fyne.KeyEvent) {
	if !r.capturing {
		return
	}

	// Track held modifiers and show a live preview.
	if _, ok := modifierKeys[e.Name]; ok {
		r.heldMods[e.Name] = true
		r.label.SetText(r.previewText())
		return
	}

	// A non-modifier key finalizes the combination.
	hk, ok := mainKeys[e.Name]
	if !ok {
		r.label.SetText(fmt.Sprintf("Unsupported key %q, try another", string(e.Name)))
		return
	}

	mods, labels := r.collectMods()
	display := joinPlus(append(labels, string(e.Name)))

	r.capturing = false
	r.label.SetText("Captured: " + display)
	if r.onCaptured != nil {
		r.onCaptured(binding{mods: mods, key: hk, display: display})
	}
}

func (r *shortcutRecorder) KeyUp(e *fyne.KeyEvent) {
	if _, ok := modifierKeys[e.Name]; ok {
		delete(r.heldMods, e.Name)
	}
}

// collectMods returns the unique held modifiers (and their labels) in a stable order.
func (r *shortcutRecorder) collectMods() ([]hotkey.Modifier, []string) {
	seen := map[hotkey.Modifier]modInfo{}
	for name := range r.heldMods {
		if info, ok := modifierKeys[name]; ok {
			seen[info.mod] = info
		}
	}
	infos := make([]modInfo, 0, len(seen))
	for _, info := range seen {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].order < infos[j].order })

	mods := make([]hotkey.Modifier, 0, len(infos))
	labels := make([]string, 0, len(infos))
	for _, info := range infos {
		mods = append(mods, info.mod)
		labels = append(labels, info.label)
	}
	return mods, labels
}

func (r *shortcutRecorder) previewText() string {
	_, labels := r.collectMods()

	if len(labels) == 0 {
		return "Press your shortcut now..."
	}

	return joinPlus(labels) + "+..."
}

func joinPlus(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "+"
		}
		out += p
	}
	return out
}
