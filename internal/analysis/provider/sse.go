package provider

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/terminal"
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

type StreamStage string

const (
	StreamStageRead   StreamStage = "read"
	StreamStageDecode StreamStage = "decode"
	StreamStageLimit  StreamStage = "limit"
)

type SSEStreamError struct {
	Stage StreamStage
	Err   error
}

func (e *SSEStreamError) Error() string {
	if e == nil {
		return "SSE stream: unknown error"
	}
	if e.Err == nil {
		return "SSE stream (" + string(e.Stage) + "): unknown error"
	}
	return "SSE stream (" + string(e.Stage) + "): " + e.Err.Error()
}

func (e *SSEStreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *SSEStreamError) FailureCode() failure.Code {
	if e == nil || e.Err == nil {
		return failure.CodeUnknown
	}
	if errors.Is(e.Err, context.Canceled) {
		return failure.CodeCanceled
	}
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return failure.CodeDeadlineExceeded
	}
	if errors.Is(e.Err, ErrSSEEventTooLarge) || errors.Is(e.Err, ErrSSEStreamTooLarge) {
		return failure.CodeInternal
	}
	switch e.Stage {
	case StreamStageRead:
		return failure.CodeUnavailable
	case StreamStageDecode, StreamStageLimit:
		return failure.CodeInternal
	default:
		return failure.CodeUnknown
	}
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
		return &SSEStreamError{Stage: StreamStageRead, Err: err}
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
		return false, &SSEStreamError{Stage: StreamStageRead, Err: ErrSSEEventTooLarge}
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
		return false, &SSEStreamError{Stage: StreamStageDecode, Err: err}
	}
	chunk = terminal.SanitizeText(chunk)
	if chunk == "" {
		return false, nil
	}
	a.streamOutputBytes += len(chunk)
	if a.streamOutputBytes > maxSSEStreamOutputBytes {
		return false, &SSEStreamError{Stage: StreamStageLimit, Err: ErrSSEStreamTooLarge}
	}
	select {
	case a.events <- NewChunk(chunk):
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
	return &Error{
		Provider:  provider,
		Operation: failure.OperationDecode,
		Err:       fmt.Errorf("%w: %w", ErrProviderMalformedResponse, err),
	}
}
