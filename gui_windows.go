// Nvidio 1.0 - native Win32 interface (no browser, no third-party libraries).
package main

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassEx   = user32.NewProc("RegisterClassExW")
	pCreateWindowEx    = user32.NewProc("CreateWindowExW")
	pDefWindowProc     = user32.NewProc("DefWindowProcW")
	pShowWindow        = user32.NewProc("ShowWindow")
	pUpdateWindow      = user32.NewProc("UpdateWindow")
	pGetMessage        = user32.NewProc("GetMessageW")
	pTranslateMessage  = user32.NewProc("TranslateMessage")
	pDispatchMessage   = user32.NewProc("DispatchMessageW")
	pIsDialogMessage   = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage   = user32.NewProc("PostQuitMessage")
	pPostMessage       = user32.NewProc("PostMessageW")
	pSendMessage       = user32.NewProc("SendMessageW")
	pSetWindowText     = user32.NewProc("SetWindowTextW")
	pBeginPaint        = user32.NewProc("BeginPaint")
	pEndPaint          = user32.NewProc("EndPaint")
	pInvalidateRect    = user32.NewProc("InvalidateRect")
	pFillRect          = user32.NewProc("FillRect")
	pFrameRect         = user32.NewProc("FrameRect")
	pDrawText          = user32.NewProc("DrawTextW")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pLoadCursor        = user32.NewProc("LoadCursorW")
	pLoadIcon          = user32.NewProc("LoadIconW")
	pMessageBox        = user32.NewProc("MessageBoxW")
	pAdjustWindowRect  = user32.NewProc("AdjustWindowRectEx")
	pGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	pEnableWindow      = user32.NewProc("EnableWindow")
	pGetDC             = user32.NewProc("GetDC")
	pReleaseDC         = user32.NewProc("ReleaseDC")
	pGetModuleHandle   = kernel32.NewProc("GetModuleHandleW")
	pCreateFont        = gdi32.NewProc("CreateFontW")
	pCreateSolidBrush  = gdi32.NewProc("CreateSolidBrush")
	pSelectObject      = gdi32.NewProc("SelectObject")
	pDeleteObject      = gdi32.NewProc("DeleteObject")
	pSetTextColor      = gdi32.NewProc("SetTextColor")
	pSetBkColor        = gdi32.NewProc("SetBkColor")
	pSetBkMode         = gdi32.NewProc("SetBkMode")
	pCreateCompatDC    = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatBmp   = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt            = gdi32.NewProc("BitBlt")
	pDeleteDC          = gdi32.NewProc("DeleteDC")
	pGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	pGetTextExtent     = gdi32.NewProc("GetTextExtentPoint32W")
)

const (
	wmCreate, wmDestroy, wmPaint, wmEraseBkgnd = 0x1, 0x2, 0xF, 0x14
	wmSetFont, wmCommand, wmCtlColorStatic     = 0x30, 0x111, 0x138
	wmApp                                      = 0x8000
	bmGetCheck, bmSetCheck                     = 0xF0, 0xF1

	wsChild, wsVisible, wsTabstop, wsGroup = 0x40000000, 0x10000000, 0x10000, 0x20000
	wsCaption, wsSysmenu, wsMinimize, wsClip = 0xC00000, 0x80000, 0x20000, 0x2000000
	bsPush, bsDefPush, bsAutoCheck, bsAutoRadio = 0, 1, 3, 9
	ssNoPrefix                                  = 0x80

	dtLeft, dtVCenter, dtSingle, dtNoPrefix, dtEllipsis = 0, 4, 0x20, 0x800, 0x8000
)

type rect struct{ L, T, R, B int32 }
type point struct{ X, Y int32 }
type msgT struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}
type paintStruct struct {
	Hdc       uintptr
	FErase    int32
	Paint     rect
	FRestore  int32
	FIncUpd   int32
	Reserved  [32]byte
}
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}
type sizeT struct{ CX, CY int32 }

func rgb(r, g, b int) uintptr { return uintptr(r | g<<8 | b<<16) }

var (
	cBg     = rgb(243, 243, 243)
	cText   = rgb(31, 31, 31)
	cGray   = rgb(96, 96, 96)
	cBorder = rgb(218, 218, 218)
	cTrack  = rgb(229, 229, 229)
	cAccent = rgb(0, 103, 192)
	cWarn   = rgb(214, 120, 0)
	cBad    = rgb(196, 43, 28)
	cWhite  = rgb(255, 255, 255)
)

