package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cpbro-engine/internal/modules/cryptobroV3/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogBroadcaster manages active real-time client subscribers
type LogBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan string]bool
}

// GlobalLogBroadcaster is the single shared instance for log broadcasting
var GlobalLogBroadcaster = NewLogBroadcaster()

// NewLogBroadcaster creates a LogBroadcaster instance
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		clients: make(map[chan string]bool),
	}
}

// Subscribe registers a new client channel
func (b *LogBroadcaster) Subscribe() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 200) // Buffer of 200 to prevent slow client block
	b.clients[ch] = true
	return ch
}

// Unsubscribe unregisters a client channel and closes it
func (b *LogBroadcaster) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.clients[ch]; exists {
		delete(b.clients, ch)
		close(ch)
	}
}

// Broadcast sends the log message to all active subscribers in a non-blocking manner
func (b *LogBroadcaster) Broadcast(msg string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// Subscriber channel is full; drop the message to avoid blocking the main server threads
		}
	}
}

// BroadcastWriter is an io.Writer that forwards logs to the LogBroadcaster
type BroadcastWriter struct {
	broadcaster *LogBroadcaster
}

// NewBroadcastWriter creates a BroadcastWriter instance
func NewBroadcastWriter(b *LogBroadcaster) *BroadcastWriter {
	return &BroadcastWriter{broadcaster: b}
}

// Write implements the io.Writer interface
func (w *BroadcastWriter) Write(p []byte) (n int, err error) {
	w.broadcaster.Broadcast(string(p))
	return len(p), nil
}

// MultiHandler multiplexes slog records to multiple underlying slog.Handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a new MultiHandler
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Enabled checks if any of the underlying handlers enable the log level
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle routes the log record to all underlying handlers
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []string
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("MultiHandler write error: %s", strings.Join(errs, "; "))
	}
	return nil
}

// WithAttrs returns a new MultiHandler with the attributes added to all underlying handlers
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		cloned[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: cloned}
}

// WithGroup returns a new MultiHandler with the group added to all underlying handlers
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	cloned := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		cloned[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: cloned}
}

// InitLogger configures the default slog logger using Lumberjack and console handlers
func InitLogger(cfg *config.Config) error {
	// 1. Determine log level
	var level slog.Level
	switch strings.ToLower(cfg.Logging.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// 2. Ensure log directory exists
	logDir := filepath.Dir(cfg.Logging.LogFilePath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// 3. Create Lumberjack file logger
	fileLogger := &lumberjack.Logger{
		Filename:   cfg.Logging.LogFilePath,
		MaxSize:    cfg.Logging.LogMaxSizeMB,
		MaxBackups: cfg.Logging.LogMaxBackups,
		MaxAge:     cfg.Logging.LogMaxAgeDays,
		Compress:   cfg.Logging.LogCompress,
	}

	// 4. Create BroadcastWriter to stream logs to SSE subscribers
	broadcastWriter := NewBroadcastWriter(GlobalLogBroadcaster)

	// Combine lumberjack and broadcaster
	fileMultiWriter := io.MultiWriter(fileLogger, broadcastWriter)

	// File handler is always JSON format so frontend/backend can parse it easily
	fileHandler := slog.NewJSONHandler(fileMultiWriter, opts)

	// 5. Create Console (Stdout) handler
	var consoleHandler slog.Handler
	if strings.ToLower(cfg.Logging.LogFormat) == "text" {
		consoleHandler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		consoleHandler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// 6. Combine handlers using MultiHandler
	multiHandler := NewMultiHandler(consoleHandler, fileHandler)

	// 7. Set as global default logger
	slog.SetDefault(slog.New(multiHandler))
	return nil
}

// ReadLastNLines reads the last n lines of a file efficiently by seeking backward
func ReadLastNLines(filePath string, n int) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	var cursor int64 = 0
	var count = 0
	var lastChunk []byte
	var readOffset int64
	chunkSize := int64(4096)

	// Seek backward in chunks to scan for newlines
	for count < n+1 && cursor < size {
		cursor += chunkSize
		if cursor > size {
			cursor = size
		}
		readOffset = size - cursor
		_, err = file.Seek(readOffset, 0)
		if err != nil {
			return nil, err
		}

		buf := make([]byte, cursor-int64(len(lastChunk)))
		if len(buf) == 0 {
			break
		}
		_, err = file.Read(buf)
		if err != nil {
			return nil, err
		}

		combined := append(buf, lastChunk...)
		lastChunk = combined

		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				count++
				if count >= n+1 {
					lastChunk = combined[i+1:]
					break
				}
			}
		}
	}

	lines := strings.Split(string(lastChunk), "\n")
	var result []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) > n {
		result = result[len(result)-n:]
	}

	return result, nil
}
