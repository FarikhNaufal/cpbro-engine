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
	mu    sync.Mutex
	calls []fetchClosedCall
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