// layout in 96-dpi units
const (
	clientW, clientH = 640, 668
	margin           = 24
	statsTop         = 112
	tileW, tileH     = 189, 50
	tileGap          = 10
)

type item struct {
	id          int
	kind, title string
	desc        string
	hwnd        uintptr
}

var items = []item{
	{kind: "plan", title: "High performance power plan", desc: "Keeps the CPU at full speed so it does not hold the GPU back."},
	{kind: "gamemode", title: "Game Mode", desc: "Gives the game you are playing priority over background tasks."},
	{kind: "dvr", title: "Stop background game capture", desc: "Turns off Xbox Game Bar recording, which uses GPU encoding and video memory."},
	{kind: "bgapps", title: "Block background apps", desc: "Stops Store apps from running in the background while you play."},
	{kind: "priority", title: "Prioritise games", desc: "Raises game and foreground app priority in the Windows scheduler."},
	{kind: "svc", title: "Pause background services", desc: "Pauses indexing, telemetry and prefetch until you turn this off or restart."},
	{kind: "visuals", title: "Lighter visuals", desc: "Turns off transparency and animations. Some changes apply after sign-out."},
	{kind: "hags", title: "GPU scheduling (restart)", desc: "GTX 10-series or newer. Needs a restart. Gains vary, so revert if none."},
}

var ui struct {
	hwnd, hinst, dpi uintptr
	fNormal, fSmall, fSemi, fTitle, fBig, fSemiSmall uintptr
	brBg, brWhite, brBorder, brTrack, brAccent, brWarn, brBad uintptr
	gpuLine, status                                             uintptr
	radios                                                      [3]uintptr
	btnOpt, btnRestore                                          uintptr
	colors                                                      map[uintptr]uintptr
	stats                                                       map[string]string
	busy                                                        int
	statusColor                                                 uintptr
}

var shared struct {
	sync.Mutex
	stats    map[string]string
	statsErr string
	st       stateSnap
	msg      string
	msgErr   bool
}

func S(v int) int { return v * int(ui.dpi) / 96 }

func send(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessage.Call(h, uintptr(m), w, l)
	return r
}
func post(m uint32) { pPostMessage.Call(ui.hwnd, uintptr(wmApp+m), 0, 0) }
func setText(h uintptr, s string) {
	p := p16(s)
	pSetWindowText.Call(h, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

func font(pt int, weight int, face string) uintptr {
	h := -(pt * int(ui.dpi) / 72)
	f := p16(face)
	r, _, _ := pCreateFont.Call(uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(f)))
	runtime.KeepAlive(f)
	return r
}
func brush(c uintptr) uintptr { r, _, _ := pCreateSolidBrush.Call(c); return r }

func mk(class, text string, style uint32, x, y, w, h, id int, f uintptr) uintptr {
	c, t := p16(class), p16(text)
	hw, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)),
		uintptr(wsChild|wsVisible|style), uintptr(S(x)), uintptr(S(y)), uintptr(S(w)), uintptr(S(h)),
		ui.hwnd, uintptr(id), ui.hinst, 0)
	runtime.KeepAlive(c)
	runtime.KeepAlive(t)
	send(hw, wmSetFont, f, 1)
	return hw
}
func label(text string, x, y, w, h int, f, color uintptr) uintptr {
	hw := mk("STATIC", text, ssNoPrefix, x, y, w, h, 0, f)
	ui.colors[hw] = color
	return hw
}

// ---------- painting the stat tiles ----------

type tile struct {
	label, unit string
	v, pct      float64
	ok          bool
	warn, bad   float64
}

