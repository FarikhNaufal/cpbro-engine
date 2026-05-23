package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type MarketDataUsecase struct {
	provider MarketDataProvider
}

func NewMarketDataUsecase(provider MarketDataProvider) *MarketDataUsecase {
	return &MarketDataUsecase{
		provider: provider,
	}
}

// FetchAllFuturesTickers24h fetches stats for all tickers.
func (uc *MarketDataUsecase) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return uc.provider.FetchAllFuturesTickers24h(timeoutCtx)
}

// FetchPremiumFundingRates fetches all active symbols funding rates.
func (uc *MarketDataUsecase) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return uc.provider.FetchPremiumFundingRates(timeoutCtx)
}

// FetchMarketData retrieves klines, open interest, funding rate, and latest price concurrently.
func (uc *MarketDataUsecase) FetchMarketData(ctx context.Context, symbol string, fundingRates map[string]float64) (MarketData, error) {
	rootCtx, cancelRoot := context.WithTimeout(ctx, 15*time.Second)
	defer cancelRoot()

	var (
		m15          []dto.Candle
		h1           []dto.Candle
		h4           []dto.Candle
		btcH1        []dto.Candle
		ethH1        []dto.Candle
		openInterest float64
		latestPrice  float64
	)

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
			name: "M15Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchClosedCandles(ctx, symbol, "15m", 50)
				if err != nil {
					return err
				}
				m15 = res
				return nil
			},
		},
		{
			name: "H1Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchClosedCandles(ctx, symbol, "1h", 50)
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
				res, err := uc.provider.FetchClosedCandles(ctx, symbol, "4h", 50)
				if err != nil {
					return err
				}
				h4 = res
				return nil
			},
		},
		{
			name: "BTCH1Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchClosedCandles(ctx, "BTCUSDT", "1h", 50)
				if err != nil {
					return err
				}
				btcH1 = res
				return nil
			},
		},
		{
			name: "ETHH1Candles",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchClosedCandles(ctx, "ETHUSDT", "1h", 50)
				if err != nil {
					return err
				}
				ethH1 = res
				return nil
			},
		},
		{
			name: "OpenInterest",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchOpenInterest(ctx, symbol)
				if err != nil {
					return err
				}
				openInterest = res
				return nil
			},
		},
		{
			name: "LatestPrice",
			fn: func(ctx context.Context) error {
				res, err := uc.provider.FetchLatestPrice(ctx, symbol)
				if err != nil {
					return err
				}
				latestPrice = res
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

	fundingRate := 0.0
	if val, ok := fundingRates[symbol]; ok {
		fundingRate = val
	}

	return MarketData{
		Symbol:          symbol,
		M15Candles:      m15,
		H1Candles:       h1,
		H4Candles:       h4,
		BTCH1Candles:    btcH1,
		ETHH1Candles:    ethH1,
		OpenInterestM15: openInterest,
		FundingRate:     fundingRate,
		LatestPrice:     latestPrice,
		LastUpdated:     time.Now(),
	}, nil
}

// FetchCandles fetches finalized candles for M15, H1, and H4 timeframes.
func (uc *MarketDataUsecase) FetchCandles(ctx context.Context, symbol string) (m15, h1, h4 []dto.Candle, err error) {
	m15, err = uc.provider.FetchClosedCandles(ctx, symbol, "15m", 50)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch M15 candles: %w", err)
	}

	h1, err = uc.provider.FetchClosedCandles(ctx, symbol, "1h", 50)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch H1 candles: %w", err)
	}

	h4, err = uc.provider.FetchClosedCandles(ctx, symbol, "4h", 50)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch H4 candles: %w", err)
	}

	return m15, h1, h4, nil
}
