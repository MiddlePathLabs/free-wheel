// Command apkcert checks that APKs are signed with an expected certificate.
//
// Usage: go run ./tools/apkcert <sha256-of-cert> file.apk...
//
// It reads the signer certificate from the APK Signature Scheme v2/v3 block
// (what Android 9 checks for io.freewheel.bridge's sharedUserId=system) and
// compares its SHA-256 digest; it does not verify the signature itself,
// Android does that at install time. Exits 1 on any mismatch or error.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	eocdSize      = 22
	sigBlockMagic = "APK Sig Block 42"
	idV2          = 0x7109871a
	idV3          = 0xf05368c0
	idV31         = 0x1b93ad61
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: apkcert <sha256-of-cert> file.apk...")
		os.Exit(2)
	}
	want := normalize(os.Args[1])
	ok := true
	for _, path := range os.Args[2:] {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			ok = false
			continue
		}
		cert, err := signerCert(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			ok = false
			continue
		}
		sum := sha256.Sum256(cert)
		got := hex.EncodeToString(sum[:])
		if got != want {
			fmt.Fprintf(os.Stderr, "%s: signed with %s, expected %s\n", path, got, want)
			ok = false
			continue
		}
		fmt.Printf("%s: signer certificate SHA-256 %s OK\n", path, got)
	}
	if !ok {
		os.Exit(1)
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.NewReplacer(":", "", " ", "").Replace(s))
}

// signerCert returns the first signer's first certificate (DER) from the
// APK Signing Block, preferring v3.1, then v3, then v2.
func signerCert(apk []byte) ([]byte, error) {
	block, err := signingBlock(apk)
	if err != nil {
		return nil, err
	}
	pairs := map[uint32][]byte{}
	for len(block) > 0 {
		if len(block) < 8 {
			return nil, errors.New("truncated signing block entry")
		}
		n := binary.LittleEndian.Uint64(block)
		block = block[8:]
		if n < 4 || n > uint64(len(block)) {
			return nil, errors.New("bad signing block entry length")
		}
		pairs[binary.LittleEndian.Uint32(block)] = block[4:n]
		block = block[n:]
	}
	for _, id := range []uint32{idV31, idV3, idV2} {
		if v, ok := pairs[id]; ok {
			return firstCert(v)
		}
	}
	return nil, errors.New("no APK Signature Scheme v2/v3 signature")
}

// signingBlock returns the id-value pairs section of the APK Signing Block.
func signingBlock(apk []byte) ([]byte, error) {
	eocd := bytes.LastIndex(apk, []byte{0x50, 0x4b, 0x05, 0x06})
	if eocd < 0 || len(apk)-eocd < eocdSize {
		return nil, errors.New("not a zip file (no end of central directory)")
	}
	cdOffset := uint64(binary.LittleEndian.Uint32(apk[eocd+16:]))
	if cdOffset < 24 || cdOffset > uint64(eocd) {
		return nil, errors.New("bad central directory offset")
	}
	footer := apk[cdOffset-24 : cdOffset]
	if string(footer[8:]) != sigBlockMagic {
		return nil, errors.New("APK is not signed with APK Signature Scheme v2/v3")
	}
	size := binary.LittleEndian.Uint64(footer)
	// The block is: size(8) | pairs | size(8) | magic(16); size excludes the first field.
	if size < 24 || size+8 > cdOffset {
		return nil, errors.New("bad signing block size")
	}
	start := cdOffset - size - 8
	if binary.LittleEndian.Uint64(apk[start:]) != size {
		return nil, errors.New("signing block size mismatch")
	}
	return apk[start+8 : cdOffset-24], nil
}

// lp splits off one uint32-length-prefixed value.
func lp(b []byte) (val, rest []byte, err error) {
	if len(b) < 4 {
		return nil, nil, errors.New("truncated length prefix")
	}
	n := binary.LittleEndian.Uint32(b)
	if uint64(n) > uint64(len(b)-4) {
		return nil, nil, errors.New("length prefix out of range")
	}
	return b[4 : 4+n], b[4+n:], nil
}

// firstCert walks signers -> signer -> signed data -> (digests, certificates)
// -> certificate; the layout is the same for v2 and v3.
func firstCert(scheme []byte) ([]byte, error) {
	signers, _, err := lp(scheme)
	if err != nil {
		return nil, err
	}
	signer, _, err := lp(signers)
	if err != nil {
		return nil, err
	}
	signedData, _, err := lp(signer)
	if err != nil {
		return nil, err
	}
	_, rest, err := lp(signedData) // digests
	if err != nil {
		return nil, err
	}
	certs, _, err := lp(rest)
	if err != nil {
		return nil, err
	}
	cert, _, err := lp(certs)
	if err != nil {
		return nil, err
	}
	if len(cert) == 0 {
		return nil, errors.New("empty signer certificate")
	}
	return cert, nil
}
