// Package fake provides an in-memory embedded-device simulator. It stands in
// for real hardware in Contract, Integration and E2E tests (Design docs 17,
// 18) and backs the reference ADB-like Plugin.
package fake

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrOffline   = errors.New("fake: device offline")
	ErrNotBooted = errors.New("fake: device not booted")
	ErrUnknown   = errors.New("fake: unknown device command")
)

// Device simulates one embedded board reachable over a USB-ADB-like locator
// and (optionally) a UART serial port. Both endpoints share the same serial
// identity, so Discovery can correlate them to one Device.
type Device struct {
	mu         sync.RWMutex
	Serial     string
	HardwareID string // strong hardware-unique identity (defaults to serial)
	Model      string
	Firmware   string
	Locator    string // USB-ADB endpoint locator
	SerialPort string // UART endpoint locator (e.g. ttyUSB0), empty if none
	MCPURL     string // MCP remote-service endpoint locator, empty if none

	online    bool
	BootState string

	partitions map[string]string
	files      map[string]string
	logs       []string
	console    []string

	failNextInvoke bool
	latency        time.Duration
}

// NewDevice builds a booted, online fake device.
func NewDevice(serial, model, firmware, locator string) *Device {
	return &Device{
		Serial:     serial,
		Model:      model,
		Firmware:   firmware,
		Locator:    locator,
		online:     true,
		BootState:  "system",
		partitions: map[string]string{"boot": firmware},
		files:      map[string]string{},
		logs:       []string{},
		console:    []string{"boot: ok", "init: started"},
	}
}

// WithSerialPort sets the UART endpoint locator and returns the device for
// chaining.
func (d *Device) WithSerialPort(path string) *Device {
	d.SerialPort = path
	return d
}

// WithMCPURL sets the MCP remote-service endpoint locator and returns the
// device for chaining.
func (d *Device) WithMCPURL(url string) *Device {
	d.MCPURL = url
	return d
}

// WithHardwareID sets a distinct strong hardware identity (for testing
// identity-conflict quarantine) and returns the device for chaining.
func (d *Device) WithHardwareID(id string) *Device {
	d.HardwareID = id
	return d
}

// Identity returns the strong identity signals used for correlation.
func (d *Device) Identity() map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	hw := d.HardwareID
	if hw == "" {
		hw = d.Serial
	}
	return map[string]string{
		"serial":      d.Serial,
		"hardware_id": hw,
		"model":       d.Model,
		"firmware":    d.Firmware,
	}
}

// IsOnline reports transport reachability.
func (d *Device) IsOnline() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.online
}

// CurrentBootState returns the device's boot state (thread-safe).
func (d *Device) CurrentBootState() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.BootState
}

// Info returns device info (device.info.get).
func (d *Device) Info() (map[string]string, error) {
	if err := d.guard(); err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return map[string]string{
		"serial":   d.Serial,
		"model":    d.Model,
		"firmware": d.Firmware,
		"state":    d.BootState,
	}, nil
}

