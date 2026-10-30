package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var errDocumentMalwareDetected = errors.New("document contains malware")

// scanDocumentWithClamAV streams a bounded document to clamd and fails closed on scanner errors.
func scanDocumentWithClamAV(ctx context.Context, address string, content []byte) error {
	if strings.TrimSpace(address) == "" {
		return errors.New("ClamAV address is not configured")
	}
	if len(content) == 0 || len(content) > maxPrivateDocumentBytes {
		return errors.New("document size is outside the scan limit")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to ClamAV: %w", err)
	}
	defer connection.Close()
	deadline := time.Now().Add(30 * time.Second)
	if requestDeadline, ok := ctx.Deadline(); ok && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set ClamAV deadline: %w", err)
	}
	if err := writeAll(connection, []byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("start ClamAV stream: %w", err)
	}
	for offset := 0; offset < len(content); {
		end := offset + 32*1024
		if end > len(content) {
			end = len(content)
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(end-offset))
		if err := writeAll(connection, length[:]); err != nil {
			return fmt.Errorf("write ClamAV chunk length: %w", err)
		}
		if err := writeAll(connection, content[offset:end]); err != nil {
			return fmt.Errorf("write ClamAV chunk: %w", err)
		}
		offset = end
	}
	if err := writeAll(connection, []byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("finish ClamAV stream: %w", err)
	}
	response, err := bufio.NewReader(io.LimitReader(connection, 1024)).ReadString('\x00')
	if err != nil {
		return fmt.Errorf("read ClamAV response: %w", err)
	}
	response = strings.TrimSuffix(response, "\x00")
	if strings.HasSuffix(response, " OK") {
		return nil
	}
	if strings.HasSuffix(response, " FOUND") {
		return errDocumentMalwareDetected
	}
	return fmt.Errorf("ClamAV returned an unexpected response")
}

// writeAll sends a complete protocol frame even when a network write is partial.
func writeAll(writer io.Writer, content []byte) error {
	for len(content) > 0 {
		written, err := writer.Write(content)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}
