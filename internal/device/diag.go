package device

import (
	"fmt"
	"strconv"
	"strings"
)

// BridgeDiag is one parsed SerialBridge status line. SerialBridge (velo-platform
// v1.5.0+) logs one every 5 seconds under logcat tag "SerialBridge" at INFO:
//
//	DIAG port=<dev>@<baud> open=<0|1> bytes=<n> frames=<n> crc=<n> lastFrameAgeMs=<ms|-1> fw=<n|-1> fwName=<name> holders=<pid:cmdline,...|none>
//
// Keep this in sync with the bridge side of the contract.
type BridgeDiag struct {
	Port           string
	Open           bool
	Bytes          int64
	Frames         int64
	CRC            int64
	LastFrameAgeMs int64
	FW             int64
	FWName         string
	Holders        string
}

// diagKeys is the field order of a DIAG line. Values are delimited by the
// next key rather than by spaces so a value containing spaces (e.g. a
// holder's cmdline) can't shift the other fields.
var diagKeys = []string{"port", "open", "bytes", "frames", "crc", "lastFrameAgeMs", "fw", "fwName", "holders"}

// ParseDiagLine extracts a BridgeDiag from a logcat line. It accepts the raw
// message or a full logcat line with any prefix (timestamp, pid, tag).
func ParseDiagLine(line string) (BridgeDiag, bool) {
	var d BridgeDiag
	start := strings.Index(line, "DIAG port=")
	if start < 0 {
		return d, false
	}
	msg := strings.TrimRight(line[start+len("DIAG"):], "\r\n")

	vals := map[string]string{}
	pos := 0
	for i, key := range diagKeys {
		marker := " " + key + "="
		idx := strings.Index(msg[pos:], marker)
		if idx < 0 {
			continue
		}
		valStart := pos + idx + len(marker)
		valEnd := len(msg)
		for _, next := range diagKeys[i+1:] {
			if j := strings.Index(msg[valStart:], " "+next+"="); j >= 0 {
				valEnd = valStart + j
				break
			}
		}
		vals[key] = strings.TrimSpace(msg[valStart:valEnd])
		pos = valEnd
	}

	num := func(key string, def int64) (int64, bool) {
		v, ok := vals[key]
		if !ok {
			return def, true
		}
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	}

	for _, required := range []string{"port", "open", "bytes", "frames"} {
		if _, ok := vals[required]; !ok {
			return d, false
		}
	}
	ok := true
	var good bool
	d.Port = vals["port"]
	d.Open = vals["open"] == "1"
	if d.Bytes, good = num("bytes", 0); !good {
		ok = false
	}
	if d.Frames, good = num("frames", 0); !good {
		ok = false
	}
	if d.CRC, good = num("crc", 0); !good {
		ok = false
	}
	if d.LastFrameAgeMs, good = num("lastFrameAgeMs", -1); !good {
		ok = false
	}
	if d.FW, good = num("fw", -1); !good {
		ok = false
	}
	d.FWName = vals["fwName"]
	if d.FWName == "" {
		d.FWName = "Unknown"
	}
	d.Holders = vals["holders"]
	if d.Holders == "" {
		d.Holders = "none"
	}
	return d, ok
}

// latestDiag returns the newest DIAG line in logcat output. With
// "logcat -v epoch" each line starts with a seconds.millis timestamp; lines
// older than since are ignored so a previous bridge instance (e.g. before an
// update reinstalled it) doesn't produce a stale verdict. since < 0 disables
// the filter, and lines whose timestamp can't be parsed are accepted.
func latestDiag(logcat string, since float64) (BridgeDiag, bool) {
	var best BridgeDiag
	found := false
	for _, line := range strings.Split(logcat, "\n") {
		if !strings.Contains(line, " DIAG ") && !strings.HasPrefix(strings.TrimSpace(line), "DIAG ") {
			continue
		}
		if since >= 0 {
			if f := strings.Fields(line); len(f) > 0 {
				if ts, err := strconv.ParseFloat(f[0], 64); err == nil && ts < since {
					continue
				}
			}
		}
		if d, ok := ParseDiagLine(line); ok {
			best = d
			found = true
		}
	}
	return best, found
}

// reportDiag logs a plain-language verdict for a DIAG line. Never fatal.
func reportDiag(d BridgeDiag, log Logger) {
	log.Dim(fmt.Sprintf("  Bridge status: port=%s open=%t bytes=%d frames=%d crc=%d fw=%s",
		d.Port, d.Open, d.Bytes, d.Frames, d.CRC, d.FWName))

	switch {
	case d.Frames > 0:
		log.Success(fmt.Sprintf("  Bike sensors: talking (fw=%s)", d.FWName))
	case !d.Open:
		log.Warn(fmt.Sprintf("  SerialBridge could not open the serial port (%s)", d.Port))
		log.Warn("  Try rebooting the bike. If it persists, see Troubleshooting in the README.")
	case d.Bytes == 0:
		log.Info("  Serial port open but NO data from bike yet — this is normal until a ride starts.")
		log.Dim("  The bike only streams sensor data once a ride is running. If RPM/power")
		log.Dim("  stay at zero DURING a ride, see 'Sensors show zero' in the README.")
	case d.CRC > 0:
		log.Warn(fmt.Sprintf("  Data received but corrupted — another app may share the port (holders=%s)", d.Holders))
	default:
		log.Dim("  Data arriving from the bike, waiting for the first complete frame")
	}

	if d.Holders != "none" {
		log.Warn(fmt.Sprintf("  Other processes have the serial port open: %s", d.Holders))
		log.Warn("  They can steal or corrupt sensor data. Re-run Jailbreak or reboot the bike.")
	}
}
