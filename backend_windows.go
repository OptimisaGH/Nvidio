package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
	"encoding/json"
)

const (
	planHigh     = "8c5e7fda-e8bf-4a54-9fc9-2fd4d5a42d9e"
	planUltimate = "e9a42b02-d5df-448d-aa00-03f14749eb61"
	absent       = "<absent>"
	regDWORD     = 4
	regSZ        = 1
)

// ---------- helpers ----------

func run(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func p16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

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
	Stopped    []string          `json:"stoppedServices,omitempty"`
	Reg        map[string]string `json:"reg"`
}

var (
	mu    sync.Mutex // serialises every action; callers of backend funcs hold it
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

// ---------- registry (native, no reg.exe) ----------

var (
	advapi             = syscall.NewLazyDLL("advapi32.dll")
	pRegCreateKeyEx    = advapi.NewProc("RegCreateKeyExW")
	pRegSetValueEx     = advapi.NewProc("RegSetValueExW")
	pRegDeleteValue    = advapi.NewProc("RegDeleteValueW")
)

func splitKey(k string) (syscall.Handle, string) {
	i := strings.Index(k, `\`)
	root := syscall.Handle(syscall.HKEY_LOCAL_MACHINE)
	if k[:i] == "HKCU" {
		root = syscall.Handle(syscall.HKEY_CURRENT_USER)
	}
	return root, k[i+1:]
}

func regRead(key, name string) (typ uint32, val string, ok bool) {
	root, sub := splitKey(key)
	var h syscall.Handle
	if syscall.RegOpenKeyEx(root, p16(sub), 0, syscall.KEY_READ, &h) != nil {
		return
	}
	defer syscall.RegCloseKey(h)
	var n uint32
	if syscall.RegQueryValueEx(h, p16(name), nil, &typ, nil, &n) != nil {
		return
	}
	buf := make([]byte, n+4)
	n2 := uint32(len(buf))
	if syscall.RegQueryValueEx(h, p16(name), nil, &typ, &buf[0], &n2) != nil {
		return
	}
	switch typ {
	case regDWORD:
		if n2 >= 4 {
			v := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
			return typ, strconv.FormatUint(uint64(v), 10), true
		}
	case regSZ:
		u := make([]uint16, n2/2)
		for i := range u {
			u[i] = uint16(buf[2*i]) | uint16(buf[2*i+1])<<8
		}
		return typ, syscall.UTF16ToString(u), true
	}
	return typ, "", false
}

func regWrite(key, name string, typ uint32, val string) error {
	root, sub := splitKey(key)
	var h syscall.Handle
	r, _, _ := pRegCreateKeyEx.Call(uintptr(root), uintptr(unsafe.Pointer(p16(sub))), 0, 0, 0,
		0x20006, 0, uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return fmt.Errorf("could not open registry key (error %d). Run Nvidio as administrator", r)
	}
	defer syscall.RegCloseKey(h)
	var data []byte
	if typ == regDWORD {
		n, _ := strconv.ParseUint(val, 10, 32)
		data = []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}
	} else {
		for _, c := range syscall.StringToUTF16(val) {
			data = append(data, byte(c), byte(c>>8))
		}
	}
	r, _, _ = pRegSetValueEx.Call(uintptr(h), uintptr(unsafe.Pointer(p16(name))), 0, uintptr(typ),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if r != 0 {
		return fmt.Errorf("registry write failed (error %d)", r)
	}
	return nil
}

func regDelete(key, name string) {
	root, sub := splitKey(key)
	var h syscall.Handle
	if syscall.RegOpenKeyEx(root, p16(sub), 0, 0x20006, &h) != nil {
		return
	}
	defer syscall.RegCloseKey(h)
	pRegDeleteValue.Call(uintptr(h), uintptr(unsafe.Pointer(p16(name))))
}

// ---------- tweaks ----------

type regEntry struct {
	Key, Name string
	Type      uint32
	Val       string
}
type tweak struct {
	ID, Title, Desc string
	Restart         bool
	Entries         []regEntry
}

const mm = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`

var tweaks = []tweak{
	{"gamemode", "Game Mode", "Gives the game you are playing priority over background tasks.", false,
		[]regEntry{
			{`HKCU\Software\Microsoft\GameBar`, "AutoGameModeEnabled", regDWORD, "1"},
			{`HKCU\Software\Microsoft\GameBar`, "AllowAutoGameMode", regDWORD, "1"},
		}},
	{"dvr", "Stop background game capture", "Turns off Xbox Game Bar recording, which uses GPU encoding and video memory.", false,
		[]regEntry{
			{`HKCU\System\GameConfigStore`, "GameDVR_Enabled", regDWORD, "0"},
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", regDWORD, "0"},
		}},
	{"bgapps", "Block background apps", "Stops Store apps from running in the background while you play.", false,
		[]regEntry{
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications`, "GlobalUserDisabled", regDWORD, "1"},
		}},
	{"priority", "Prioritise games", "Raises game and foreground app priority in the Windows scheduler.", false,
		[]regEntry{
			{mm, "SystemResponsiveness", regDWORD, "10"},
			{mm + `\Tasks\Games`, "GPU Priority", regDWORD, "8"},
			{mm + `\Tasks\Games`, "Priority", regDWORD, "6"},
			{mm + `\Tasks\Games`, "Scheduling Category", regSZ, "High"},
			{mm + `\Tasks\Games`, "SFIO Priority", regSZ, "High"},
			{`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", regDWORD, "38"},
		}},
	{"visuals", "Lighter visuals", "Turns off transparency and animations. Some changes apply after sign-out.", false,
		[]regEntry{
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "EnableTransparency", regDWORD, "0"},
			{`HKCU\Control Panel\Desktop\WindowMetrics`, "MinAnimate", regSZ, "0"},
			{`HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "TaskbarAnimations", regDWORD, "0"},
		}},
	{"hags", "GPU scheduling (restart)", "GTX 10-series or newer. Needs a restart. Gains vary, so revert if none.", true,
		[]regEntry{
			{`HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode", regDWORD, "2"},
		}},
}

func tweakApplied(t tweak) bool {
	for _, e := range t.Entries {
		typ, v, ok := regRead(e.Key, e.Name)
		if !ok || typ != e.Type || !strings.EqualFold(v, e.Val) {
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
	if typ, v, ok := regRead(e.Key, e.Name); ok {
		state.Reg[k] = strconv.Itoa(int(typ)) + "|" + v
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
	if orig == absent {
		regDelete(e.Key, e.Name)
	} else {
		parts := strings.SplitN(orig, "|", 2)
		t, _ := strconv.Atoi(parts[0])
		if err := regWrite(e.Key, e.Name, uint32(t), parts[1]); err != nil {
			return err
		}
	}
	delete(state.Reg, k)
	return nil
}

func setTweak(id string, enable bool) error {
	for _, t := range tweaks {
		if t.ID != id {
			continue
		}
		for _, e := range t.Entries {
			if enable {
				backupReg(e)
				if err := regWrite(e.Key, e.Name, e.Type, e.Val); err != nil {
					saveState()
					return err
				}
			} else if err := restoreReg(e); err != nil {
				return err
			}
		}
		saveState()
		return nil
	}
	return fmt.Errorf("unknown setting")
}

// ---------- power plan ----------

var guidRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func activePlan() string {
	out, _ := run("powercfg", "/getactivescheme")
	return strings.ToLower(guidRe.FindString(out))
}

func planOn() bool {
	g := activePlan()
	return g == planHigh || g == planUltimate || (state.PlanGUID != "" && g != "" && g != state.PlanGUID)
}

func setHighPerformance(enable bool) error {
	cur := activePlan()
	if enable {
		if cur == planHigh || cur == planUltimate {
			return nil
		}
		if state.PlanGUID == "" {
			state.PlanGUID = cur
		}
		if _, err := run("powercfg", "/setactive", planHigh); err != nil {
			out, derr := run("powercfg", "-duplicatescheme", planHigh)
			g := strings.ToLower(guidRe.FindString(out))
			if derr != nil || g == "" {
				return fmt.Errorf("could not create the High performance plan")
			}
			if _, err := run("powercfg", "/setactive", g); err != nil {
				return fmt.Errorf("could not activate the High performance plan")
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

// ---------- background services (paused until restart or turned off) ----------

var boostServices = []string{"SysMain", "WSearch", "DiagTrack", "DoSvc"}

func svcRunning(name string) bool {
	out, _ := run("sc", "query", name)
	return strings.Contains(out, "RUNNING")
}

func servicesPaused() bool {
	if len(state.Stopped) == 0 {
		return false
	}
	for _, s := range state.Stopped {
		if !svcRunning(s) {
			return true
		}
	}
	state.Stopped = nil // they were restarted (for example by a reboot)
	saveState()
	return false
}

func setServices(enable bool) error {
	if enable {
		stopped := 0
		for _, s := range boostServices {
			if !svcRunning(s) {
				continue
			}
			if _, err := run("sc", "stop", s); err == nil {
				stopped++
				found := false
				for _, x := range state.Stopped {
					found = found || x == s
				}
				if !found {
					state.Stopped = append(state.Stopped, s)
				}
			}
		}
		saveState()
		if stopped == 0 && len(state.Stopped) == 0 {
			return fmt.Errorf("no background services could be paused")
		}
		return nil
	}
	for _, s := range state.Stopped {
		run("sc", "start", s)
	}
	state.Stopped = nil
	saveState()
	return nil
}

// ---------- GPU ----------

var smiFields = []string{"name", "driver_version", "temperature.gpu", "utilization.gpu",
	"power.draw", "power.limit", "power.default_limit", "power.min_limit", "power.max_limit",
	"clocks.gr", "clocks.max.gr", "fan.speed", "memory.used", "memory.total"}

func gpuInfo() (map[string]string, string) {
	if demoMode {
		return demoGPU(), ""
	}
	p := smiPath()
	if p == "" {
		return nil, "NVIDIA driver tools not found. Install or repair your NVIDIA driver."
	}
	out, err := run(p, "-i", "0", "--query-gpu="+strings.Join(smiFields, ","), "--format=csv,noheader,nounits")
	if err != nil {
		return nil, "Could not talk to the NVIDIA driver: " + firstLine(out)
	}
	parts := strings.Split(firstLine(out), ",")
	if len(parts) < len(smiFields) {
		return nil, "Unexpected response from the NVIDIA driver."
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
		return fmt.Errorf("this GPU does not allow power limit changes (common on laptops and some older cards)")
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
	if demoMode {
		return nil
	}
	out, err := run(smiPath(), "-i", "0", "-pl", strconv.Itoa(int(w+0.5)))
	if err != nil {
		return fmt.Errorf("the driver refused the power limit: %s", firstLine(out))
	}
	if pct == 100 {
		state.PowerLimit = ""
	}
	saveState()
	return nil
}

// ---------- high-level actions ----------

type stateSnap struct {
	Plan, Svc bool
	Tweaks    map[string]bool
	Ratio     float64
	HasPL     bool
}

func gatherState() stateSnap {
	mu.Lock()
	defer mu.Unlock()
	s := stateSnap{Tweaks: map[string]bool{}}
	for _, t := range tweaks {
		s.Tweaks[t.ID] = tweakApplied(t)
	}
	s.Plan = planOn()
	s.Svc = servicesPaused()
	if g, _ := gpuInfo(); g != nil {
		pl, e1 := strconv.ParseFloat(g["power.limit"], 64)
		def, e2 := strconv.ParseFloat(g["power.default_limit"], 64)
		if e1 == nil && e2 == nil && def > 0 {
			s.Ratio, s.HasPL = pl/def, true
		}
	}
	return s
}

// the set applied by "Optimise now"
var recommended = []string{"plan", "gamemode", "dvr", "bgapps", "priority", "svc"}

func applyItem(kind string, enable bool) error {
	switch kind {
	case "plan":
		return setHighPerformance(enable)
	case "svc":
		return setServices(enable)
	}
	return setTweak(kind, enable)
}

func optimiseNow() error {
	var errs []string
	for _, k := range recommended {
		if err := applyItem(k, true); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func restoreAll() error {
	var errs []string
	for _, t := range tweaks {
		for _, e := range t.Entries {
			if err := restoreReg(e); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	setHighPerformance(false)
	setServices(false)
	if state.PowerLimit != "" {
		if err := setPowerPercent(100); err != nil {
			errs = append(errs, err.Error())
		}
	}
	state = State{Reg: map[string]string{}}
	saveState()
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