func num(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func tiles() []tile {
	g := ui.stats
	t := []tile{{label: "Temperature", unit: "°C", warn: 75, bad: 85}, {label: "GPU load", unit: "%"},
		{label: "Power draw", unit: "W"}, {label: "Core clock", unit: "MHz"}, {label: "Fan speed", unit: "%"},
		{label: "Video memory", unit: "MB"}}
	if g == nil {
		return t
	}
	t[0].v, t[0].ok = num(g["temperature.gpu"])
	t[0].pct = t[0].v
	t[1].v, t[1].ok = num(g["utilization.gpu"])
	t[1].pct = t[1].v
	t[2].v, t[2].ok = num(g["power.draw"])
	if pl, ok := num(g["power.limit"]); ok && pl > 0 {
		t[2].pct = t[2].v / pl * 100
	}
	t[3].v, t[3].ok = num(g["clocks.gr"])
	if mx, ok := num(g["clocks.max.gr"]); ok && mx > 0 {
		t[3].pct = t[3].v / mx * 100
	}
	t[4].v, t[4].ok = num(g["fan.speed"])
	t[4].pct = t[4].v
	t[5].v, t[5].ok = num(g["memory.used"])
	if mt, ok := num(g["memory.total"]); ok && mt > 0 {
		t[5].pct = t[5].v / mt * 100
	}
	return t
}

func statsRect() rect {
	return rect{int32(S(margin)), int32(S(statsTop)), int32(S(margin + 3*tileW + 2*tileGap)), int32(S(statsTop + 2*tileH + tileGap))}
}

func drawText(hdc uintptr, s string, r rect, flags uintptr) {
	p := p16(s)
	pDrawText.Call(hdc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	runtime.KeepAlive(p)
}
func fill(hdc uintptr, r rect, br uintptr) { pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), br) }

func onPaint() {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(ui.hwnd, uintptr(unsafe.Pointer(&ps)))
	area := statsRect()
	w, h := int(area.R-area.L), int(area.B-area.T)
	mdc, _, _ := pCreateCompatDC.Call(hdc)
	bmp, _, _ := pCreateCompatBmp.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mdc, bmp)
	pSetBkMode.Call(mdc, 1)
	fill(mdc, rect{0, 0, int32(w), int32(h)}, ui.brBg)

	for i, t := range tiles() {
		col, row := i%3, i/3
		x, y := S(col*(tileW+tileGap)), S(row*(tileH+tileGap))
		tw, th := S(tileW), S(tileH)
		box := rect{int32(x), int32(y), int32(x + tw), int32(y + th)}
		fill(mdc, box, ui.brWhite)
		pFrameRect.Call(mdc, uintptr(unsafe.Pointer(&box)), ui.brBorder)

		pSelectObject.Call(mdc, ui.fSmall)
		pSetTextColor.Call(mdc, cGray)
		drawText(mdc, t.label, rect{int32(x + S(10)), int32(y + S(5)), int32(x + tw), int32(y + S(21))}, dtLeft|dtSingle|dtNoPrefix)

		val, unit := "n/a", ""
		if t.ok {
			val = strconv.Itoa(int(t.v + 0.5))
			unit = t.unit
		}
		pSelectObject.Call(mdc, ui.fBig)
		pSetTextColor.Call(mdc, cText)
		var sz sizeT
		vp := p16(val)
		pGetTextExtent.Call(mdc, uintptr(unsafe.Pointer(vp)), uintptr(len(val)), uintptr(unsafe.Pointer(&sz)))
		drawText(mdc, val, rect{int32(x + S(10)), int32(y + S(18)), int32(x + tw), int32(y + S(40))}, dtLeft|dtSingle|dtNoPrefix)
		if unit != "" {
			pSelectObject.Call(mdc, ui.fSmall)
			pSetTextColor.Call(mdc, cGray)
			drawText(mdc, unit, rect{int32(x + S(10) + int(sz.CX) + S(4)), int32(y + S(25)), int32(x + tw), int32(y + S(41))}, dtLeft|dtSingle|dtNoPrefix)
		}

		// thin progress bar
		bx0, bx1, by := x+S(10), x+tw-S(10), y+th-S(7)
		fill(mdc, rect{int32(bx0), int32(by), int32(bx1), int32(by + S(3))}, ui.brTrack)
		if t.ok && t.pct > 0 {
			p := t.pct
			if p > 100 {
				p = 100
			}
			br := ui.brAccent
			if t.bad > 0 && t.v >= t.bad {
				br = ui.brBad
			} else if t.warn > 0 && t.v >= t.warn {
				br = ui.brWarn
			}
			fill(mdc, rect{int32(bx0), int32(by), int32(bx0 + int(float64(bx1-bx0)*p/100)), int32(by + S(3))}, br)
		}
	}
	pBitBlt.Call(hdc, uintptr(area.L), uintptr(area.T), uintptr(w), uintptr(h), mdc, 0, 0, 0x00CC0020)
	pSelectObject.Call(mdc, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mdc)
	pEndPaint.Call(ui.hwnd, uintptr(unsafe.Pointer(&ps)))
}

// ---------- state <-> controls ----------

