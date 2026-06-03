package usecase

import (
	"context"
	"sync"
	"testing"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
	"github.com/stretchr/testify/require"
)

type recordingMarketDataProvider struct {
	mu              sync.Mutex
	calls           []fetchClosedCall
	openInterestHit int
}

type fetchClosedCall struct {
	interval string
	limit    int
}

func (p *recordingMarketDataProvider) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	p.mu.Lock()
	p.calls = append(p.calls, fetchClosedCall{interval: interval, limit: limit})
	p.mu.Unlock()

	// Return "closed candles" with deterministic open-times.
	// The implementation under test treats dto.Candle.Time as open-time.
	candles := make([]dto.Candle, limit)
	now := time.Now().UTC().Truncate(time.Hour)
	step := 15 * time.Minute
	switch interval {
	case "1h":
		step = time.Hour
	case "4h":
		step = 4 * time.Hour
	}
	start := now.Add(-step * time.Duration(limit))
	for i := 0; i < limit; i++ {
		candles[i] = dto.Candle{
			Time:  start.Add(step * time.Duration(i)),
			Open:  100,
			High:  101,
			Low:   99,
			Close: 100,
			Vol:   1,
		}
	}
	return candles, nil
}

func (p *recordingMarketDataProvider) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}

func (p *recordingMarketDataProvider) FetchAllFuturesTickers24h(ctx context.Context) ([]dto.Ticker24h, error) {
	return nil, nil
}

func (p *recordingMarketDataProvider) FetchPremiumFundingRates(ctx context.Context) (map[string]float64, error) {
	return nil, nil
}

func (p *recordingMarketDataProvider) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	p.mu.Lock()
	p.openInterestHit++
	p.mu.Unlock()
	return 1000000, nil
}

func (p *recordingMarketDataProvider) FetchHistoricalCandles(ctx context.Context, symbol string, interval string, startTime time.Time, endTime time.Time) ([]dto.Candle, error) {
	return nil, nil
}

func TestMarketDataUsecase_FetchMarketData_H4LimitSupportsEMA200(t *testing.T) {
	provider := &recordingMarketDataProvider{}
	uc := NewMarketDataUsecase(provider)

	_, err := uc.FetchMarketData(context.Background(), "BTCUSDT", map[string]float64{})
	require.NoError(t, err)

	provider.mu.Lock()
	defer provider.mu.Unlock()

	h4Limit := 0
	for _, c := range provider.calls {
		if c.interval == "4h" {
			h4Limit = c.limit
			break
		}
	}
	require.GreaterOrEqual(t, h4Limit, 200, "H4 candle limit must support EMA(200) trend checks for TREND_PULLBACK")
}

func TestMarketDataUsecase_FetchCandles_ReusesClosedCandleCacheWithinWindow(t *testing.T) {
	provider := &recordingMarketDataProvider{}
	uc := NewMarketDataUsecase(provider)

	_, _, _, err := uc.FetchCandles(context.Background(), "BTCUSDT")
	require.NoError(t, err)
	_, _, _, err = uc.FetchCandles(context.Background(), "BTCUSDT")
	require.NoError(t, err)

	provider.mu.Lock()
	defer provider.mu.Unlock()
	require.Len(t, provider.calls, 3, "expected cached second fetch to avoid refetching the same closed candles")
}

func TestMarketDataUsecase_FetchInitialMarketData_OnlyFetchesM15(t *testing.T) {
	provider := &recordingMarketDataProvider{}
	uc := NewMarketDataUsecase(provider)

	md, err := uc.FetchInitialMarketData(context.Background(), "BTCUSDT", map[string]float64{"BTCUSDT": 0.001})
	require.NoError(t, err)
	require.NotEmpty(t, md.M15Candles)
	require.Empty(t, md.H1Candles)
	require.Empty(t, md.H4Candles)
	require.Equal(t, 0.001, md.FundingRate)

	provider.mu.Lock()
	defer provider.mu.Unlock()
	require.Len(t, provider.calls, 1)
	require.Equal(t, "15m", provider.calls[0].interval)
	require.Equal(t, 0, provider.openInterestHit)
}

func TestMarketDataUsecase_EnrichMarketData_ReusesOpenInterestCache(t *testing.T) {
	provider := &recordingMarketDataProvider{}
	uc := NewMarketDataUsecase(provider)

	initial, err := uc.FetchInitialMarketData(context.Background(), "BTCUSDT", map[string]float64{})
	require.NoError(t, err)

	_, err = uc.EnrichMarketData(context.Background(), initial)
	require.NoError(t, err)
	_, err = uc.EnrichMarketData(context.Background(), initial)
	require.NoError(t, err)

	provider.mu.Lock()
	defer provider.mu.Unlock()
	require.Equal(t, 1, provider.openInterestHit, "expected open interest to be cached across enrich calls")
}
