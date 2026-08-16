// Package runtime assembles the whole Connectivity Core from its components
// (Design doc 13 §38 startup flow). It is the composition root: the only
// place that wires concrete Plugins, Registries, Managers and the SDK client
// together.
package runtime

import (
	"context"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/artifact"
	"github.com/1daerduo/hardware-channel-abstraction/core/discovery"
	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/core/operation"
	"github.com/1daerduo/hardware-channel-abstraction/core/recovery"
	"github.com/1daerduo/hardware-channel-abstraction/core/registry"
	"github.com/1daerduo/hardware-channel-abstraction/core/resolver"
	"github.com/1daerduo/hardware-channel-abstraction/core/resource"
	"github.com/1daerduo/hardware-channel-abstraction/core/security"
	"github.com/1daerduo/hardware-channel-abstraction/core/session"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/adb"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/mcp"
	pluginregistry "github.com/1daerduo/hardware-channel-abstraction/plugin/registry"
	pluginsdk "github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
	plugintcp "github.com/1daerduo/hardware-channel-abstraction/plugin/tcp"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/uart"
	"github.com/1daerduo/hardware-channel-abstraction/sdk"
	serialtransport "github.com/1daerduo/hardware-channel-abstraction/transport/serial"
	transporttcp "github.com/1daerduo/hardware-channel-abstraction/transport/tcp"
)

// Runtime is the assembled system. It exposes the high-level client plus the
// lower-level components for inspection and testing.
type Runtime struct {
	Client        *sdk.Client
	Farm          *fake.Farm
	Plugins       *pluginregistry.Registry
	Registry      *registry.Registry
	Engine        *operation.Engine
	Bus           *event.Bus
	Policy        *security.Policy
	Approval      *security.ApprovalService
	Authenticator *security.Authenticator
	Secrets       *security.SecretStore
	Artifacts     *artifact.Service
	RealConsoles  []*serialtransport.Console
	RealTCPs      []*transporttcp.Console
}

// Close releases any real serial ports / TCP connections opened during
// Bootstrap.
func (rt *Runtime) Close() {
	for _, c := range rt.RealConsoles {
		_ = c.Close()
	}
	for _, c := range rt.RealTCPs {
		_ = c.Close()
	}
}

// Option configures a Runtime before assembly.
type Option func(*config)

type config struct {
	devices    []*fake.Device
	realSerial []serialConfig
	tcpAddrs   []string
	lockTTL    time.Duration
	maxRetries int
}

type serialConfig struct {
	path string
	baud int
}

// WithDevices registers fake devices at startup.
func WithDevices(devices ...*fake.Device) Option {
	return func(c *config) { c.devices = append(c.devices, devices...) }
}

// WithRealSerial opens a real serial port (e.g. /dev/ttyUSB0) as a UART
// console device at startup.
func WithRealSerial(path string, baud int) Option {
	return func(c *config) { c.realSerial = append(c.realSerial, serialConfig{path: path, baud: baud}) }
}

// WithTCPDevice dials a TCP console device at startup.
func WithTCPDevice(addr string) Option {
	return func(c *config) { c.tcpAddrs = append(c.tcpAddrs, addr) }
}

// WithLockTTL sets the resource lease TTL.
func WithLockTTL(ttl time.Duration) Option {
	return func(c *config) { c.lockTTL = ttl }
}

