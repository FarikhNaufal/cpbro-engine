package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestBinanceRealtimePriceStream_StoresLatestPriceFromMiniTickerPayload(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com",
		MaxActiveSymbols: 10,
		ReconnectDelay:   time.Second,
		StaleAfter:       5 * time.Second,
		ForceRestart:     time.Hour,
	})

	raw := []byte(fmt.Sprintf(`{"E":%d,"s":"BTCUSDT","c":"105123.45"}`, time.Now().UnixMilli()))

	if err := stream.handleTickerPricePayload(raw); err != nil {
		t.Fatalf("handleTickerPricePayload failed: %v", err)
	}

	price, _, ok := stream.GetLatestPrice("BTCUSDT")
	if !ok {
		t.Fatal("expected latest price to be available")
	}
	if price != 105123.45 {
		t.Fatalf("expected 105123.45, got %v", price)
	}
}

func TestBinanceRealtimePriceStream_HandlePayloadSupportsCombinedWrapper(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com",
		MaxActiveSymbols: 10,
		ReconnectDelay:   time.Second,
		StaleAfter:       5 * time.Second,
		ForceRestart:     time.Hour,
	})

	raw := []byte(fmt.Sprintf(`{"stream":"btcusdt@miniTicker","data":{"E":%d,"s":"BTCUSDT","c":"105123.45"}}`, time.Now().UnixMilli()))
	if err := stream.handlePayload(raw); err != nil {
		t.Fatalf("handlePayload failed for combined wrapper: %v", err)
	}

	price, _, ok := stream.GetLatestPrice("BTCUSDT")
	if !ok || price != 105123.45 {
		t.Fatalf("expected combined payload price to be stored, got price=%v ok=%v", price, ok)
	}
}

func TestBinanceRealtimePriceStream_TrimsDuplicateSymbols(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com",
		MaxActiveSymbols: 2,
		ReconnectDelay:   time.Second,
		StaleAfter:       time.Second,
		ForceRestart:     time.Hour,
	})

	if err := stream.SyncSymbols([]string{"btcusdt", "BTCUSDT", "ethusdt", "solusdt"}); err != nil {
		t.Fatalf("SyncSymbols failed: %v", err)
	}

	got := stream.snapshotSymbols()
	raw, _ := json.Marshal(got)
	if len(got) != 2 || got[0] != "BTCUSDT" || got[1] != "ETHUSDT" {
		t.Fatalf("unexpected symbols after trim/dedupe: %s", raw)
	}
}

func TestBinanceRealtimePriceStream_BuildURLUsesCombinedMiniTickerStreams(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com/",
		MaxActiveSymbols: 2,
		ReconnectDelay:   time.Second,
		StaleAfter:       time.Second,
		ForceRestart:     time.Hour,
	})

	got := stream.buildURL([]string{"BTCUSDT", "ETHUSDT"})
	want := "wss://fstream.binance.com/market/stream?streams=btcusdt%40miniTicker%2Fethusdt%40miniTicker"
	if got != want {
		t.Fatalf("unexpected websocket url: %s", got)
	}
}

func TestBinanceRealtimePriceStream_RealtimeStatusRequiresFreshValidPayload(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com/",
		MaxActiveSymbols: 2,
		ReconnectDelay:   time.Second,
		StaleAfter:       time.Second,
		ForceRestart:     time.Hour,
	})
	_ = stream.SyncSymbols([]string{"BTCUSDT"})
	stream.connected.Store(true)

	if status := stream.RealtimeStatus(); status.Connected {
		t.Fatalf("expected no valid payload yet to report disconnected")
	}

	stream.lastMsgNS.Store(time.Now().Add(-3 * time.Second).UnixNano())
	if status := stream.RealtimeStatus(); status.Connected {
		t.Fatalf("expected stale payload timestamp to report disconnected")
	}

	stream.lastMsgNS.Store(time.Now().UnixNano())
	if status := stream.RealtimeStatus(); !status.Connected {
		t.Fatalf("expected fresh valid payload to report connected")
	}
}

func TestDiffStreamSubscriptions(t *testing.T) {
	subscribe, unsubscribe := diffStreamSubscriptions(
		[]string{"btcusdt@miniTicker", "ethusdt@miniTicker"},
		[]string{"ethusdt@miniTicker", "solusdt@miniTicker"},
	)

	if len(subscribe) != 1 || subscribe[0] != "solusdt@miniTicker" {
		t.Fatalf("unexpected subscribe diff: %#v", subscribe)
	}
	if len(unsubscribe) != 1 || unsubscribe[0] != "btcusdt@miniTicker" {
		t.Fatalf("unexpected unsubscribe diff: %#v", unsubscribe)
	}
}

func TestBinanceRealtimePriceStream_SyncSymbolsUsesLiveSubscriptionDeltaWhenConnected(t *testing.T) {
	stream := NewBinanceRealtimePriceStream(BinanceRealtimePriceConfig{
		Enabled:          true,
		BaseURL:          "wss://fstream.binance.com/",
		MaxActiveSymbols: 4,
		ReconnectDelay:   time.Second,
		StaleAfter:       time.Second,
		ForceRestart:     time.Hour,
	})

	stream.mu.Lock()
	stream.symbols = []string{"BTCUSDT", "ETHUSDT"}
	stream.prices["BTCUSDT"] = priceTick{Price: 100, UpdatedAt: time.Now()}
	stream.prices["ETHUSDT"] = priceTick{Price: 200, UpdatedAt: time.Now()}
	stream.mu.Unlock()
	stream.connected.Store(true)

	var gotSubscribe, gotUnsubscribe []string
	stream.liveUpdateHook = func(subscribe, unsubscribe []string) error {
		gotSubscribe = append([]string(nil), subscribe...)
		gotUnsubscribe = append([]string(nil), unsubscribe...)
		return nil
	}

	if err := stream.SyncSymbols([]string{"ETHUSDT", "SOLUSDT"}); err != nil {
		t.Fatalf("SyncSymbols failed: %v", err)
	}

	if len(gotSubscribe) != 1 || gotSubscribe[0] != "solusdt@miniTicker" {
		t.Fatalf("unexpected live subscribe diff: %#v", gotSubscribe)
	}
	if len(gotUnsubscribe) != 1 || gotUnsubscribe[0] != "btcusdt@miniTicker" {
		t.Fatalf("unexpected live unsubscribe diff: %#v", gotUnsubscribe)
	}
	if _, _, ok := stream.GetLatestPrice("BTCUSDT"); ok {
		t.Fatalf("expected removed symbol price to be pruned")
	}
	if stream.timer != nil {
		t.Fatalf("expected live subscription update to avoid reconnect timer")
	}
}
