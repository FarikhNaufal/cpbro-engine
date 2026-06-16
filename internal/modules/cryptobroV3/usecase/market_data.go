package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type MarketDataUsecase struct {
	provider MarketDataProvider

	bootstrapTimeout time.Duration
	initialTimeout   time.Duration
	enrichTimeout    time.Duration
	globalCacheTTL   time.Duration

	oiMu             sync.Mutex
	lastOpenInterest map[string]float64
	oiCacheMu        sync.RWMutex
	oiCache          map[string]cachedFloat64

	candleCacheMu sync.RWMutex
	candleCache   map[string]cachedClosedCandles

	tickerCacheMu      sync.RWMutex
	tickerCache        []dto.Ticker24h
	tickerCacheUntil   time.Time
	tickerCacheFetched time.Time

	fundingCacheMu      sync.RWMutex
	fundingCache        map[string]float64
	fundingCacheUntil   time.Time
	fundingCacheFetched time.Time
}

type bootstrapSource string

const (
	bootstrapSourceLive  bootstrapSource = "live"
	bootstrapSourceCache bootstrapSource = "cache"
	bootstrapSourceNone  bootstrapSource = "none"
)

type bootstrapFetchMeta struct {
	Source   bootstrapSource
	CacheAge time.Duration
}

type cachedClosedCandles struct {
	candles    []dto.Candle
	validUntil time.Time
}

type cachedFloat64 struct {
	value      float64
	validUntil time.Time
}

const (
	maxClosedCandleCacheEntries = 512
	maxOpenInterestCacheEntries = 512
)

type MarketDataUsecaseConfig struct {
	BootstrapTimeout time.Duration
	InitialTimeout   time.Duration
	EnrichTimeout    time.Duration
	GlobalCacheTTL   time.Duration
}

func NewMarketDataUsecase(provider MarketDataProvider, configs ...MarketDataUsecaseConfig) *MarketDataUsecase {
	cfg := MarketDataUsecaseConfig{
		BootstrapTimeout: 20 * time.Second,
		InitialTimeout:   10 * time.Second,
		EnrichTimeout:    15 * time.Second,
		GlobalCacheTTL:   30 * time.Second,
	}
	if len(configs) > 0 {
		in := configs[0]
		if in.BootstrapTimeout > 0 {
			cfg.BootstrapTimeout = in.BootstrapTimeout
		}
		if in.InitialTimeout > 0 {
			cfg.InitialTimeout = in.InitialTimeout
		}
		if in.EnrichTimeout > 0 {
			cfg.EnrichTimeout = in.EnrichTimeout
		}
		if in.GlobalCacheTTL > 0 {
			cfg.GlobalCacheTTL = in.GlobalCacheTTL
		}
	}

	return &MarketDataUsecase{
		provider:         provider,
		bootstrapTimeout: cfg.BootstrapTimeout,
		initialTimeout:   cfg.InitialTimeout,
		enrichTimeout:    cfg.EnrichTimeout,
		globalCacheTTL:   cfg.GlobalCacheTTL,
		lastOpenInterest: make(map[string]float64),
		oiCache:          make(map[string]cachedFloat64),
		candleCache:      make(map[string]cachedClosedCandles),
		fundingCache:     make(map[string]float64),
	}
}

// FetchAllFuturesTickers24h fetches stats for all tickers.
func (uc *MarketDataUsecase) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	tickers, _, err := uc.FetchAllFuturesTickers24hWithMeta(ctx)
	return tickers, err
}

// FetchAllFuturesTickers24hWithMeta fetches stats for all tickers and reports whether the result came from
// a live Binance response or a local cache fallback.
func (uc *MarketDataUsecase) FetchAllFuturesTickers24hWithMeta(ctx context.Context) ([]dto.Ticker24h, bootstrapFetchMeta, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, uc.bootstrapTimeout)
	defer cancel()

	tickers, err := uc.provider.FetchAllFuturesTickers24h(timeoutCtx)
	if err == nil {
		uc.storeTickerCache(tickers, time.Now())
		GetGlobalMetrics().ClearBootstrapTickerCacheAge()
		return tickers, bootstrapFetchMeta{Source: bootstrapSourceLive}, nil
	}

	if cached, fetchedAt, ok := uc.loadTickerCache(time.Now()); ok {
		cacheAge := time.Since(fetchedAt)
		GetGlobalMetrics().RecordBootstrapTickerCacheFallback(cacheAge)
		slog.Warn("Binance futures ticker bootstrap failed; using cached ticker snapshot", "error", err, "cached_count", len(cached), "cache_age_seconds", roundDurationSeconds(cacheAge))
		return cached, bootstrapFetchMeta{Source: bootstrapSourceCache, CacheAge: cacheAge}, nil
	}

	return nil, bootstrapFetchMeta{Source: bootstrapSourceNone}, err
}

