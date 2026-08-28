package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	initialSSEScanBufferBytes = 64 * 1024
	maxSSELineBytes           = 1024 * 1024
	maxSSEEventBytes          = 4 * 1024 * 1024
	maxSSEStreamOutputBytes   = 4 * 1024 * 1024
	sseDonePayload            = "[DONE]"
)

var ErrSSEEventTooLarge = errors.New("SSE event exceeded safety limit")

var ErrSSEStreamTooLarge = errors.New("SSE stream output exceeded safety limit")

type SSEStreamError struct {
	Stage string
	Err   error
}

func (e *SSEStreamError) Error() string {
	if e.Err == nil {
		return "SSE stream (" + e.Stage + "): unknown error"
	}
	return "SSE stream (" + e.Stage + "): " + e.Err.Error()
}

func (e *SSEStreamError) Unwrap() error {
	return e.Err
}

type sseChunkDecoder func(payload []byte) (string, error)

type sseAccumulator struct {
	dataLines         []string
	eventBytes        int
	streamOutputBytes int
	events            chan<- StreamEvent
	decode            sseChunkDecoder
}

func streamSSE(
	ctx context.Context,
	body io.Reader,
	events chan<- StreamEvent,
	decode sseChunkDecoder,
) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, initialSSEScanBufferBytes), maxSSELineBytes)
	accumulator := sseAccumulator{
		dataLines: make([]string, 0, 1),
		events:    events,
		decode:    decode,
	}

	for scanner.Scan() {
		done, err := accumulator.acceptLine(ctx, scanner.Text())
		if err != nil || done {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return &SSEStreamError{Stage: "read", Err: err}
	}
	_, err := accumulator.dispatch(ctx)
	return err
}

func (a *sseAccumulator) acceptLine(ctx context.Context, rawLine string) (bool, error) {
	line := strings.TrimSuffix(rawLine, "\r")
	if line == "" {
		return a.dispatch(ctx)
	}
	data, ok := sseDataValue(line)
	if !ok {
		return false, nil
	}
	a.eventBytes += len(data)
	if a.eventBytes > maxSSEEventBytes {
		return false, &SSEStreamError{Stage: "read", Err: ErrSSEEventTooLarge}
	}
	a.dataLines = append(a.dataLines, data)
	return false, nil
}

func (a *sseAccumulator) dispatch(ctx context.Context) (bool, error) {
	if len(a.dataLines) == 0 {
		return false, nil
	}
	payload := strings.Join(a.dataLines, "\n")
	a.dataLines = a.dataLines[:0]
	a.eventBytes = 0
	if strings.TrimSpace(payload) == sseDonePayload {
		return true, nil
	}
	chunk, err := a.decode([]byte(payload))
	if err != nil {
		return false, &SSEStreamError{Stage: "decode", Err: err}
	}
	chunk = stripANSI(chunk)
	if chunk == "" {
		return false, nil
	}
	a.streamOutputBytes += len(chunk)
	if a.streamOutputBytes > maxSSEStreamOutputBytes {
		return false, &SSEStreamError{Stage: "limit", Err: ErrSSEStreamTooLarge}
	}
	select {
	case a.events <- newStreamChunk(chunk):
		return false, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func sseDataValue(line string) (string, bool) {
	if strings.HasPrefix(line, ":") {
		return "", false
	}
	field, value, found := strings.Cut(line, ":")
	if !found || field != "data" {
		return "", false
	}
	value = strings.TrimPrefix(value, " ")
	return value, true
}

func malformedSSEPayload(provider string, err error) error {
	return &ProviderError{Provider: provider, Operation: "decode stream", Err: fmt.Errorf("invalid JSON payload: %w", err)}
}