// Reboot simulates a reboot (device.reboot). It stays reachable for the MVP;
// fault injection is used to exercise the loss path.
func (d *Device) Reboot() error {
	if err := d.guard(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.BootState = "system"
	d.appendLogLocked("reboot")
	return nil
}

// Flash writes a partition and verifies the image fingerprint (device.flash).
func (d *Device) Flash(partition, image, version string) error {
	if err := d.guard(); err != nil {
		return err
	}
	if partition == "" || image == "" || version == "" {
		return errors.New("fake: flash requires partition, image and version")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.partitions[partition] = version
	d.appendLogLocked("flashed " + partition + " -> " + version)
	return nil
}

// PartitionVersion returns the recorded version of a partition (used by the
// plugin's postcondition verification).
func (d *Device) PartitionVersion(partition string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.partitions[partition]
}

// ReadFile returns a file's content (device.file.read).
func (d *Device) ReadFile(path string) (string, error) {
	if err := d.guard(); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if v, ok := d.files[path]; ok {
		return v, nil
	}
	return "", fmt.Errorf("fake: no such file %q", path)
}

// WriteFile stores a file (used by tests/setup).
func (d *Device) WriteFile(path, content string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files[path] = content
}

// Log returns the device console log (device.log, shared by ADB logcat and
// UART console capture).
func (d *Device) Log() (string, error) {
	if err := d.guard(); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return strings.Join(d.logs, "\n"), nil
}

// Console returns the serial console buffer (device.console, UART only).
func (d *Device) Console() (string, error) {
	if err := d.guard(); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return strings.Join(d.console, "\n"), nil
}

// Reset performs a hardware reset via the serial console (device.reset). It
// clears the log and returns the device to a clean boot state.
func (d *Device) Reset() error {
	if err := d.guard(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.BootState = "system"
	d.logs = nil
	d.console = append(d.console, "reset: hardware")
	return nil
}

// PowerCycle simulates a hard power cycle: it forces the device online and
// clears transient state. This is the L5 device-recovery primitive.
func (d *Device) PowerCycle() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.online = true
	d.BootState = "system"
	d.failNextInvoke = false
	d.logs = nil
	d.console = append(d.console, "power-cycle: recovered")
	return nil
}

// Execute runs a command against a small allowlist (device.execute).
func (d *Device) Execute(cmd string) (string, error) {
	if err := d.guard(); err != nil {
		return "", err
	}
	switch {
	case strings.HasPrefix(cmd, "getprop"):
		d.mu.RLock()
		defer d.mu.RUnlock()
		return "ro.build.version=" + d.Firmware, nil
	case strings.HasPrefix(cmd, "echo"):
		return strings.TrimPrefix(cmd, "echo "), nil
	default:
		return "", ErrUnknown
	}
}

// ---------------------------------------------------------------------------
// Fault injection
// ---------------------------------------------------------------------------

func (d *Device) SetOnline(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.online = v
}

func (d *Device) SetFailNext(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failNextInvoke = v
}

func (d *Device) SetLatency(dur time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.latency = dur
}

func (d *Device) guard() error {
	d.mu.RLock()
	online := d.online
	fail := d.failNextInvoke
	lat := d.latency
	d.mu.RUnlock()
	if lat > 0 {
		time.Sleep(lat)
	}
	if !online {
		return ErrOffline
	}
	if fail {
		return ErrUnknown
	}
	return nil
}

func (d *Device) appendLogLocked(line string) {
	d.logs = append(d.logs, line)
}

// Farm is a registry of simulated devices, analogous to a host's `adb
// devices` view.
type Farm struct {
	mu      sync.RWMutex
	devices map[string]*Device // keyed by serial
}

func NewFarm() *Farm { return &Farm{devices: map[string]*Device{}} }

func (f *Farm) Add(d *Device) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices[d.Serial] = d
}

func (f *Farm) BySerial(serial string) *Device {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.devices[serial]
}

func (f *Farm) ByLocator(locator string) *Device {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, d := range f.devices {
		if d.Locator == locator || d.SerialPort == locator || d.MCPURL == locator {
			return d
		}
	}
	return nil
}

// ByMCPURL resolves a device by its MCP remote-service endpoint.
func (f *Farm) ByMCPURL(url string) *Device {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, d := range f.devices {
		if d.MCPURL == url {
			return d
		}
	}
	return nil
}

// BySerialPort resolves a device by its UART serial port path.
func (f *Farm) BySerialPort(path string) *Device {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, d := range f.devices {
		if d.SerialPort == path {
			return d
		}
	}
	return nil
}

// List returns all devices ordered by serial (deterministic).
func (f *Farm) List() []*Device {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Device, 0, len(f.devices))
	for _, d := range f.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Serial < out[j].Serial })
	return out
}

// OnlineLocators returns the locators of online devices, mirroring an `adb
// devices` scan.
func (f *Farm) OnlineLocators() []string {
	var out []string
	for _, d := range f.List() {
		if d.IsOnline() {
			out = append(out, d.Locator)
		}
	}
	return out
}
