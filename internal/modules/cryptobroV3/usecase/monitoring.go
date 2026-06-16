package usecase

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type MonitoringUsecase struct {
	marketDataProvider MarketDataProvider
	storageUsecase     *StorageUsecase
	latestPriceFeed    LatestPriceFeed
	candleCacheMu      sync.Mutex
	candleCache        map[string]monitoringCandleCacheEntry
}

type monitoringCandleCacheEntry struct {
	candles    []dto.Candle
	validUntil time.Time
}

type monitoringStatusProfile struct {
	Monitoring Status
	TP1Hit     Status
	TP2Hit     Status
	SLHit      Status
	Expired    Status

	CandleSLReason    string
	CandleTP1Reason   string
	CandleTP2Reason   string
	LiveSLReason      string
	LiveTP1Reason     string
	LiveTP2Reason     string
	LiveSLAfterTP1    string
	ExpiredMonitoring string
	ExpiredAfterTP1   string
}

func executeMonitoringProfile() monitoringStatusProfile {
	holdLabel := getMonitoringMaxHoldLabel()
	return monitoringStatusProfile{
		Monitoring:        MONITORING,
		TP1Hit:            TP1_HIT,
		TP2Hit:            TP2_HIT,
		SLHit:             SL_HIT,
		Expired:           EXPIRED,
		CandleSLReason:    "Stop Loss hit during candle evaluation",
		CandleTP1Reason:   "TP1 hit during candle evaluation",
		CandleTP2Reason:   "TP2 hit during candle evaluation",
		LiveSLReason:      "Stop Loss hit live",
		LiveTP1Reason:     "TP1 hit live",
		LiveTP2Reason:     "TP2 hit live",
		LiveSLAfterTP1:    "SL hit live after TP1 (partial success)",
		ExpiredMonitoring: "Monitoring period expired (" + holdLabel + " elapsed) without hitting SL or TP1",
		ExpiredAfterTP1:   "Monitoring period expired (" + holdLabel + " elapsed) with TP1 success",
	}
}

func watchMonitoringProfile() monitoringStatusProfile {
	holdLabel := getMonitoringMaxHoldLabel()
	return monitoringStatusProfile{
		Monitoring:        WATCH_MONITORING,
		TP1Hit:            VIRTUAL_TP1_HIT,
		TP2Hit:            VIRTUAL_TP2_HIT,
		SLHit:             VIRTUAL_SL_HIT,
		Expired:           VIRTUAL_EXPIRED,
		CandleSLReason:    "Virtual Stop Loss hit during candle evaluation",
		CandleTP1Reason:   "Virtual TP1 hit during candle evaluation",
		CandleTP2Reason:   "Virtual TP2 hit during candle evaluation",
		LiveSLReason:      "Virtual Stop Loss hit live",
		LiveTP1Reason:     "Virtual TP1 hit live",
		LiveTP2Reason:     "Virtual TP2 hit live",
		LiveSLAfterTP1:    "Virtual SL hit live after virtual TP1 (partial success)",
		ExpiredMonitoring: "Virtual monitoring period expired (" + holdLabel + " elapsed) without hitting virtual SL or TP1",
		ExpiredAfterTP1:   "Virtual monitoring period expired (" + holdLabel + " elapsed) with virtual TP1 success",
	}
}

func NewMonitoringUsecase(provider MarketDataProvider, storage *StorageUsecase) *MonitoringUsecase {
	return &MonitoringUsecase{
		marketDataProvider: provider,
		storageUsecase:     storage,
		candleCache:        make(map[string]monitoringCandleCacheEntry),
	}
}

func (uc *MonitoringUsecase) SetLatestPriceFeed(feed LatestPriceFeed) {
	if uc == nil {
		return
	}
	uc.latestPriceFeed = feed
}

