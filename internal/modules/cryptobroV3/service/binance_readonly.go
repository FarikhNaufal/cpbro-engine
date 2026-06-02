package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"github.com/adshao/go-binance/v2/futures"
)

type BinanceReadonlyService struct {
	client         *futures.Client
	requestTimeout time.Duration
	maxRetry       int
	retryBackoff   time.Duration
}

func NewBinanceReadonlyService(apiKey, apiSecret string) *BinanceReadonlyService {
	return NewBinanceReadonlyServiceWithOptions(apiKey, apiSecret, 15*time.Second, 2, 300*time.Millisecond)
}

func NewBinanceReadonlyServiceWithOptions(apiKey, apiSecret string, requestTimeout time.Duration, maxRetry int, retryBackoff time.Duration) *BinanceReadonlyService {
	if requestTimeout <= 0 {
		requestTimeout = 15 * time.Second
	}
	if maxRetry < 0 {
		maxRetry = 0
	}
	if retryBackoff <= 0 {
		retryBackoff = 300 * time.Millisecond
	}
	// Note: We use read-only client config. No execution capability allowed.
	return &BinanceReadonlyService{
		client:         futures.NewClient(apiKey, apiSecret),
		requestTimeout: requestTimeout,
		maxRetry:       maxRetry,
		retryBackoff:   retryBackoff,
	}
}

func (s *BinanceReadonlyService) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.requestTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, s.requestTimeout)
}

func (s *BinanceReadonlyService) withRetry(ctx context.Context, op func(context.Context) error) error {
	attempts := s.maxRetry + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		opCtx, cancel := s.withTimeout(ctx)
		err := op(opCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == attempts {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * s.retryBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func parseFuturesCandle(symbol, interval string, k *futures.Kline) (dto.Candle, error) {
	open, err := strconv.ParseFloat(k.Open, 64)
	if err != nil {
		return dto.Candle{}, fmt.Errorf("failed to parse open for %s (%s): %w", symbol, interval, err)
	}
	high, err := strconv.ParseFloat(k.High, 64)
	if err != nil {
		return dto.Candle{}, fmt.Errorf("failed to parse high for %s (%s): %w", symbol, interval, err)
	}
	low, err := strconv.ParseFloat(k.Low, 64)
	if err != nil {
		return dto.Candle{}, fmt.Errorf("failed to parse low for %s (%s): %w", symbol, interval, err)
	}
	closePrice, err := strconv.ParseFloat(k.Close, 64)
	if err != nil {
		return dto.Candle{}, fmt.Errorf("failed to parse close for %s (%s): %w", symbol, interval, err)
	}
	vol, err := strconv.ParseFloat(k.Volume, 64)
	if err != nil {
		return dto.Candle{}, fmt.Errorf("failed to parse volume for %s (%s): %w", symbol, interval, err)
	}
	return dto.Candle{
		Time:  time.UnixMilli(k.OpenTime),
		Open:  open,
		High:  high,
		Low:   low,
		Close: closePrice,
		Vol:   vol,
	}, nil
}

// FetchClosedCandles returns historical closed candle data from Binance Futures.
// Strictly uses closed candles for indicators.
func (s *BinanceReadonlyService) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	var klines []*futures.Kline
	err := s.withRetry(ctx, func(opCtx context.Context) error {
		var fetchErr error
		klines, fetchErr = s.client.NewKlinesService().
			Symbol(symbol).
			Interval(interval).
			Limit(limit + 1). // +1 to exclude the currently open/active candle
			Do(opCtx)
		return fetchErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch futures klines for %s (%s): %w", symbol, interval, err)
	}

	if len(klines) == 0 {
		return nil, nil
	}

	var candles []dto.Candle
	// We iterate up to len(klines)-1 to exclude the last (incomplete/active) candle
	// in order to guarantee we only use closed candles for indicators.
	closedCount := len(klines) - 1
	if closedCount > limit {
		closedCount = limit
	}

	for i := 0; i < closedCount; i++ {
		candle, err := parseFuturesCandle(symbol, interval, klines[i])
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}

	return candles, nil
}

// FetchLatestPrice fetches the current ticker price (only for staleness validation/monitoring).
func (s *BinanceReadonlyService) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	var prices []*futures.SymbolPrice
	err := s.withRetry(ctx, func(opCtx context.Context) error {
		var fetchErr error
		prices, fetchErr = s.client.NewListPricesService().Symbol(symbol).Do(opCtx)
		return fetchErr
	})
	if err != nil {
		return 0, fmt.Errorf("failed to fetch latest price for %s: %w", symbol, err)
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("no price returned for symbol %s", symbol)
	}
	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price for %s: %w", symbol, err)
	}
	return price, nil
}

// FetchAllFuturesTickers24h fetches stats for all tickers over the last 24h.
func (s *BinanceReadonlyService) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	var stats []*futures.PriceChangeStats
	err := s.withRetry(ctx, func(opCtx context.Context) error {
		var fetchErr error
		stats, fetchErr = s.client.NewListPriceChangeStatsService().Do(opCtx)
		return fetchErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all futures tickers: %w", err)
	}

	var tickers []dto.Ticker24h
	for _, item := range stats {
		pricePercent, err := strconv.ParseFloat(item.PriceChangePercent, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 24h price change for %s: %w", item.Symbol, err)
		}
		lastPrice, err := strconv.ParseFloat(item.LastPrice, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse last price for %s: %w", item.Symbol, err)
		}
		volume, err := strconv.ParseFloat(item.Volume, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse volume for %s: %w", item.Symbol, err)
		}
		quoteVol, err := strconv.ParseFloat(item.QuoteVolume, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse quote volume for %s: %w", item.Symbol, err)
		}

		tickers = append(tickers, dto.Ticker24h{
			Symbol:             item.Symbol,
			PriceChangePercent: pricePercent,
			LastPrice:          lastPrice,
			Volume:             volume,
			QuoteVolume:        quoteVol,
		})
	}
	return tickers, nil
}

// FetchPremiumFundingRates returns the map of funding rates for all active symbols.
func (s *BinanceReadonlyService) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	var rates []*futures.PremiumIndex
	err := s.withRetry(ctx, func(opCtx context.Context) error {
		var fetchErr error
		rates, fetchErr = s.client.NewPremiumIndexService().Do(opCtx)
		return fetchErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch premium/funding rates: %w", err)
	}

	result := make(map[string]float64)
	for _, rate := range rates {
		fundingRate, err := strconv.ParseFloat(rate.LastFundingRate, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse funding rate for %s: %w", rate.Symbol, err)
		}
		result[rate.Symbol] = fundingRate
	}
	return result, nil
}

// FetchOpenInterest fetches the current open interest for a symbol.
func (s *BinanceReadonlyService) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	var oi *futures.OpenInterest
	err := s.withRetry(ctx, func(opCtx context.Context) error {
		var fetchErr error
		oi, fetchErr = s.client.NewGetOpenInterestService().Symbol(symbol).Do(opCtx)
		return fetchErr
	})
	if err != nil {
		return 0, fmt.Errorf("failed to fetch open interest for %s: %w", symbol, err)
	}
	val, err := strconv.ParseFloat(oi.OpenInterest, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse open interest for %s: %w", symbol, err)
	}
	return val, nil
}

