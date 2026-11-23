package server

import (
	"context"
	"sync"
)

// LogBuf keeps a rolling in-memory copy of recent log lines for /log.
type LogBuf struct {
	mu       sync.Mutex
	buf      []byte
	maxLines int
	start    int64 // absolute byte offset of buf[0]
	written  int64 // absolute end offset
	notify   chan struct{}
}

func NewLogBuf(maxLines int) *LogBuf {
	if maxLines < 1 {
		maxLines = 50
	}
	return &LogBuf{
		maxLines: maxLines,
		notify:   make(chan struct{}),
	}
}

func (l *LogBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	l.written += int64(len(p))
	if trimmed := lastNLines(l.buf, l.maxLines); len(trimmed) < len(l.buf) {
		l.buf = append([]byte(nil), trimmed...)
		l.start = l.written - int64(len(l.buf))
	}
	ch := l.notify
	l.notify = make(chan struct{})
	close(ch)
	return len(p), nil
}

func (l *LogBuf) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.buf...)
}

// TailLines returns up to the last n log lines and the absolute end offset.
func (l *LogBuf) TailLines(n int) (chunk []byte, end int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > l.maxLines {
		n = l.maxLines
	}
	return append([]byte(nil), lastNLines(l.buf, n)...), l.written
}

func lastNLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return nil
	}
	lines := 0
	if b[len(b)-1] != '\n' {
		lines = 1
	}
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			lines++
			if lines > n {
				return b[i+1:]
			}
		}
	}
	return b
}

// snapshotFrom returns bytes at/after absolute pos and the wait channel.
func (l *LogBuf) snapshotFrom(pos int64) (chunk []byte, next int64, wait <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if pos < l.start {
		pos = l.start
	}
	off := int(pos - l.start)
	if off < 0 {
		off = 0
	}
	if off > len(l.buf) {
		off = len(l.buf)
	}
	if off < len(l.buf) {
		chunk = append([]byte(nil), l.buf[off:]...)
	}
	return chunk, l.written, l.notify
}

// Follow writes the last n lines, then streams new bytes until ctx ends.
func (l *LogBuf) Follow(ctx context.Context, n int, write func([]byte) error) error {
	chunk, pos := l.TailLines(n)
	if len(chunk) > 0 {
		if err := write(chunk); err != nil {
			return err
		}
	}
	for {
		chunk, next, wait := l.snapshotFrom(pos)
		if len(chunk) > 0 {
			if err := write(chunk); err != nil {
				return err
			}
			pos = next
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}