// MonitorVirtualPositions updates both actionable FINAL_EXECUTE signals and
// non-actionable FINAL_WATCH paper setups without ever upgrading watch to execute.
func (uc *MonitoringUsecase) MonitorVirtualPositions(ctx context.Context) error {
	signalJournal, err := uc.storageUsecase.LoadSignalJournal()
	if err != nil {
		signalJournal = nil
	}
	watchJournal, err := uc.storageUsecase.LoadWatchJournal()
	if err != nil {
		watchJournal = nil
	}

	activeSymbols := collectActiveJournalSymbols(signalJournal, watchJournal)
	uc.pruneMonitoringCandleCache(activeSymbols)
	if uc.latestPriceFeed != nil {
		_ = uc.latestPriceFeed.SyncSymbols(activeSymbols)
	}
	uc.preloadMonitoringCandles(ctx, activeSymbols, time.Now())

	if err := uc.monitorSignalJournal(ctx, signalJournal); err != nil {
		return err
	}
	if err := uc.monitorWatchJournal(ctx, watchJournal); err != nil {
		return err
	}
	return nil
}

func (uc *MonitoringUsecase) pruneMonitoringCandleCache(activeSymbols []string) {
	if uc == nil {
		return
	}
	activeSet := make(map[string]struct{}, len(activeSymbols))
	for _, symbol := range activeSymbols {
		activeSet[symbol] = struct{}{}
	}

	uc.candleCacheMu.Lock()
	defer uc.candleCacheMu.Unlock()
	for symbol := range uc.candleCache {
		if _, ok := activeSet[symbol]; !ok {
			delete(uc.candleCache, symbol)
		}
	}
}

func (uc *MonitoringUsecase) preloadMonitoringCandles(ctx context.Context, activeSymbols []string, now time.Time) {
	if uc == nil || len(activeSymbols) == 0 {
		return
	}

	concurrency := minInt(len(activeSymbols), 4)
	if limit := getRuntimeSettings().MaxMonitoringCandleConcurrency; limit > 0 {
		concurrency = minInt(len(activeSymbols), limit)
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for _, symbol := range activeSymbols {
		if uc.getMonitoringCandlesFromCache(symbol, now) != nil {
			continue
		}
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if candles, err := uc.marketDataProvider.FetchClosedCandles(ctx, symbol, "15m", 20); err == nil {
				sort.Slice(candles, func(a, b int) bool {
					return candles[a].Time.Before(candles[b].Time)
				})
				uc.storeMonitoringCandles(symbol, candles, now)
			}
		}(symbol)
	}
	wg.Wait()
}

func (uc *MonitoringUsecase) monitorSignalJournal(ctx context.Context, journal []SignalJournal) error {
	if len(journal) == 0 {
		return nil
	}

	now := time.Now()
	_, changedEntries := monitorJournalEntries(ctx, uc, uc.marketDataProvider, uc.latestPriceFeed, journal, now, executeMonitoringProfile())
	if len(changedEntries) == 0 {
		return nil
	}

	return uc.storageUsecase.UpsertSignalJournalEntries(changedEntries)
}

func (uc *MonitoringUsecase) monitorWatchJournal(ctx context.Context, journal []WatchJournal) error {
	if len(journal) == 0 {
		return nil
	}

	now := time.Now()
	_, changedEntries := monitorJournalEntries(ctx, uc, uc.marketDataProvider, uc.latestPriceFeed, journal, now, watchMonitoringProfile())
	if len(changedEntries) == 0 {
		return nil
	}

	return uc.storageUsecase.UpsertWatchJournalEntries(changedEntries)
}

