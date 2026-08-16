package sdk

// ConsoleDevice is the transport-level device interface a console/serial
// plugin drives. Both the in-memory fake simulator and a real serial port
// implement it, so the UART plugin is transport-agnostic: swapping a fake
// device for a real one requires no plugin change.
type ConsoleDevice interface {
	// Identity returns strong identity signals for correlation.
	Identity() map[string]string
	// IsOnline reports transport reachability.
	IsOnline() bool
	// CurrentBootState returns the device boot state (e.g. "uboot", "linux").
	CurrentBootState() string
	// Console reads the console buffer / current console output.
	Console() (string, error)
	// Log reads the device log stream.
	Log() (string, error)
	// Execute sends a command and returns the response (the console command
	// primitive; for U-Boot this is a bootloader command).
	Execute(cmd string) (string, error)
	// Reset issues a device reset.
	Reset() error
	// PowerCycle performs a hard power cycle (L5 device recovery).
	PowerCycle() error
}
