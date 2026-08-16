package adb

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Invoke maps a unified operation to fake-ADB actions and returns a
// normalized result with Evidence. Raw errors are translated to domain.Error
// so protocol text never leaks into Core (Design doc 12 §18).
func (p *Plugin) Invoke(_ context.Context, channel *domain.Channel, req sdk.InvokeRequest) (*sdk.InvokeResult, error) {
	dev := p.device(channel.ID)
	if dev == nil {
		return nil, sdk.ConnectionError(domain.CodeChannelLost, "channel not bound")
	}

	switch req.Capability {
	case domain.CapabilityInfoGet:
		info, err := dev.Info()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output: formatKV(info),
			Evidence: []domain.Evidence{
				*evidenceFor(req, "info.model", info["model"]),
			},
		}, nil

	case domain.CapabilityReboot:
		if err := dev.Reboot(); err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output: "rebooted",
			Evidence: []domain.Evidence{
				*evidenceFor(req, "reboot.state", "system"),
			},
		}, nil

	case domain.CapabilityFlash:
		partition := req.Parameters["partition"]
		image := req.Parameters["image"]
		version := req.Parameters["version"]
		if err := dev.Flash(partition, image, version); err != nil {
			return nil, sdk.Error(domain.CodeInvalidInput, domain.CategoryValidation, err.Error())
		}
		// Postcondition evidence: the partition version must match.
		got := dev.PartitionVersion(partition)
		// A durable flash report artifact accompanies the result.
		report := domain.NewArtifact("flash-report")
		report.ContentType = "text/plain"
		report.Content = []byte("partition=" + partition + "\nversion=" + got + "\nimage=" + image + "\n")
		return &sdk.InvokeResult{
			Output: "flashed " + partition,
			Evidence: []domain.Evidence{
				*evidenceFor(req, "flash.partition", partition),
				*evidenceFor(req, "flash.version", got),
			},
			Artifacts: []domain.Artifact{*report},
		}, nil

	case domain.CapabilityFileRead:
		path := req.Parameters["path"]
		content, err := dev.ReadFile(path)
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{Output: content}, nil

	case domain.CapabilityLog:
		out, err := dev.Log()
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{
			Output:   out,
			Evidence: []domain.Evidence{*evidenceFor(req, "log.source", "adb")},
		}, nil

	case domain.CapabilityExecute:
		out, err := dev.Execute(req.Parameters["command"])
		if err != nil {
			return nil, mapErr(err)
		}
		return &sdk.InvokeResult{Output: out}, nil

	default:
		return nil, sdk.Error(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"unsupported capability "+string(req.Capability))
	}
}

// mapErr translates fake-device transport errors into the unified taxonomy.
func mapErr(err error) *domain.Error {
	if errors.Is(err, fake.ErrOffline) {
		return sdk.DeviceStateError(domain.CodeDeviceOffline, "device offline")
	}
	return sdk.ProtocolError(domain.CodeInternal, err.Error(), err.Error())
}

// formatKV renders a map deterministically as "k=v" lines.
func formatKV(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(m[k])
		b.WriteString("\n")
	}
	return b.String()
}

func evidenceFor(req sdk.InvokeRequest, name, value string) *domain.Evidence {
	ev := domain.NewEvidence(name, value)
	ev.OperationID = req.OperationID
	return ev
}