func monitorJournalEntries(ctx context.Context, uc *MonitoringUsecase, provider MarketDataProvider, feed LatestPriceFeed, journal []SignalJournal, now time.Time, profile monitoringStatusProfile) ([]SignalJournal, []SignalJournal) {
	changedEntries := make([]SignalJournal, 0)

	for i := range journal {
		original := journal[i]
		item := original
		if !isActiveMonitoringStatus(item.Status, profile, item.ClosedAt) {
			continue
		}

		candles, err := loadMonitoringCandles(ctx, uc, provider, item.Symbol, now)
		if err == nil && len(candles) > 0 {
			for _, c := range candles {
				if c.Time.Before(item.CreatedAt) || c.Time.Equal(item.CreatedAt) {
					continue
				}
				if c.Time.After(item.ExpiresAt) {
					break
				}

				updateExcursionFromCandle(&item, c)

				if isStopLossHit(item.Direction, c, item.StopLoss) {
					item.Status = profile.SLHit
					item.TimeToSL = c.Time.Sub(item.CreatedAt).String()
					if item.TimeToTP1 != "" {
						item.OutcomeReason = profile.LiveSLAfterTP1
					} else {
						item.OutcomeReason = profile.CandleSLReason
					}
					break
				}

				if item.Status == profile.Monitoring && isTP1Hit(item.Direction, c, item.TP1) {
					item.Status = profile.TP1Hit
					item.TimeToTP1 = c.Time.Sub(item.CreatedAt).String()
					item.OutcomeReason = profile.CandleTP1Reason
					item.StopLoss = item.EntryPrice // Move SL to entry price (breakeven)
				}

				if item.Status == profile.TP1Hit && isTP2Hit(item.Direction, c, item.TP2) {
					item.Status = profile.TP2Hit
					item.TimeToTP2 = c.Time.Sub(item.CreatedAt).String()
					item.OutcomeReason = profile.CandleTP2Reason
					break
				}
			}
		}

		if item.Status == profile.Monitoring || item.Status == profile.TP1Hit {
			price, err := resolveLatestPriceForMonitoring(ctx, provider, feed, item.Symbol)
			if err == nil {
				item.LatestPrice = price
				updateLivePnl(&item, price)
				updateExcursionFromPrice(&item, price)

				// Capture elapsed once to ensure TimeToTP1 <= TimeToTP2 and
				// prevent tp2_before_tp1 anomaly from multiple time.Since() calls.
				elapsed := time.Since(item.CreatedAt)

				if item.Direction == LONG {
					switch {
					case item.Status == profile.Monitoring && price <= item.StopLoss:
						item.Status = profile.SLHit
						item.TimeToSL = elapsed.String()
						item.OutcomeReason = profile.LiveSLReason
					case item.Status == profile.TP1Hit && price <= item.StopLoss:
						item.TimeToSL = elapsed.String()
						item.OutcomeReason = profile.LiveSLAfterTP1
						item.ClosedAt = now
						item.ExpiresAt = now.Add(-1 * time.Minute)
					case item.Status == profile.Monitoring && price >= item.TP1:
						item.Status = profile.TP1Hit
						item.TimeToTP1 = elapsed.String()
						item.OutcomeReason = profile.LiveTP1Reason
						item.StopLoss = item.EntryPrice // Move SL to entry price (breakeven)
						if price >= item.TP2 {
							item.Status = profile.TP2Hit
							item.TimeToTP2 = elapsed.String()
							item.OutcomeReason = profile.LiveTP2Reason
						}
					case item.Status == profile.TP1Hit && price >= item.TP2:
						item.Status = profile.TP2Hit
						item.TimeToTP2 = elapsed.String()
						item.OutcomeReason = profile.LiveTP2Reason
					}
				} else {
					switch {
					case item.Status == profile.Monitoring && price >= item.StopLoss:
						item.Status = profile.SLHit
						item.TimeToSL = elapsed.String()
						item.OutcomeReason = profile.LiveSLReason
					case item.Status == profile.TP1Hit && price >= item.StopLoss:
						item.TimeToSL = elapsed.String()
						item.OutcomeReason = profile.LiveSLAfterTP1
						item.ClosedAt = now
						item.ExpiresAt = now.Add(-1 * time.Minute)
					case item.Status == profile.Monitoring && price <= item.TP1:
						item.Status = profile.TP1Hit
						item.TimeToTP1 = elapsed.String()
						item.OutcomeReason = profile.LiveTP1Reason
						item.StopLoss = item.EntryPrice // Move SL to entry price (breakeven)
						if price <= item.TP2 {
							item.Status = profile.TP2Hit
							item.TimeToTP2 = elapsed.String()
							item.OutcomeReason = profile.LiveTP2Reason
						}
					case item.Status == profile.TP1Hit && price <= item.TP2:
						item.Status = profile.TP2Hit
						item.TimeToTP2 = elapsed.String()
						item.OutcomeReason = profile.LiveTP2Reason
					}
				}
			}
		}

		if item.Status == profile.Monitoring || item.Status == profile.TP1Hit {
			if now.After(item.ExpiresAt) || now.Sub(item.CreatedAt) >= 120*time.Minute {
				if item.Status == profile.Monitoring {
					item.Status = profile.Expired
					item.OutcomeReason = profile.ExpiredMonitoring
				} else if item.Status == profile.TP1Hit {
					if strings.TrimSpace(item.OutcomeReason) == "" || item.OutcomeReason == profile.LiveTP1Reason || item.OutcomeReason == profile.CandleTP1Reason {
						item.OutcomeReason = profile.ExpiredAfterTP1
					}
					if item.ClosedAt.IsZero() {
						item.ClosedAt = now
					}
				}
			}
		}

		applyJournalOutcomePnl(&item, profile, now)

		if !monitoringJournalEntryChanged(original, item) {
			continue
		}

		if original.Status != item.Status {
			slog.Info("Monitoring status transitioned",
				"symbol", item.Symbol,
				"direction", string(item.Direction),
				"old_status", string(original.Status),
				"new_status", string(item.Status),
				"entry", item.EntryPrice,
				"latest_price", item.LatestPrice,
				"reason", item.OutcomeReason,
			)
		}

		item.UpdatedAt = now
		if isTerminalMonitoringStatus(item.Status, profile) && item.ClosedAt.IsZero() {
			item.ClosedAt = now
		}
		journal[i] = item
		changedEntries = append(changedEntries, item)
	}

	return journal, changedEntries
}