func applyStats() {
	shared.Lock()
	ui.stats = shared.stats
	errMsg := shared.statsErr
	shared.Unlock()
	if ui.stats != nil {
		setText(ui.gpuLine, ui.stats["name"]+"   |   Driver "+ui.stats["driver_version"])
	} else {
		setText(ui.gpuLine, "No NVIDIA GPU detected")
		if ui.busy == 0 && errMsg != "" {
			setStatus(errMsg, true)
		}
	}
	r := statsRect()
	pInvalidateRect.Call(ui.hwnd, uintptr(unsafe.Pointer(&r)), 0)
}

func applyState() {
	shared.Lock()
	st := shared.st
	shared.Unlock()
	for _, it := range items {
		on := false
		switch it.kind {
		case "plan":
			on = st.Plan
		case "svc":
			on = st.Svc
		default:
			on = st.Tweaks[it.kind]
		}
		v := uintptr(0)
		if on {
			v = 1
		}
		send(it.hwnd, bmSetCheck, v, 0)
	}
	pct := []float64{1, 0.9, 0.8}
	for i, r := range ui.radios {
		v := uintptr(0)
		if st.HasPL && abs(st.Ratio-pct[i]) < 0.025 {
			v = 1
		}
		send(r, bmSetCheck, v, 0)
		en := uintptr(0)
		if st.HasPL {
			en = 1
		}
		pEnableWindow.Call(r, en)
	}
}
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func setStatus(s string, isErr bool) {
	ui.colors[ui.status] = cGray
	if isErr {
		ui.colors[ui.status] = cBad
	}
	setText(ui.status, s)
}

// run a backend action off the UI thread
func act(okMsg string, fn func() error) {
	ui.busy++
	setStatus("Working...", false)
	go func() {
		mu.Lock()
		err := fn()
		mu.Unlock()
		shared.Lock()
		shared.msg, shared.msgErr = okMsg, false
		if err != nil {
			shared.msg, shared.msgErr = "Could not finish: "+err.Error(), true
		}
		shared.Unlock()
		post(2)
		st := gatherState()
		shared.Lock()
		shared.st = st
		shared.Unlock()
		post(3)
	}()
}

func isChecked(h uintptr) bool { return send(h, bmGetCheck, 0, 0) == 1 }