// FetchPremiumFundingRates fetches all active symbols funding rates.
func (uc *MarketDataUsecase) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	funding, _, err := uc.FetchPremiumFundingRatesWithMeta(ctx)
	return funding, err
}

// FetchPremiumFundingRatesWithMeta fetches funding data and reports whether it came from a live Binance
// response or a cache fallback.
func (uc *MarketDataUsecase) FetchPremiumFundingRatesWithMeta(ctx context.Context) (map[string]float64, bootstrapFetchMeta, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, uc.bootstrapTimeout)
	defer cancel()

	funding, err := uc.provider.FetchPremiumFundingRates(timeoutCtx)
	if err == nil {
		uc.storeFundingCache(funding, time.Now())
		GetGlobalMetrics().ClearBootstrapFundingCacheAge()
		return funding, bootstrapFetchMeta{Source: bootstrapSourceLive}, nil
	}

	if cached, fetchedAt, ok := uc.loadFundingCache(time.Now()); ok {
		cacheAge := time.Since(fetchedAt)
		GetGlobalMetrics().RecordBootstrapFundingCacheFallback(cacheAge)
		slog.Warn("Binance premium funding bootstrap failed; using cached funding snapshot", "error", err, "cached_count", len(cached), "cache_age_seconds", roundDurationSeconds(cacheAge))
		return cached, bootstrapFetchMeta{Source: bootstrapSourceCache, CacheAge: cacheAge}, nil
	}

	return nil, bootstrapFetchMeta{Source: bootstrapSourceNone}, err
}

// FetchMarketData retrieves klines, open interest, funding rate, and latest price concurrently.
func (uc *MarketDataUsecase) FetchMarketData(ctx context.Context, symbol string, fundingRates map[string]float64) (MarketData, error) {
	initial, err := uc.FetchInitialMarketData(ctx, symbol, fundingRates)
	if err != nil {
		return MarketData{}, err
	}
	return uc.EnrichMarketData(ctx, initial)
}

// FetchInitialMarketData retrieves the cheapest closed-candle input first so callers can fast-reject stale symbols
// before paying the full H1/H4/OI snapshot cost.
func (uc *MarketDataUsecase) FetchInitialMarketData(ctx context.Context, symbol string, fundingRates map[string]float64) (MarketData, error) {
	rootCtx, cancelRoot := context.WithTimeout(ctx, uc.initialTimeout)
	defer cancelRoot()

	m15, err := uc.fetchClosedCandlesCached(rootCtx, symbol, "15m", 50)
	if err != nil {
		return MarketData{}, fmt.Errorf("failed to fetch initial market data for %s: %w", symbol, err)
	}

	fundingRate := 0.0
	if val, ok := fundingRates[symbol]; ok {
		fundingRate = val
	}

	return MarketData{
		Symbol:      symbol,
		M15Candles:  m15,
		FundingRate: fundingRate,
		LastUpdated: time.Now(),
	}, nil
}