func loadMonitoringCandles(ctx context.Context, uc *MonitoringUsecase, provider MarketDataProvider, symbol string, now time.Time) ([]dto.Candle, error) {
	if uc != nil {
		if candles := uc.getMonitoringCandlesFromCache(symbol, now); candles != nil {
			return candles, nil
		}
	}

	candles, err := provider.FetchClosedCandles(ctx, symbol, "15m", 20)
	if err != nil {
		return nil, err
	}
	sort.Slice(candles, func(a, b int) bool {
		return candles[a].Time.Before(candles[b].Time)
	})

	if uc != nil {
		uc.storeMonitoringCandles(symbol, candles, now)
	}

	return candles, nil
}

func (uc *MonitoringUsecase) getMonitoringCandlesFromCache(symbol string, now time.Time) []dto.Candle {
	if uc == nil {
		return nil
	}
	uc.candleCacheMu.Lock()
	defer uc.candleCacheMu.Unlock()
	entry, ok := uc.candleCache[symbol]
	if !ok || !now.Before(entry.validUntil) {
		return nil
	}
	return append([]dto.Candle(nil), entry.candles...)
}

func (uc *MonitoringUsecase) storeMonitoringCandles(symbol string, candles []dto.Candle, now time.Time) {
	if uc == nil {
		return
	}
	validUntil := now.Add(15 * time.Second)
	if len(candles) > 0 {
		lastCloseTime := candles[len(candles)-1].Time.Add(15 * time.Minute)
		validUntil = lastCloseTime.Add(15 * time.Minute)
	}
	uc.candleCacheMu.Lock()
	uc.candleCache[symbol] = monitoringCandleCacheEntry{
		candles:    append([]dto.Candle(nil), candles...),
		validUntil: validUntil,
	}
	uc.candleCacheMu.Unlock()
}