func onCommand(id int) {
	switch {
	case id >= 100 && id <= 102:
		pcts := []int{100, 90, 80}
		names := []string{"Performance", "Balanced", "Cool & quiet"}
		p := pcts[id-100]
		act("Thermal profile set: "+names[id-100], func() error { return setPowerPercent(p) })
	case id >= 110 && id < 110+len(items):
		it := items[id-110]
		on := isChecked(it.hwnd)
		msg := "Turned off: " + it.title
		if on {
			msg = "Turned on: " + it.title
			if it.kind == "hags" {
				msg += ". Restart Windows to finish."
			}
		}
		act(msg, func() error { return applyItem(it.kind, on) })
	case id == 200:
		act("Optimised. Game Mode, priorities and background tasks are set.", optimiseNow)
	case id == 201:
		t, c := p16("Undo every change Nvidio has made and restore your previous settings?"), p16("Nvidio")
		r, _, _ := pMessageBox.Call(ui.hwnd, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x24)
		runtime.KeepAlive(t)
		runtime.KeepAlive(c)
		if r == 6 {
			act("Everything restored to your previous settings.", restoreAll)
		}
	}
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch uint32(msg) {
	case wmEraseBkgnd:
		var r rect
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
		fill(wp, r, ui.brBg)
		return 1
	case wmPaint:
		onPaint()
		return 0
	case wmCtlColorStatic:
		col, ok := ui.colors[lp]
		if !ok {
			col = cText
		}
		pSetTextColor.Call(wp, col)
		pSetBkColor.Call(wp, cBg)
		return ui.brBg
	case wmCommand:
		if (wp>>16)&0xFFFF == 0 {
			onCommand(int(wp & 0xFFFF))
		}
		return 0
	case wmApp + 1:
		applyStats()
		return 0
	case wmApp + 2:
		ui.busy--
		shared.Lock()
		m, e := shared.msg, shared.msgErr
		shared.Unlock()
		setStatus(m, e)
		return 0
	case wmApp + 3:
		if ui.busy == 0 {
			applyState()
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func main() {
	runtime.LockOSThread()
	loadState()

	hdc, _, _ := pGetDC.Call(0)
	dpi, _, _ := pGetDeviceCaps.Call(hdc, 88)
	pReleaseDC.Call(0, hdc)
	if dpi == 0 {
		dpi = 96
	}
	ui.dpi = dpi
	ui.colors = map[uintptr]uintptr{}
	hi, _, _ := pGetModuleHandle.Call(0)
	ui.hinst = hi

	ui.fNormal = font(9, 400, "Segoe UI")
	ui.fSmall = font(8, 400, "Segoe UI")
	ui.fSemi = font(10, 600, "Segoe UI")
	ui.fSemiSmall = font(8, 600, "Segoe UI")
	ui.fTitle = font(20, 300, "Segoe UI")
	ui.fBig = font(15, 600, "Segoe UI")
	ui.brBg, ui.brWhite, ui.brBorder = brush(cBg), brush(cWhite), brush(cBorder)
	ui.brTrack, ui.brAccent, ui.brWarn, ui.brBad = brush(cTrack), brush(cAccent), brush(cWarn), brush(cBad)

	cls := p16("NvidioWindow")
	cur, _, _ := pLoadCursor.Call(0, 32512)
	ico, _, _ := pLoadIcon.Call(hi, 1)
	wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: syscall.NewCallback(wndProc),
		Instance: hi, Icon: ico, IconSm: ico, Cursor: cur, ClassName: cls}
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	style := uintptr(wsCaption | wsSysmenu | wsMinimize | wsClip)
	r := rect{0, 0, int32(S(clientW)), int32(S(clientH))}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	ww, wh := int(r.R-r.L), int(r.B-r.T)
	sx, _, _ := pGetSystemMetrics.Call(0)
	sy, _, _ := pGetSystemMetrics.Call(1)
	title := p16("Nvidio 1.0")
	h, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), style,
		(sx-uintptr(ww))/2, (sy-uintptr(wh))/2, uintptr(ww), uintptr(wh), 0, 0, hi, 0)
	ui.hwnd = h

	// header
	label("Nvidio", margin, 14, 300, 36, ui.fTitle, cText)
	label("GPU optimiser for GeForce GTX 950 and newer", margin, 52, 400, 18, ui.fSmall, cGray)
	v := mk("STATIC", "Version 1.0", ssNoPrefix|2, clientW-margin-120, 22, 120, 18, 0, ui.fSmall)
	ui.colors[v] = cGray
	ui.gpuLine = label("Detecting GPU...", margin, 82, 592, 20, ui.fSemi, cText)

	// thermal profile
	label("THERMAL PROFILE", margin, 238, 300, 16, ui.fSemiSmall, cGray)
	rl := []string{"Performance (stock)", "Balanced (90% power)", "Cool && quiet (80% power)"}
	for i := range ui.radios {
		st := uint32(bsAutoRadio | wsTabstop)
		if i == 0 {
			st |= wsGroup
		}
		ui.radios[i] = mk("BUTTON", rl[i], st, margin+i*200, 260, 195, 22, 100+i, ui.fNormal)
	}
	label("Lower power limits reduce heat and fan noise for a small speed cost. Resets on restart.", margin, 286, 592, 18, ui.fSmall, cGray)

	// optimisations
	label("OPTIMISATIONS", margin, 322, 300, 16, ui.fSemiSmall, cGray)
	for i := range items {
		col, row := i%2, i/2
		x, y := margin+col*296, 344+row*58
		items[i].id = 110 + i
		items[i].hwnd = mk("BUTTON", items[i].title, bsAutoCheck|wsTabstop, x, y, 284, 22, items[i].id, ui.fNormal)
		label(items[i].desc, x+22, y+23, 270, 32, ui.fSmall, cGray)
	}

	ui.btnOpt = mk("BUTTON", "Optimise now", bsDefPush|wsTabstop, margin, 590, 140, 32, 200, ui.fNormal)
	ui.btnRestore = mk("BUTTON", "Restore defaults", bsPush|wsTabstop, margin+152, 590, 140, 32, 201, ui.fNormal)
	ui.status = label("Ready", margin, 634, 592, 20, ui.fSmall, cGray)

	pShowWindow.Call(h, 5)
	pUpdateWindow.Call(h)

	go func() {
		st := gatherState()
		shared.Lock()
		shared.st = st
		shared.Unlock()
		post(3)
	}()
	go func() {
		for {
			g, msg := gpuInfo()
			shared.Lock()
			shared.stats, shared.statsErr = g, msg
			shared.Unlock()
			post(1)
			time.Sleep(2 * time.Second)
		}
	}()

	var m msgT
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if d, _, _ := pIsDialogMessage.Call(h, uintptr(unsafe.Pointer(&m))); d != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	_ = fmt.Sprint
}
