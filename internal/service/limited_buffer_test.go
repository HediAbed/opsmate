package service

import (
	"bytes"
	"sync"
	"testing"
)

func TestLimitedBuffer_CapsStoredDataAndDrainsWrites(t *testing.T) {
	buffer := newLimitedBuffer(5)
	input := []byte("abcdefgh")
	written, err := buffer.Write(input)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if written != len(input) {
		t.Fatalf("Write reported %d bytes, want %d", written, len(input))
	}
	if !bytes.Equal(buffer.Bytes(), []byte("abcde")) {
		t.Fatalf("stored bytes = %q, want abcde", buffer.Bytes())
	}
	if !buffer.Truncated() {
		t.Fatal("buffer must report truncation")
	}
}

func TestLimitedBuffer_ConcurrentWritersStayWithinLimit(t *testing.T) {
	const limit = 128
	buffer := newLimitedBuffer(limit)
	var writers sync.WaitGroup
	for range 8 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			if _, err := buffer.Write(bytes.Repeat([]byte("x"), limit)); err != nil {
				t.Errorf("Write returned error: %v", err)
			}
		}()
	}
	writers.Wait()
	if got := len(buffer.Bytes()); got != limit {
		t.Fatalf("stored length = %d, want %d", got, limit)
	}
	if !buffer.Truncated() {
		t.Fatal("concurrent writes must report truncation")
	}
}