func resolveLatestPriceForMonitoring(ctx context.Context, provider MarketDataProvider, feed LatestPriceFeed, symbol string) (float64, error) {
	if feed != nil {
		if price, _, ok := feed.GetLatestPrice(symbol); ok && price > 0 {
			return price, nil
		}
	}
	return provider.FetchLatestPrice(ctx, symbol)
}

func monitoringJournalEntryChanged(before, after SignalJournal) bool {
	if before.Status != after.Status {
		return true
	}
	if before.LatestPrice != after.LatestPrice {
		return true
	}
	if before.PnlPercentage != after.PnlPercentage {
		return true
	}
	if before.MFE != after.MFE || before.MAE != after.MAE {
		return true
	}
	if before.TimeToTP1 != after.TimeToTP1 || before.TimeToTP2 != after.TimeToTP2 || before.TimeToSL != after.TimeToSL {
		return true
	}
	if before.OutcomeReason != after.OutcomeReason {
		return true
	}
	if !before.ExpiresAt.Equal(after.ExpiresAt) {
		return true
	}
	return false
}

func isTerminalMonitoringStatus(status Status, profile monitoringStatusProfile) bool {
	switch status {
	case profile.TP2Hit, profile.SLHit, profile.Expired:
		return true
	default:
		return false
	}
}

func applyJournalOutcomePnl(item *SignalJournal, profile monitoringStatusProfile, now time.Time) {
	if item == nil || item.EntryPrice <= 0 {
		return
	}

	switch item.Status {
	case profile.TP2Hit:
		item.PnlPercentage = realizedTP2Pnl(item.EntryPrice, item.TP1, item.TP2)
	case profile.SLHit:
		if item.TimeToTP1 != "" {
			item.PnlPercentage = realizedStoppedAfterTP1Pnl(item.EntryPrice, item.TP1, item.StopLoss)
			return
		}
		item.PnlPercentage = realizedSLPnl(item.EntryPrice, item.StopLoss)
	case profile.Expired:
		item.PnlPercentage = 0.0
	case profile.TP1Hit:
		if isTP1FinalizedForJournal(*item, now) {
			if item.TimeToSL != "" {
				item.PnlPercentage = realizedStoppedAfterTP1Pnl(item.EntryPrice, item.TP1, item.StopLoss)
			} else {
				// 50% closed at TP1 + 50% closed at the actual close/expiration price (LatestPrice)
				floatingPnl := 0.0
				if item.LatestPrice > 0 {
					if item.Direction == LONG {
						floatingPnl = 50.0 * ((item.LatestPrice - item.EntryPrice) / item.EntryPrice)
					} else {
						floatingPnl = 50.0 * ((item.EntryPrice - item.LatestPrice) / item.EntryPrice)
					}
				}
				item.PnlPercentage = realizedTP1PartialPnl(item.EntryPrice, item.TP1) + floatingPnl
			}
		}
	}
}

func isTP1FinalizedForJournal(item SignalJournal, now time.Time) bool {
	if item.ClosedAt.IsZero() == false {
		return true
	}
	if item.ExpiresAt.IsZero() {
		return true
	}
	return !now.Before(item.ExpiresAt)
}

func realizedTP1PartialPnl(entryPrice, tp1 float64) float64 {
	if entryPrice <= 0 {
		return 0.0
	}
	return 50.0 * (absFloat(tp1-entryPrice) / entryPrice)
}

func realizedTP2Pnl(entryPrice, tp1, tp2 float64) float64 {
	if entryPrice <= 0 {
		return 0.0
	}
	return realizedTP1PartialPnl(entryPrice, tp1) + (50.0 * (absFloat(tp2-entryPrice) / entryPrice))
}

func realizedSLPnl(entryPrice, stopLoss float64) float64 {
	if entryPrice <= 0 {
		return 0.0
	}
	return -100.0 * (absFloat(entryPrice-stopLoss) / entryPrice)
}

