package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func u32(n int) []byte    { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, uint32(n)); return b }
func u64(n int) []byte    { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, uint64(n)); return b }
func lpw(b []byte) []byte { return append(u32(len(b)), b...) }

// fakeAPK builds: zip entries | signing block | (empty) central dir | EOCD.
func fakeAPK(id uint32, cert []byte) []byte {
	signedData := append(lpw(nil), lpw(lpw(cert))...) // digests, certificates
	scheme := lpw(lpw(lpw(signedData)))               // signers -> signer -> signed data
	pair := append(u32(int(id)), scheme...)
	pairs := append(u64(len(pair)), pair...)
	size := len(pairs) + 8 + 16
	var b bytes.Buffer
	b.WriteString("PK\x03\x04 fake entries")
	b.Write(u64(size))
	b.Write(pairs)
	b.Write(u64(size))
	b.WriteString(sigBlockMagic)
	cdOffset := b.Len()
	eocd := make([]byte, eocdSize)
	copy(eocd, []byte{0x50, 0x4b, 0x05, 0x06})
	binary.LittleEndian.PutUint32(eocd[16:], uint32(cdOffset))
	b.Write(eocd)
	return b.Bytes()
}

func TestSignerCert(t *testing.T) {
	for _, id := range []uint32{idV2, idV3} {
		got, err := signerCert(fakeAPK(id, []byte("CERT")))
		if err != nil || string(got) != "CERT" {
			t.Fatalf("id %x: got %q, %v", id, got, err)
		}
	}
}

func TestSignerCert_Rejects(t *testing.T) {
	if _, err := signerCert([]byte("not a zip")); err == nil {
		t.Error("expected error for non-zip")
	}
	if _, err := signerCert(fakeAPK(0x12345678, []byte("CERT"))); err == nil {
		t.Error("expected error without a v2/v3 block")
	}
	unsigned := fakeAPK(idV2, []byte("CERT"))
	copy(unsigned[bytes.Index(unsigned, []byte(sigBlockMagic)):], "APK Sig Block 00")
	if _, err := signerCert(unsigned); err == nil {
		t.Error("expected error without signing block magic")
	}
}

func TestNormalize(t *testing.T) {
	if normalize("2D:37:0C") != "2d370c" {
		t.Fatal(normalize("2D:37:0C"))
	}
}
