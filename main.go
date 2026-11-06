// Nvidio - optimisation utility for NVIDIA GeForce GTX 950 and newer (Windows 10/11, 64-bit).
package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

//go:embed ui/index.html
var indexHTML []byte

const (
	planHigh     = "8c5e7fda-e8bf-4a54-9fc9-2fd4d5a42d9e"
	planUltimate = "e9a42b02-d5df-448d-aa00-03f14749eb61"
	absent       = "<absent>"
)

// ---------- helpers ----------

func run(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func smiPath() string {
	if p, err := exec.LookPath("nvidia-smi.exe"); err == nil {
		return p
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvidia-smi.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---------- persistent state (so everything can be undone) ----------

type State struct {
	PlanGUID   string            `json:"planGuid,omitempty"`
	PowerLimit string            `json:"powerLimit,omitempty"`
	Reg        map[string]string `json:"reg"`
}

var (
	mu    sync.Mutex
	state = State{Reg: map[string]string{}}
)

func statePath() string {
	d := filepath.Join(os.Getenv("ProgramData"), "Nvidio")
	os.MkdirAll(d, 0o755)
	return filepath.Join(d, "state.json")
}
func loadState() {
	if b, err := os.ReadFile(statePath()); err == nil {
		json.Unmarshal(b, &state)
	}
	if state.Reg == nil {
		state.Reg = map[string]string{}
	}
}
func saveState() {
	b, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(statePath(), b, 0o644)
}

// ---------- registry tweaks ----------

type regEntry struct{ Key, Name, On string }
type tweak struct {
	ID, Title, Desc string
	Restart         bool
	Entries         []regEntry
}

var tweaks = []tweak{
	{"gamemode", "Windows Game Mode",
		"Prioritises the foreground game and stops Windows from running driver installs or updates mid-session.", false,
		[]regEntry{
			{`HKCU\Software\Microsoft\GameBar`, "AutoGameModeEnabled", "1"},
			{`HKCU\Software\Microsoft\GameBar`, "AllowAutoGameMode", "1"},
		}},
	{"dvr", "Disable background game capture",
		"Turns off Xbox Game Bar background recording, which uses GPU encode and VRAM while you play.", false,
		[]regEntry{
			{`HKCU\System\GameConfigStore`, "GameDVR_Enabled", "0"},
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", "0"},
		}},
	{"transparency", "Lighter desktop compositing",
		"Switches off Windows transparency effects to cut idle GPU load, heat and VRAM use.", false,
		[]regEntry{
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "EnableTransparency", "0"},
		}},
	{"hags", "Hardware-accelerated GPU scheduling",
		"Lets the GPU manage its own memory scheduling. Needs GTX 10-series or newer and a restart. Gains vary, so revert it if you see none.", true,
		[]regEntry{
			{`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode", "2"},
		}},
}

func regGet(key, name string) (string, bool) {
	out, err := run("reg", "query", key, "/v", name)
	if err != nil {
		return "", false
	}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 3 && strings.EqualFold(f[0], name) {
			v := f[len(f)-1]
			if n, err := strconv.ParseInt(v, 0, 64); err == nil {
				return strconv.FormatInt(n, 10), true
			}
			return v, true
		}
	}
	return "", false
}

func tweakApplied(t tweak) bool {
	for _, e := range t.Entries {
		if v, ok := regGet(e.Key, e.Name); !ok || v != e.On {
			return false
		}
	}
	return true
}

func backupReg(e regEntry) {
	k := e.Key + "|" + e.Name
	if _, done := state.Reg[k]; done {
		return
	}
	if v, ok := regGet(e.Key, e.Name); ok {
		state.Reg[k] = v
	} else {
		state.Reg[k] = absent
	}
}

func restoreReg(e regEntry) error {
	k := e.Key + "|" + e.Name
	orig, ok := state.Reg[k]
	if !ok {
		return nil
	}
	var err error
	if orig == absent {
		_, err = run("reg", "delete", e.Key, "/v", e.Name, "/f")
		if _, still := regGet(e.Key, e.Name); !still {
			err = nil
		}
	} else {
		_, err = run("reg", "add", e.Key, "/v", e.Name, "/t", "REG_DWORD", "/d", orig, "/f")
	}
	if err == nil {
		delete(state.Reg, k)
	}
	return err
}

func setTweak(id string, enable bool) error {
	for _, t := range tweaks {
		if t.ID != id {
			continue
		}
		for _, e := range t.Entries {
			if enable {
				backupReg(e)
				if out, err := run("reg", "add", e.Key, "/v", e.Name, "/t", "REG_DWORD", "/d", e.On, "/f"); err != nil {
					return fmt.Errorf("registry write failed: %s", out)
				}
			} else if err := restoreReg(e); err != nil {
				return err
			}
		}
		saveState()
		return nil
	}
	return fmt.Errorf("unknown tweak")
}

// ---------- power plan ----------

var guidRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func activePlan() (guid, name string) {
	out, _ := run("powercfg", "/getactivescheme")
	guid = strings.ToLower(guidRe.FindString(out))
	if i := strings.Index(out, "("); i >= 0 {
		name = strings.Trim(out[i:], "() ")
	}
	return
}

func setHighPerformance(enable bool) error {
	cur, _ := activePlan()
	if enable {
		if cur == planHigh || cur == planUltimate {
			return nil
		}
		if state.PlanGUID == "" {
			state.PlanGUID = cur
		}
		if _, err := run("powercfg", "/setactive", planHigh); err != nil {
			// Plan hidden on this PC (modern standby laptops): create it first.
			out, derr := run("powercfg", "-duplicatescheme", planHigh)
			g := strings.ToLower(guidRe.FindString(out))
			if derr != nil || g == "" {
				return fmt.Errorf("could not create High performance plan: %s", out)
			}
			if out, err := run("powercfg", "/setactive", g); err != nil {
				return fmt.Errorf("could not activate plan: %s", out)
			}
		}
		saveState()
		return nil
	}
	if state.PlanGUID != "" {
		if _, err := run("powercfg", "/setactive", state.PlanGUID); err != nil {
			run("powercfg", "/setactive", "381b4222-f694-41f0-9685-ff5bb260df2e") // Balanced
		}
		state.PlanGUID = ""
		saveState()
	}
	return nil
}

// ---------- GPU ----------

var smiFields = []string{"name", "driver_version", "temperature.gpu", "utilization.gpu",
	"power.draw", "power.limit", "power.default_limit", "power.min_limit", "power.max_limit",
	"clocks.gr", "clocks.max.gr", "fan.speed", "memory.used", "memory.total"}

func gpuInfo() (map[string]string, string) {
	p := smiPath()
	if p == "" {
		return nil, "nvidia-smi was not found. Install or repair your NVIDIA driver and try again."
	}
	out, err := run(p, "-i", "0", "--query-gpu="+strings.Join(smiFields, ","), "--format=csv,noheader,nounits")
	if err != nil {
		return nil, "Could not talk to the NVIDIA driver: " + out
	}
	parts := strings.Split(strings.SplitN(out, "\n", 2)[0], ",")
	if len(parts) < len(smiFields) {
		return nil, "Unexpected nvidia-smi output: " + out
	}
	m := map[string]string{}
	for i, f := range smiFields {
		m[f] = strings.TrimSpace(parts[i])
	}
	return m, ""
}

func setPowerPercent(pct int) error {
	g, msg := gpuInfo()
	if g == nil {
		return fmt.Errorf("%s", msg)
	}
	def, err1 := strconv.ParseFloat(g["power.default_limit"], 64)
	if err1 != nil {
		return fmt.Errorf("this GPU does not expose a power limit (common on laptops and some older cards)")
	}
	if state.PowerLimit == "" {
		state.PowerLimit = g["power.limit"]
	}
	w := def * float64(pct) / 100
	if mn, err := strconv.ParseFloat(g["power.min_limit"], 64); err == nil && w < mn {
		w = mn
	}
	if mx, err := strconv.ParseFloat(g["power.max_limit"], 64); err == nil && w > mx {
		w = mx
	}
	out, err := run(smiPath(), "-i", "0", "-pl", strconv.Itoa(int(w+0.5)))
	if err != nil {
		return fmt.Errorf("driver refused the power limit change: %s", firstLine(out))
	}
	if pct == 100 {
		state.PowerLimit = ""
	}
	saveState()
	return nil
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

// ---------- HTTP ----------

var (
	token    string
	port     int
	live     int32
	everSeen int32
)

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Guard against other websites / DNS rebinding talking to this local server.
		if r.Host != fmt.Sprintf("127.0.0.1:%d", port) || r.Header.Get("X-Nvidio-Token") != token {
			jsonOut(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		h(w, r)
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	g, msg := gpuInfo()
	type tw struct {
		ID, Title, Desc string
		Restart, On     bool
	}
	var list []tw
	for _, t := range tweaks {
		list = append(list, tw{t.ID, t.Title, t.Desc, t.Restart, tweakApplied(t)})
	}
	guid, name := activePlan()
	jsonOut(w, 200, map[string]any{
		"gpu": g, "gpuError": msg, "tweaks": list,
		"plan": map[string]any{"name": name, "on": guid == planHigh || guid == planUltimate || (state.PlanGUID != "" && guid != state.PlanGUID)},
	})
}

func actionHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	var req struct {
		Action  string `json:"action"`
		ID      string `json:"id"`
		Enable  bool   `json:"enable"`
		Percent int    `json:"percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonOut(w, 400, map[string]string{"error": "bad request"})
		return
	}
	var err error
	switch req.Action {
	case "tweak":
		err = setTweak(req.ID, req.Enable)
	case "plan":
		err = setHighPerformance(req.Enable)
	case "power":
		if req.Percent < 50 || req.Percent > 100 {
			err = fmt.Errorf("percent out of range")
		} else {
			err = setPowerPercent(req.Percent)
		}
	case "restore":
		var errs []string
		for _, t := range tweaks {
			for _, e := range t.Entries {
				if e2 := restoreReg(e); e2 != nil {
					errs = append(errs, e2.Error())
				}
			}
		}
		if e2 := setHighPerformance(false); e2 != nil {
			errs = append(errs, e2.Error())
		}
		if e2 := setPowerPercent(100); e2 != nil && smiPath() != "" {
			errs = append(errs, e2.Error())
		}
		state = State{Reg: map[string]string{}}
		saveState()
		if len(errs) > 0 {
			err = fmt.Errorf("%s", strings.Join(errs, "; "))
		}
	default:
		err = fmt.Errorf("unknown action")
	}
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": err.Error()})
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func liveHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&live, 1)
	atomic.StoreInt32(&everSeen, 1)
	defer atomic.AddInt32(&live, -1)
	w.WriteHeader(200)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	<-r.Context().Done()
}

// ---------- window ----------

func openWindow(url string) {
	var bases []string
	for _, e := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if v := os.Getenv(e); v != "" {
			bases = append(bases, v)
		}
	}
	rel := []string{`Microsoft\Edge\Application\msedge.exe`, `Google\Chrome\Application\chrome.exe`}
	profile := filepath.Join(os.TempDir(), "nvidio-ui")
	for _, r := range rel {
		for _, b := range bases {
			p := filepath.Join(b, r)
			if _, err := os.Stat(p); err == nil {
				c := exec.Command(p, "--app="+url, "--window-size=1060,740", "--user-data-dir="+profile,
					"--no-first-run", "--no-default-browser-check", "--disable-extensions")
				if c.Start() == nil {
					return
				}
			}
		}
	}
	run("rundll32", "url.dll,FileProtocolHandler", url)
}

func main() {
	loadState()
	b := make([]byte, 16)
	rand.Read(b)
	token = hex.EncodeToString(b)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Host != fmt.Sprintf("127.0.0.1:%d", port) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/api/status", auth(statusHandler))
	mux.HandleFunc("/api/action", auth(actionHandler))
	mux.HandleFunc("/api/live", auth(liveHandler))
	mux.HandleFunc("/api/quit", auth(func(w http.ResponseWriter, r *http.Request) { os.Exit(0) }))
	go http.Serve(ln, mux)

	openWindow(fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, token))

	// Exit when the window is closed (its live connection drops) or never opened.
	start := time.Now()
	var idleSince time.Time
	for {
		time.Sleep(500 * time.Millisecond)
		if atomic.LoadInt32(&everSeen) == 0 {
			if time.Since(start) > 45*time.Second {
				return
			}
			continue
		}
		if atomic.LoadInt32(&live) == 0 {
			if idleSince.IsZero() {
				idleSince = time.Now()
			} else if time.Since(idleSince) > 4*time.Second {
				return
			}
		} else {
			idleSince = time.Time{}
		}
	}
}