func realizedStoppedAfterTP1Pnl(entryPrice, tp1, stopLoss float64) float64 {
	return realizedTP1PartialPnl(entryPrice, tp1) + (realizedSLPnl(entryPrice, stopLoss) / 2.0)
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func isActiveMonitoringStatus(status Status, profile monitoringStatusProfile, closedAt time.Time) bool {
	if status == profile.Monitoring {
		return true
	}
	if status == profile.TP1Hit && closedAt.IsZero() {
		return true
	}
	return false
}

func updateExcursionFromCandle(item *SignalJournal, candle dto.Candle) {
	high := candle.High
	low := candle.Low
	if item.Direction == LONG {
		floatingProfitPercent := ((high - item.EntryPrice) / item.EntryPrice) * 100
		if floatingProfitPercent > item.MFE {
			item.MFE = floatingProfitPercent
		}
		floatingLossPercent := ((item.EntryPrice - low) / item.EntryPrice) * 100
		if floatingLossPercent > item.MAE {
			item.MAE = floatingLossPercent
		}
		return
	}

	floatingProfitPercent := ((item.EntryPrice - low) / item.EntryPrice) * 100
	if floatingProfitPercent > item.MFE {
		item.MFE = floatingProfitPercent
	}
	floatingLossPercent := ((high - item.EntryPrice) / item.EntryPrice) * 100
	if floatingLossPercent > item.MAE {
		item.MAE = floatingLossPercent
	}
}

func updateExcursionFromPrice(item *SignalJournal, price float64) {
	if item.Direction == LONG {
		floatingProfitPercent := ((price - item.EntryPrice) / item.EntryPrice) * 100
		if floatingProfitPercent > item.MFE {
			item.MFE = floatingProfitPercent
		}
		floatingLossPercent := ((item.EntryPrice - price) / item.EntryPrice) * 100
		if floatingLossPercent > item.MAE {
			item.MAE = floatingLossPercent
		}
		return
	}

	floatingProfitPercent := ((item.EntryPrice - price) / item.EntryPrice) * 100
	if floatingProfitPercent > item.MFE {
		item.MFE = floatingProfitPercent
	}
	floatingLossPercent := ((price - item.EntryPrice) / item.EntryPrice) * 100
	if floatingLossPercent > item.MAE {
		item.MAE = floatingLossPercent
	}
}

func updateLivePnl(item *SignalJournal, price float64) {
	if item.EntryPrice <= 0 {
		return
	}
	isTP1 := item.Status == TP1_HIT || item.Status == VIRTUAL_TP1_HIT
	if item.Direction == LONG {
		if isTP1 {
			item.PnlPercentage = realizedTP1PartialPnl(item.EntryPrice, item.TP1) + (50.0 * ((price - item.EntryPrice) / item.EntryPrice))
		} else {
			item.PnlPercentage = ((price - item.EntryPrice) / item.EntryPrice) * 100
		}
		return
	}
	if item.Direction == SHORT {
		if isTP1 {
			item.PnlPercentage = realizedTP1PartialPnl(item.EntryPrice, item.TP1) + (50.0 * ((item.EntryPrice - price) / item.EntryPrice))
		} else {
			item.PnlPercentage = ((item.EntryPrice - price) / item.EntryPrice) * 100
		}
	}
}

func isStopLossHit(direction Direction, candle dto.Candle, stopLoss float64) bool {
	if direction == LONG {
		return candle.Low <= stopLoss
	}
	return candle.High >= stopLoss
}

func isTP1Hit(direction Direction, candle dto.Candle, tp1 float64) bool {
	if direction == LONG {
		return candle.High >= tp1
	}
	return candle.Low <= tp1
}

func isTP2Hit(direction Direction, candle dto.Candle, tp2 float64) bool {
	if direction == LONG {
		return candle.High >= tp2
	}
	return candle.Low <= tp2
}