// Bootstrap assembles a Runtime with an ADB-like plugin backed by a fake
// device farm, following the startup flow of Design doc 13 §38.
func Bootstrap(opts ...Option) *Runtime {
	cfg := &config{lockTTL: 30 * time.Second, maxRetries: 1}
	for _, o := range opts {
		o(cfg)
	}

	farm := fake.NewFarm()
	for _, d := range cfg.devices {
		farm.Add(d)
	}

	// Open any real serial ports configured.
	var realConsoles []*serialtransport.Console
	realDevices := map[string]pluginsdk.ConsoleDevice{}
	for _, sc := range cfg.realSerial {
		con, err := serialtransport.Open(sc.path, sc.baud)
		if err != nil {
			continue // a missing/blocked port is non-fatal; fake devices still work
		}
		realConsoles = append(realConsoles, con)
		realDevices[con.Path()] = con
	}

	// Dial any TCP console devices configured.
	var realTCPs []*transporttcp.Console
	tcpDevices := map[string]pluginsdk.ConsoleDevice{}
	for _, addr := range cfg.tcpAddrs {
		con, err := transporttcp.Dial(addr)
		if err != nil {
			continue // a down/unreachable TCP device is non-fatal
		}
		realTCPs = append(realTCPs, con)
		tcpDevices[con.Path()] = con
	}

	plugins := pluginregistry.New()
	adbPlugin := adb.New(farm)
	uartPlugin := uart.NewWithResolver(uartResolver{farm: farm, real: realDevices})
	mcpPlugin := mcp.New(farm)
	tcpPlugin := plugintcp.NewWithResolver(tcpResolver{devices: tcpDevices})
	_ = plugins.Register(adbPlugin)
	_ = plugins.Register(uartPlugin)
	_ = plugins.Register(mcpPlugin)
	_ = plugins.Register(tcpPlugin)
	_ = plugins.Load(adb.PluginID)
	_ = plugins.Load(uart.PluginID)
	_ = plugins.Load(mcp.PluginID)
	_ = plugins.Load(plugintcp.PluginID)
	plugins.Ready(adb.PluginID)
	plugins.Ready(uart.PluginID)
	plugins.Ready(mcp.PluginID)
	plugins.Ready(plugintcp.PluginID)

	reg := registry.New()
	bus := event.New()

	disc := discovery.New(plugins, reg, bus)
	disc.AddScanner(farmScanner{farm: farm})
	if len(realConsoles) > 0 {
		disc.AddScanner(realSerialScanner{consoles: realConsoles})
	}
	if len(realTCPs) > 0 {
		disc.AddScanner(realTCPScanner{consoles: realTCPs})
	}

	res := resolver.New(reg)
	sessions := session.NewManager()
	resources := resource.NewRegistry()
	locks := resource.NewManager()

	policy := security.NewPolicy()
	approval := security.NewApprovalService()
	authenticator := security.NewAuthenticator()
	secrets := security.NewSecretStore()
	audit := security.NewAuditService(bus)
	recovBudget := recovery.DefaultBudget()
	recovBudget.MaxAttempts = cfg.maxRetries
	recov := recovery.NewManager(plugins, bus, recovBudget).WithResolver(res).WithDiscovery(disc)
	reconciler := recovery.NewReconciler()
	classifier := recovery.NewClassifier()
	artifactSvc := artifact.New()

	engine := operation.New(operation.Deps{
		Registry:   reg,
		Resolver:   res,
		Sessions:   sessions,
		Resources:  resources,
		Locks:      locks,
		Plugins:    plugins,
		Bus:        bus,
		Policy:     policy,
		Approval:   approval,
		Audit:      audit,
		Recovery:   recov,
		Reconciler: reconciler,
		Classifier: classifier,
		Artifacts:  artifactSvc,
		MaxRetries: cfg.maxRetries,
		LockTTL:    cfg.lockTTL,
	})

	client := sdk.New(sdk.Deps{
		Discovery:     disc,
		Engine:        engine,
		Registry:      reg,
		Resolver:      res,
		Plugins:       plugins,
		Sessions:      sessions,
		Bus:           bus,
		Policy:        policy,
		Approval:      approval,
		Authenticator: authenticator,
		Secrets:       secrets,
		Artifacts:     artifactSvc,
	})

	return &Runtime{
		Client:        client,
		Farm:          farm,
		Plugins:       plugins,
		Registry:      reg,
		Engine:        engine,
		Bus:           bus,
		Policy:        policy,
		Approval:      approval,
		Authenticator: authenticator,
		Secrets:       secrets,
		Artifacts:     artifactSvc,
		RealConsoles:  realConsoles,
		RealTCPs:      realTCPs,
	}
}