// EnrichMarketData fills the higher-timeframe and derivatives context for a symbol that has already passed cheap
// initial checks (for example M15 freshness).
func (uc *MarketDataUsecase) EnrichMarketData(ctx context.Context, base MarketData) (MarketData, error) {
	rootCtx, cancelRoot := context.WithTimeout(ctx, uc.enrichTimeout)
	defer cancelRoot()
	symbol := base.Symbol

	var (
		h1           []dto.Candle
		h4           []dto.Candle
		openInterest float64
	)

	// NOTE:
	// TREND_PULLBACK requires H4 EMA(200) for trend alignment checks.
	// Fetching too few H4 candles makes the playbook permanently ineligible (systemic bug).
	const h4TrendCandleLimit = 210

	// Concurrency limit of 3 concurrent requests to prevent rate limits
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	tasks := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{
			name: "H1Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.fetchClosedCandlesCached(ctx, symbol, "1h", 50)
				if err != nil {
					return err
				}
				h1 = res
				return nil
			},
		},
		{
			name: "H4Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.fetchClosedCandlesCached(ctx, symbol, "4h", h4TrendCandleLimit)
				if err != nil {
					return err
				}
				h4 = res
				return nil
			},
		},
		{
			name: "OpenInterest",
			fn: func(ctx context.Context) error {
				res, err := uc.fetchOpenInterestCached(ctx, symbol)
				if err != nil {
					return err
				}
				openInterest = res
				return nil
			},
		},
	}

	for _, task := range tasks {
		wg.Add(1)
		go func(t struct {
			name string
			fn   func(ctx context.Context) error
		}) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-rootCtx.Done():
				setErr(fmt.Errorf("task %s cancelled before run: %w", t.name, rootCtx.Err()))
				return
			}

			reqCtx, cancelReq := context.WithTimeout(rootCtx, 5*time.Second)
			defer cancelReq()

			if err := t.fn(reqCtx); err != nil {
				setErr(fmt.Errorf("task %s failed: %w", t.name, err))
			}
		}(task)
	}

	wg.Wait()

	if firstErr != nil {
		return MarketData{}, fmt.Errorf("failed to fetch market data snapshot for %s: %w", symbol, firstErr)
	}

	oiChangePct := 0.0
	if openInterest > 0 {
		uc.oiMu.Lock()
		prev, hasPrev := uc.lastOpenInterest[symbol]
		uc.lastOpenInterest[symbol] = openInterest
		uc.oiMu.Unlock()

		if hasPrev && prev > 0 {
			oiChangePct = ((openInterest - prev) / prev) * 100.0
		}
	}

	return MarketData{
		Symbol:          symbol,
		M15Candles:      append([]dto.Candle(nil), base.M15Candles...),
		H1Candles:       h1,
		H4Candles:       h4,
		OpenInterestM15: openInterest,
		OIChangePct:     oiChangePct,
		FundingRate:     base.FundingRate,
		LatestPrice:     base.LatestPrice,
		PriceChange24h:  base.PriceChange24h,
		LastUpdated:     time.Now(),
	}, nil
}

func (uc *MarketDataUsecase) storeTickerCache(tickers []dto.Ticker24h, now time.Time) {
	if uc == nil {
		return
	}
	cloned := append([]dto.Ticker24h(nil), tickers...)
	uc.tickerCacheMu.Lock()
	uc.tickerCache = cloned
	uc.tickerCacheFetched = now
	uc.tickerCacheUntil = now.Add(uc.globalCacheTTL)
	uc.tickerCacheMu.Unlock()
}

func (uc *MarketDataUsecase) loadTickerCache(now time.Time) ([]dto.Ticker24h, time.Time, bool) {
	if uc == nil {
		return nil, time.Time{}, false
	}
	uc.tickerCacheMu.RLock()
	defer uc.tickerCacheMu.RUnlock()
	if len(uc.tickerCache) == 0 || !now.Before(uc.tickerCacheUntil) {
		return nil, time.Time{}, false
	}
	return append([]dto.Ticker24h(nil), uc.tickerCache...), uc.tickerCacheFetched, true
}

func (uc *MarketDataUsecase) storeFundingCache(funding map[string]float64, now time.Time) {
	if uc == nil {
		return
	}
	cloned := make(map[string]float64, len(funding))
	for k, v := range funding {
		cloned[k] = v
	}
	uc.fundingCacheMu.Lock()
	uc.fundingCache = cloned
	uc.fundingCacheFetched = now
	uc.fundingCacheUntil = now.Add(uc.globalCacheTTL)
	uc.fundingCacheMu.Unlock()
}

func (uc *MarketDataUsecase) loadFundingCache(now time.Time) (map[string]float64, time.Time, bool) {
	if uc == nil {
		return nil, time.Time{}, false
	}
	uc.fundingCacheMu.RLock()
	defer uc.fundingCacheMu.RUnlock()
	if len(uc.fundingCache) == 0 || !now.Before(uc.fundingCacheUntil) {
		return nil, time.Time{}, false
	}
	cloned := make(map[string]float64, len(uc.fundingCache))
	for k, v := range uc.fundingCache {
		cloned[k] = v
	}
	return cloned, uc.fundingCacheFetched, true
}

func roundDurationSeconds(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(d.Milliseconds()) / 1000.0
}

// FetchCandles fetches finalized candles for M15, H1, and H4 timeframes.
func (uc *MarketDataUsecase) FetchCandles(ctx context.Context, symbol string) (m15, h1, h4 []dto.Candle, err error) {
	m15, err = uc.fetchClosedCandlesCached(ctx, symbol, "15m", 50)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch M15 candles: %w", err)
	}

	h1, err = uc.fetchClosedCandlesCached(ctx, symbol, "1h", 50)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch H1 candles: %w", err)
	}

	// Keep consistent with FetchMarketData() to avoid systemic playbook ineligibility.
	const h4TrendCandleLimit = 210
	h4, err = uc.fetchClosedCandlesCached(ctx, symbol, "4h", h4TrendCandleLimit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch H4 candles: %w", err)
	}

	return m15, h1, h4, nil
}

