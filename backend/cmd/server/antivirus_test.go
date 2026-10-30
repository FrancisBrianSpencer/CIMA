package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"strings"
	"testing"
)

func TestScanDocumentWithClamAV(t *testing.T) {
	for _, test := range []struct {
		name       string
		verdict    string
		wantInfect bool
	}{
		{name: "clean", verdict: "stream: OK\x00"},
		{name: "infected", verdict: "stream: Eicar-Test-Signature FOUND\x00", wantInfect: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				defer connection.Close()
				reader := bufio.NewReader(connection)
				command, err := reader.ReadString('\x00')
				if err != nil || command != "zINSTREAM\x00" {
					return
				}
				for {
					var length [4]byte
					if _, err := io.ReadFull(reader, length[:]); err != nil {
						return
					}
					chunkSize := binary.BigEndian.Uint32(length[:])
					if chunkSize == 0 {
						break
					}
					if _, err := io.CopyN(io.Discard, reader, int64(chunkSize)); err != nil {
						return
					}
				}
				_, _ = io.WriteString(connection, test.verdict)
				_ = listener.Close()
			}()

			err = scanDocumentWithClamAV(context.Background(), listener.Addr().String(), []byte("test document"))
			if test.wantInfect && err != errDocumentMalwareDetected {
				t.Fatalf("scanDocumentWithClamAV() error = %v, want malware verdict", err)
			}
			if !test.wantInfect && err != nil {
				t.Fatalf("scanDocumentWithClamAV() unexpected error: %v", err)
			}
		})
	}
}

func TestScanDocumentWithClamAVRejectsInvalidInput(t *testing.T) {
	if err := scanDocumentWithClamAV(context.Background(), "", []byte("file")); err == nil {
		t.Fatal("scanDocumentWithClamAV() accepted an empty scanner address")
	}
	if err := scanDocumentWithClamAV(context.Background(), "127.0.0.1:1", nil); err == nil {
		t.Fatal("scanDocumentWithClamAV() accepted empty content")
	}
}

func TestClamAVDockerIntegration(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("CLAMAV_ADDRESS"))
	if address == "" {
		t.Skip("CLAMAV_ADDRESS is not set")
	}
	if err := scanDocumentWithClamAV(context.Background(), address, []byte("CIMA benign document scan check")); err != nil {
		t.Fatalf("clean document scan failed: %v", err)
	}
	eicar := []byte("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*")
	if err := scanDocumentWithClamAV(context.Background(), address, eicar); err != errDocumentMalwareDetected {
		t.Fatalf("EICAR test signature scan error = %v, want malware verdict", err)
	}
}
