package main

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/hotkey"
)

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