func (uc *MarketDataUsecase) fetchClosedCandlesCached(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	cacheKey := fmt.Sprintf("%s|%s|%d", symbol, interval, limit)
	now := time.Now()

	uc.candleCacheMu.RLock()
	if cached, ok := uc.candleCache[cacheKey]; ok && now.Before(cached.validUntil) && len(cached.candles) > 0 {
		out := append([]dto.Candle(nil), cached.candles...)
		uc.candleCacheMu.RUnlock()
		return out, nil
	}
	uc.candleCacheMu.RUnlock()

	candles, err := uc.provider.FetchClosedCandles(ctx, symbol, interval, limit)
	if err != nil {
		return nil, err
	}

	validUntil := now.Add(15 * time.Second)
	if next := nextClosedCandleAvailability(candles, interval); !next.IsZero() && next.After(now) {
		validUntil = next
	}

	cloned := append([]dto.Candle(nil), candles...)
	uc.candleCacheMu.Lock()
	pruneExpiredClosedCandleCache(uc.candleCache, now, maxClosedCandleCacheEntries)
	uc.candleCache[cacheKey] = cachedClosedCandles{
		candles:    cloned,
		validUntil: validUntil,
	}
	pruneExpiredClosedCandleCache(uc.candleCache, now, maxClosedCandleCacheEntries)
	uc.candleCacheMu.Unlock()

	return append([]dto.Candle(nil), cloned...), nil
}

func (uc *MarketDataUsecase) fetchOpenInterestCached(ctx context.Context, symbol string) (float64, error) {
	now := time.Now()

	uc.oiCacheMu.RLock()
	if cached, ok := uc.oiCache[symbol]; ok && now.Before(cached.validUntil) {
		uc.oiCacheMu.RUnlock()
		return cached.value, nil
	}
	uc.oiCacheMu.RUnlock()

	value, err := uc.provider.FetchOpenInterest(ctx, symbol)
	if err != nil {
		return 0, err
	}

	uc.oiCacheMu.Lock()
	pruneExpiredFloatCache(uc.oiCache, now, maxOpenInterestCacheEntries)
	uc.oiCache[symbol] = cachedFloat64{
		value:      value,
		validUntil: now.Add(30 * time.Second),
	}
	pruneExpiredFloatCache(uc.oiCache, now, maxOpenInterestCacheEntries)
	uc.oiCacheMu.Unlock()

	return value, nil
}

func nextClosedCandleAvailability(candles []dto.Candle, interval string) time.Time {
	if len(candles) == 0 {
		return time.Time{}
	}

	tf, ok := intervalToDuration(interval)
	if !ok || tf <= 0 {
		return time.Time{}
	}

	lastOpen := candles[len(candles)-1].Time
	if lastOpen.IsZero() {
		return time.Time{}
	}

	return lastOpen.Add(tf * 2)
}

func pruneExpiredClosedCandleCache(cache map[string]cachedClosedCandles, now time.Time, maxEntries int) {
	for key, cached := range cache {
		if !now.Before(cached.validUntil) {
			delete(cache, key)
		}
	}
	if len(cache) <= maxEntries {
		return
	}
	for key := range cache {
		delete(cache, key)
		if len(cache) <= maxEntries {
			return
		}
	}
}

func pruneExpiredFloatCache(cache map[string]cachedFloat64, now time.Time, maxEntries int) {
	for key, cached := range cache {
		if !now.Before(cached.validUntil) {
			delete(cache, key)
		}
	}
	if len(cache) <= maxEntries {
		return
	}
	for key := range cache {
		delete(cache, key)
		if len(cache) <= maxEntries {
			return
		}
	}
}

func (uc *MarketDataUsecase) FetchM5Candles(ctx context.Context, symbol string, limit int) ([]dto.Candle, error) {
	return uc.fetchClosedCandlesCached(ctx, symbol, "5m", limit)
}

func intervalToDuration(interval string) (time.Duration, bool) {
	switch interval {
	case "5m":
		return 5 * time.Minute, true
	case "15m":
		return 15 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "4h":
		return 4 * time.Hour, true
	default:
		return 0, false
	}
}
