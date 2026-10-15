package device

import (
	"strings"
	"testing"
)

func TestParseDiagLine(t *testing.T) {
	line := "1760500000.123  1234  1250 I SerialBridge: DIAG port=/dev/ttyS4@230400 open=1 bytes=5120 frames=42 crc=0 lastFrameAgeMs=180 fw=3 fwName=APP holders=none"
	d, ok := ParseDiagLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	want := BridgeDiag{Port: "/dev/ttyS4@230400", Open: true, Bytes: 5120, Frames: 42, CRC: 0,
		LastFrameAgeMs: 180, FW: 3, FWName: "APP", Holders: "none"}
	if d != want {
		t.Fatalf("got %+v, want %+v", d, want)
	}
}

func TestParseDiagLine_HoldersWithSpaces(t *testing.T) {
	line := "DIAG port=/dev/ttyS4@230400 open=1 bytes=10 frames=0 crc=7 lastFrameAgeMs=-1 fw=-1 fwName=Unknown holders=812:com.nautilus.bowflex.usb,900:/system/bin/foo --flag"
	d, ok := ParseDiagLine(line)
	if !ok {
		t.Fatal("expected line to parse")
	}
	if d.Holders != "812:com.nautilus.bowflex.usb,900:/system/bin/foo --flag" {
		t.Fatalf("holders = %q", d.Holders)
	}
	if d.CRC != 7 || d.LastFrameAgeMs != -1 || d.FW != -1 || d.FWName != "Unknown" {
		t.Fatalf("unexpected fields: %+v", d)
	}
}

func TestParseDiagLine_Rejects(t *testing.T) {
	for _, line := range []string{
		"",
		"I SerialBridge: Bridge started",
		"DIAG port=/dev/ttyS4@230400 open=1", // missing bytes/frames
		"DIAG port=/dev/ttyS4@230400 open=1 bytes=abc frames=1 crc=0", // bad number
	} {
		if _, ok := ParseDiagLine(line); ok {
			t.Errorf("expected %q to be rejected", line)
		}
	}
}

func TestLatestDiag_SinceFilter(t *testing.T) {
	out := strings.Join([]string{
		"100.000  1  1 I SerialBridge: DIAG port=/dev/ttyS4@230400 open=1 bytes=1 frames=1 crc=0 lastFrameAgeMs=1 fw=1 fwName=OLD holders=none",
		"205.500  2  2 I SerialBridge: DIAG port=/dev/ttyS4@230400 open=1 bytes=0 frames=0 crc=0 lastFrameAgeMs=-1 fw=-1 fwName=Unknown holders=none",
		"210.500  2  2 I SerialBridge: DIAG port=/dev/ttyS4@230400 open=1 bytes=9 frames=3 crc=0 lastFrameAgeMs=5 fw=2 fwName=NEW holders=none",
	}, "\n")

	d, ok := latestDiag(out, 200)
	if !ok || d.FWName != "NEW" {
		t.Fatalf("got %+v ok=%v, want newest line", d, ok)
	}
	if _, ok := latestDiag(out, 300); ok {
		t.Fatal("expected no line newer than since")
	}
	if d, ok := latestDiag(out, -1); !ok || d.FWName != "NEW" {
		t.Fatalf("no filter: got %+v ok=%v", d, ok)
	}
}

type recLogger struct{ lines []string }

func (r *recLogger) add(l, m string)   { r.lines = append(r.lines, l+": "+m) }
func (r *recLogger) Info(m string)     { r.add("info", m) }
func (r *recLogger) Success(m string)  { r.add("success", m) }
func (r *recLogger) Error(m string)    { r.add("error", m) }
func (r *recLogger) Warn(m string)     { r.add("warn", m) }
func (r *recLogger) Step(m string)     { r.add("step", m) }
func (r *recLogger) Dim(m string)      { r.add("dim", m) }
func (r *recLogger) has(s string) bool { return strings.Contains(strings.Join(r.lines, "\n"), s) }

func TestReportDiag_Verdicts(t *testing.T) {
	cases := []struct {
		name string
		d    BridgeDiag
		want string
	}{
		{"talking", BridgeDiag{Open: true, Bytes: 10, Frames: 2, FWName: "APP", Holders: "none"}, "Bike sensors: talking (fw=APP)"},
		{"idle", BridgeDiag{Open: true, Holders: "none"}, "NO data from bike yet"},
		{"corrupt", BridgeDiag{Open: true, Bytes: 50, CRC: 4, Holders: "812:x"}, "corrupted — another app may share the port (holders=812:x)"},
		{"closed", BridgeDiag{Port: "/dev/ttyS4@230400", Holders: "none"}, "could not open the serial port"},
	}
	for _, c := range cases {
		r := &recLogger{}
		reportDiag(c.d, r)
		if !r.has(c.want) {
			t.Errorf("%s: missing %q in %v", c.name, c.want, r.lines)
		}
	}

	r := &recLogger{}
	reportDiag(BridgeDiag{Open: true, Bytes: 1, Frames: 1, Holders: "812:x"}, r)
	if !r.has("warn: ") || !r.has("812:x") {
		t.Errorf("expected holders warning, got %v", r.lines)
	}
}
