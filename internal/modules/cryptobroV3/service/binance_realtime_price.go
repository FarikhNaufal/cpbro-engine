package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/usecase"
	"github.com/gorilla/websocket"
)

type BinanceRealtimePriceConfig struct {
	Enabled          bool
	BaseURL          string
	MaxActiveSymbols int
	ReconnectDelay   time.Duration
	StaleAfter       time.Duration
	ForceRestart     time.Duration
}

type BinanceRealtimePriceStream struct {
	cfg    BinanceRealtimePriceConfig
	dialer *websocket.Dialer

	mu      sync.RWMutex
	symbols []string
	prices  map[string]priceTick
	conn    *websocket.Conn
	writeMu sync.Mutex

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	done      chan struct{}
	updateCh  chan struct{}
	timerMu   sync.Mutex
	timer     *time.Timer

	connected atomic.Bool
	lastMsgNS atomic.Int64
	requestID atomic.Uint64

	liveUpdateHook func(subscribe, unsubscribe []string) error
}

type priceTick struct {
	Price     float64
	UpdatedAt time.Time
}

type combinedStreamMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type tickerPricePayload struct {
	Symbol    string `json:"s"`
	LastPrice string `json:"c"`
	EventTime int64  `json:"E"`
}

func NewBinanceRealtimePriceStream(cfg BinanceRealtimePriceConfig) *BinanceRealtimePriceStream {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "wss://fstream.binance.com"
	}
	if cfg.MaxActiveSymbols <= 0 {
		cfg.MaxActiveSymbols = 50
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = 5 * time.Second
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 15 * time.Second
	}
	if cfg.ForceRestart <= 0 {
		cfg.ForceRestart = 23 * time.Hour
	}

	return &BinanceRealtimePriceStream{
		cfg:      cfg,
		dialer:   websocket.DefaultDialer,
		prices:   make(map[string]priceTick),
		done:     make(chan struct{}),
		updateCh: make(chan struct{}, 1),
	}
}

func (s *BinanceRealtimePriceStream) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		return
	}
	s.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		go s.run(runCtx)
	})
}

func (s *BinanceRealtimePriceStream) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		} else {
			return
		}
		s.stopReconnectTimer()
		s.closeConn()
		<-s.done
	})
}

func (s *BinanceRealtimePriceStream) SyncSymbols(symbols []string) error {
	if !s.cfg.Enabled {
		return nil
	}
	normalized := normalizeSymbols(symbols)
	if len(normalized) > s.cfg.MaxActiveSymbols {
		normalized = normalized[:s.cfg.MaxActiveSymbols]
	}

	s.mu.Lock()
	previous := append([]string(nil), s.symbols...)
	changed := !equalStrings(s.symbols, normalized)
	if changed {
		s.symbols = normalized
	}
	canLiveUpdate := s.conn != nil || s.liveUpdateHook != nil
	s.mu.Unlock()

	if !changed {
		return nil
	}

	s.pruneRemovedPrices(previous, normalized)

	if len(normalized) == 0 {
		s.signalUpdate()
		s.scheduleReconnect()
		return nil
	}

	if canLiveUpdate && s.connected.Load() {
		if err := s.applySubscriptionDelta(previous, normalized); err == nil {
			return nil
		} else {
			slog.Warn("Failed to update websocket subscriptions live; reconnecting", "error", err)
		}
	}

	s.signalUpdate()
	s.scheduleReconnect()
	return nil
}

func (s *BinanceRealtimePriceStream) GetLatestPrice(symbol string) (float64, time.Time, bool) {
	if !s.cfg.Enabled {
		return 0, time.Time{}, false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return 0, time.Time{}, false
	}

	s.mu.RLock()
	tick, ok := s.prices[symbol]
	s.mu.RUnlock()
	if !ok {
		return 0, time.Time{}, false
	}
	if time.Since(tick.UpdatedAt) > s.cfg.StaleAfter {
		return 0, tick.UpdatedAt, false
	}
	return tick.Price, tick.UpdatedAt, true
}

func (s *BinanceRealtimePriceStream) RealtimeStatus() usecase.RealtimePriceStatus {
	if s == nil {
		return usecase.RealtimePriceStatus{}
	}
	status := usecase.RealtimePriceStatus{
		Enabled:       s.cfg.Enabled,
		ActiveSymbols: len(s.snapshotSymbols()),
	}
	if lastMsgNS := s.lastMsgNS.Load(); lastMsgNS > 0 {
		status.LastMessageTime = time.Unix(0, lastMsgNS)
	}
	status.Connected = s.connected.Load()
	if status.Enabled && status.ActiveSymbols > 0 {
		if status.LastMessageTime.IsZero() || time.Since(status.LastMessageTime) > (s.cfg.StaleAfter*2) {
			status.Connected = false
		}
	}
	return status
}

func (s *BinanceRealtimePriceStream) run(ctx context.Context) {
	defer close(s.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		symbols := s.snapshotSymbols()
		if len(symbols) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.updateCh:
				continue
			case <-time.After(2 * time.Second):
				continue
			}
		}

		if err := s.runConnection(ctx, symbols); err != nil && ctx.Err() == nil {
			slog.Warn("Binance websocket price stream disconnected", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-s.updateCh:
		case <-time.After(s.cfg.ReconnectDelay):
		}
	}
}