// tcpResolver resolves TCP addresses to console devices.
type tcpResolver struct {
	devices map[string]pluginsdk.ConsoleDevice
}

func (r tcpResolver) ByTCPAddr(addr string) pluginsdk.ConsoleDevice {
	return r.devices[addr]
}

// realTCPScanner emits a TCP endpoint per dialed console device.
type realTCPScanner struct{ consoles []*transporttcp.Console }

func (s realTCPScanner) Scan(_ context.Context) ([]domain.Endpoint, error) {
	var out []domain.Endpoint
	for _, con := range s.consoles {
		ep := domain.Endpoint{
			ID:        domain.NewEndpointID(),
			Type:      domain.EndpointTCP,
			Locator:   con.Path(),
			Transport: "tcp-ip",
			Source:    "real-tcp",
		}
		for k, v := range con.Identity() {
			ep.SetAttr(k, v)
		}
		out = append(out, ep)
	}
	return out, nil
}

// uartResolver resolves serial locators to fake or real console devices.
type uartResolver struct {
	farm *fake.Farm
	real map[string]pluginsdk.ConsoleDevice
}

func (r uartResolver) BySerialPort(locator string) pluginsdk.ConsoleDevice {
	if c, ok := r.real[locator]; ok {
		return c
	}
	d := r.farm.BySerialPort(locator)
	if d == nil {
		return nil
	}
	return d
}

// realSerialScanner emits a UART endpoint per opened real serial console.
type realSerialScanner struct{ consoles []*serialtransport.Console }

func (s realSerialScanner) Scan(_ context.Context) ([]domain.Endpoint, error) {
	var out []domain.Endpoint
	for _, con := range s.consoles {
		ep := domain.Endpoint{
			ID:        domain.NewEndpointID(),
			Type:      domain.EndpointUART,
			Locator:   con.Path(),
			Transport: "serial",
			Source:    "real-serial",
		}
		for k, v := range con.Identity() {
			ep.SetAttr(k, v)
		}
		out = append(out, ep)
	}
	return out, nil
}

// farmScanner adapts the fake farm into a discovery.Scanner.
type farmScanner struct{ farm *fake.Farm }

func (s farmScanner) Scan(_ context.Context) ([]domain.Endpoint, error) {
	var out []domain.Endpoint
	for _, d := range s.farm.List() {
		if !d.IsOnline() {
			continue
		}
		// USB-ADB endpoint.
		if d.Locator != "" {
			ep := domain.Endpoint{
				ID:        domain.NewEndpointID(),
				Type:      domain.EndpointUSBADB,
				Locator:   d.Locator,
				Transport: "usb",
				Source:    "fake-farm",
			}
			for k, v := range d.Identity() {
				ep.SetAttr(k, v)
			}
			out = append(out, ep)
		}
		// UART serial endpoint (if configured).
		if d.SerialPort != "" {
			ep := domain.Endpoint{
				ID:        domain.NewEndpointID(),
				Type:      domain.EndpointUART,
				Locator:   d.SerialPort,
				Transport: "serial",
				Source:    "fake-farm",
			}
			for k, v := range d.Identity() {
				ep.SetAttr(k, v)
			}
			out = append(out, ep)
		}
		// MCP remote-service endpoint (if configured).
		if d.MCPURL != "" {
			ep := domain.Endpoint{
				ID:        domain.NewEndpointID(),
				Type:      domain.EndpointMCP,
				Locator:   d.MCPURL,
				Transport: "http",
				Source:    "fake-farm",
			}
			for k, v := range d.Identity() {
				ep.SetAttr(k, v)
			}
			out = append(out, ep)
		}
	}
	return out, nil
}
