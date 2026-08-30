package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

var state = &HeistGUI{
	clients: make(map[*websocket.Conn]bool),
}

func main() {
	defer recoverLog()

	killswitch(state)
}

func killswitch(g *HeistGUI) {

	g.server.WriteMessage(12, []byte("aaaaa"))

	if _, err := os.Stat(settings_file); os.IsNotExist(err) {
		_ = os.WriteFile(settings_file,
			[]byte(default_config), 0644)
		fmt.Printf("Created %s with default config\n", settings_file)
	} else {
		fmt.Printf("%s already exists\n", settings_file)
	}

	myApp := app.NewWithID("com.heistkillswitch")
	myApp.Settings().SetTheme(&customTheme{Theme: theme.DefaultTheme()})

	myWindow := myApp.NewWindow("GTA V CMM Panic Switch")
	myWindow.Resize(fyne.NewSize(500, 600))

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
		container.NewGridWithColumns(2,
			widget.NewLabel("Client: Host Address (ws://...):"),
			addrInput,
		),
	)

	hostContainer := container.NewVBox(
		container.NewGridWithColumns(3,
			widget.NewLabel("Host Only: DuckDNS Subdomain & Token:"),
			domainInput, tokenInput),
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

		goSafe(onHotKeyPress, g, hk)
	})

	g.logArea = widget.NewMultiLineEntry()
	g.logArea.SetText("[i] Ready. Configure your settings and click Initialize.")
	g.logArea.Disable()

	initBtn := widget.NewButton("Initialize Network", func() {
		g.Addr = addrInput.Text
		g.DuckDomain = domainInput.Text
		g.DuckToken = tokenInput.Text

		if g.isHost {
			goSafe(func() { g.startServer() })
			goSafe(func() { g.startDuckDNSUpdater() })
			g.appendLog("[+] Host server & IPv6 DuckDNS auto-updater initialized...")
		} else {
			goSafe(func() { g.startClient() })
			g.appendLog(fmt.Sprintf("[*] Connecting to host at %s...", g.Addr))
		}

		{
			config, _ := json.MarshalIndent(g, "    ", "")
			_ = os.WriteFile(settings_file, config, 0644)
		}
	})
	initBtn.Importance = widget.HighImportance

	avg_latency := canvas.NewText("0", color.White)
	worst_latency := canvas.NewText("0", color.White)

	goSafe(func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			fyne.DoAndWait(func() {

				change_color := func(l *canvas.Text, val *int64) {
					if *val < 60 {
						l.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
					} else if *val < 120 {
						l.Color = color.RGBA{R: 255, G: 255, B: 0, A: 255}
					} else {
						l.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
					}
				}

				change_color(avg_latency, &stats.Avg_latency)
				change_color(worst_latency, &stats.Worst_latency)

				avg_latency.Text = strconv.FormatInt(stats.Avg_latency, 10)
				worst_latency.Text = strconv.FormatInt(stats.Worst_latency, 10)
				avg_latency.Refresh()
				worst_latency.Refresh()
			})
		}
	})

	topContainer := container.NewVBox(
		widget.NewSeparator(),
		container.NewGridWithColumns(2, widget.NewLabel("Operating Mode:"), roleSelect),
		clientContainer,
		hostContainer,
		container.NewGridWithColumns(3,
			widget.NewLabel("Panic Shortcut Combo:"),
			recorder, registerBtn,
		),
		initBtn,

		widget.NewSeparator(),
		widget.NewLabel("Connection Stats"),
		container.NewHBox(
			widget.NewLabel("Average Latency (ms):"),
			avg_latency,
		),
		container.NewHBox(
			widget.NewLabel("Worst Latency (ms):"),
			worst_latency,
		),

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