func (s *BinanceRealtimePriceStream) runConnection(ctx context.Context, symbols []string) error {
	conn, _, err := s.dialer.DialContext(ctx, s.buildURL(symbols), http.Header{})
	if err != nil {
		s.connected.Store(false)
		return err
	}

	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	defer s.closeConn()

	s.connected.Store(true)
	startedAt := time.Now()
	readDeadline := 10 * time.Minute
	_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetPongHandler(func(appData string) error {
		return conn.SetReadDeadline(time.Now().Add(readDeadline))
	})

	for {
		if s.cfg.ForceRestart > 0 && time.Since(startedAt) >= s.cfg.ForceRestart {
			return fmt.Errorf("websocket force restart window reached")
		}

		_, payload, err := conn.ReadMessage()
		if err != nil {
			s.connected.Store(false)
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
		if err := s.handlePayload(payload); err != nil {
			slog.Debug("Ignoring websocket payload parse error", "error", err)
			continue
		}
		s.lastMsgNS.Store(time.Now().UnixNano())
	}
}

func (s *BinanceRealtimePriceStream) handlePayload(payload []byte) error {
	var combined combinedStreamMessage
	if err := json.Unmarshal(payload, &combined); err == nil && len(combined.Data) > 0 {
		return s.handleTickerPricePayload(combined.Data)
	}
	return s.handleTickerPricePayload(payload)
}

func (s *BinanceRealtimePriceStream) handleTickerPricePayload(payload []byte) error {
	var msg tickerPricePayload
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}
	price, err := strconv.ParseFloat(msg.LastPrice, 64)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.prices[strings.ToUpper(msg.Symbol)] = priceTick{
		Price:     price,
		UpdatedAt: time.Now(),
	}
	s.mu.Unlock()
	return nil
}

func (s *BinanceRealtimePriceStream) buildURL(symbols []string) string {
	streams := streamNamesForSymbols(symbols)
	base := strings.TrimRight(s.cfg.BaseURL, "/")
	return base + "/market/stream?streams=" + strings.Join(streams, "/")
}

func (s *BinanceRealtimePriceStream) applySubscriptionDelta(previous, desired []string) error {
	subscribe, unsubscribe := diffStreamSubscriptions(streamNamesForSymbols(previous), streamNamesForSymbols(desired))
	if len(subscribe) == 0 && len(unsubscribe) == 0 {
		return nil
	}
	if s.liveUpdateHook != nil {
		return s.liveUpdateHook(subscribe, unsubscribe)
	}

	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("websocket connection unavailable")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if len(unsubscribe) > 0 {
		if err := conn.WriteJSON(subscriptionCommand{
			Method: "UNSUBSCRIBE",
			Params: unsubscribe,
			ID:     s.requestID.Add(1),
		}); err != nil {
			return err
		}
	}
	if len(subscribe) > 0 {
		if err := conn.WriteJSON(subscriptionCommand{
			Method: "SUBSCRIBE",
			Params: subscribe,
			ID:     s.requestID.Add(1),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *BinanceRealtimePriceStream) snapshotSymbols() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.symbols))
	copy(out, s.symbols)
	return out
}

func (s *BinanceRealtimePriceStream) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.connected.Store(false)
}

func (s *BinanceRealtimePriceStream) signalUpdate() {
	select {
	case s.updateCh <- struct{}{}:
	default:
	}
}

func (s *BinanceRealtimePriceStream) scheduleReconnect() {
	debounce := 300 * time.Millisecond
	if s.cfg.ReconnectDelay > 0 && s.cfg.ReconnectDelay < debounce {
		debounce = s.cfg.ReconnectDelay
	}

	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(debounce, func() {
		s.closeConn()
	})
}

func (s *BinanceRealtimePriceStream) stopReconnectTimer() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func normalizeSymbols(symbols []string) []string {
	unique := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			continue
		}
		if _, ok := unique[symbol]; ok {
			continue
		}
		unique[symbol] = struct{}{}
		out = append(out, symbol)
	}
	sort.Strings(out)
	return out
}

type subscriptionCommand struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     uint64   `json:"id"`
}

func streamNamesForSymbols(symbols []string) []string {
	streams := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		streams = append(streams, strings.ToLower(symbol)+"@miniTicker")
	}
	return streams
}

func diffStreamSubscriptions(previous, desired []string) (subscribe, unsubscribe []string) {
	prevSet := make(map[string]struct{}, len(previous))
	nextSet := make(map[string]struct{}, len(desired))
	for _, stream := range previous {
		prevSet[stream] = struct{}{}
	}
	for _, stream := range desired {
		nextSet[stream] = struct{}{}
		if _, ok := prevSet[stream]; !ok {
			subscribe = append(subscribe, stream)
		}
	}
	for _, stream := range previous {
		if _, ok := nextSet[stream]; !ok {
			unsubscribe = append(unsubscribe, stream)
		}
	}
	return subscribe, unsubscribe
}

func (s *BinanceRealtimePriceStream) pruneRemovedPrices(previous, desired []string) {
	desiredSet := make(map[string]struct{}, len(desired))
	for _, symbol := range desired {
		desiredSet[symbol] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, symbol := range previous {
		if _, ok := desiredSet[symbol]; ok {
			continue
		}
		delete(s.prices, symbol)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
