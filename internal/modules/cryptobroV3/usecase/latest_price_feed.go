package usecase

import (
	"context"
	"strings"
	"time"
)

type LatestPriceFeed interface {
	SyncSymbols(symbols []string) error
	GetLatestPrice(symbol string) (float64, time.Time, bool)
}

func normalizeActiveSymbols(symbols []string) []string {
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
	return out
}

func collectActiveJournalSymbols(signalJournal []SignalJournal, watchJournal []WatchJournal) []string {
	symbols := make([]string, 0, len(signalJournal)+len(watchJournal))
	for _, item := range signalJournal {
		if item.Status == MONITORING || item.Status == TP1_HIT {
			symbols = append(symbols, item.Symbol)
		}
	}
	for _, item := range watchJournal {
		if item.Status == WATCH_MONITORING || item.Status == VIRTUAL_TP1_HIT {
			symbols = append(symbols, item.Symbol)
		}
	}
	return normalizeActiveSymbols(symbols)
}

func unionActiveSymbols(groups ...[]string) []string {
	var merged []string
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return normalizeActiveSymbols(merged)
}

type latestPriceResolver struct {
	realtime LatestPriceFeed
	fallback MarketDataProvider
}

func (r latestPriceResolver) Resolve(ctx context.Context, symbol string) (float64, bool) {
	if r.realtime != nil {
		if price, _, ok := r.realtime.GetLatestPrice(symbol); ok && price > 0 {
			return price, true
		}
	}
	if r.fallback != nil {
		if price, err := r.fallback.FetchLatestPrice(ctx, symbol); err == nil && price > 0 {
			return price, true
		}
	}
	return 0, false
}