// FetchHistoricalCandles returns a complete range of closed candles from Binance Futures.
func (s *BinanceReadonlyService) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	var candles []dto.Candle
	currentStart := startTime.UnixMilli()
	endMilli := endTime.UnixMilli()

	for currentStart < endMilli {
		var klines []*futures.Kline
		err := s.withRetry(ctx, func(opCtx context.Context) error {
			var fetchErr error
			klines, fetchErr = s.client.NewKlinesService().
				Symbol(symbol).
				Interval(interval).
				StartTime(currentStart).
				EndTime(endMilli).
				Limit(1000).
				Do(opCtx)
			return fetchErr
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch futures historical klines for %s: %w", symbol, err)
		}
		if len(klines) == 0 {
			break
		}

		for _, k := range klines {
			closeTime := time.UnixMilli(k.CloseTime)
			if closeTime.After(time.Now()) {
				// Prevent lookahead bias with currently incomplete candles
				continue
			}

			candle, err := parseFuturesCandle(symbol, interval, k)
			if err != nil {
				return nil, err
			}
			candles = append(candles, candle)
		}

		// Advance start time pointer
		lastKline := klines[len(klines)-1]
		currentStart = lastKline.CloseTime + 1
	}

	return candles, nil
}

/*
   CRITICAL PROHIBITED FUNCTIONS (DO NOT ADD OR CALL):
   - NewCreateOrderService
   - NewCreateBatchOrdersService
   - NewCancelOrderService
   - NewChangeLeverageService
   - NewChangeMarginTypeService
*/
