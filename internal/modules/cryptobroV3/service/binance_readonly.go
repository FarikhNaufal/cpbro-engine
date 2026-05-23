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
	client *futures.Client
}

func NewBinanceReadonlyService(apiKey, apiSecret string) *BinanceReadonlyService {
	// Note: We use read-only client config. No execution capability allowed.
	return &BinanceReadonlyService{
		client: futures.NewClient(apiKey, apiSecret),
	}
}

// FetchClosedCandles returns historical closed candle data from Binance Futures.
// Strictly uses closed candles for indicators.
func (s *BinanceReadonlyService) FetchClosedCandles(ctx context.Context, symbol string, interval string, limit int) ([]dto.Candle, error) {
	klines, err := s.client.NewKlinesService().
		Symbol(symbol).
		Interval(interval).
		Limit(limit + 1). // +1 to exclude the currently open/active candle
		Do(ctx)
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
		k := klines[i]
		openTime := time.UnixMilli(k.OpenTime)
		open, _ := strconv.ParseFloat(k.Open, 64)
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)
		closePrice, _ := strconv.ParseFloat(k.Close, 64)
		vol, _ := strconv.ParseFloat(k.Volume, 64)

		candles = append(candles, dto.Candle{
			Time:  openTime,
			Open:  open,
			High:  high,
			Low:   low,
			Close: closePrice,
			Vol:   vol,
		})
	}

	return candles, nil
}

// FetchLatestPrice fetches the current ticker price (only for staleness validation/monitoring).
func (s *BinanceReadonlyService) FetchLatestPrice(ctx context.Context, symbol string) (float64, error) {
	prices, err := s.client.NewListPricesService().Symbol(symbol).Do(ctx)
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
	stats, err := s.client.NewListPriceChangeStatsService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all futures tickers: %w", err)
	}

	var tickers []dto.Ticker24h
	for _, item := range stats {
		pricePercent, _ := strconv.ParseFloat(item.PriceChangePercent, 64)
		lastPrice, _ := strconv.ParseFloat(item.LastPrice, 64)
		volume, _ := strconv.ParseFloat(item.Volume, 64)
		quoteVol, _ := strconv.ParseFloat(item.QuoteVolume, 64)

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
	rates, err := s.client.NewPremiumIndexService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch premium/funding rates: %w", err)
	}

	result := make(map[string]float64)
	for _, rate := range rates {
		fundingRate, _ := strconv.ParseFloat(rate.LastFundingRate, 64)
		result[rate.Symbol] = fundingRate
	}
	return result, nil
}

// FetchOpenInterest fetches the current open interest for a symbol.
func (s *BinanceReadonlyService) FetchOpenInterest(ctx context.Context, symbol string) (float64, error) {
	oi, err := s.client.NewGetOpenInterestService().Symbol(symbol).Do(ctx)
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
		klines, err := s.client.NewKlinesService().
			Symbol(symbol).
			Interval(interval).
			StartTime(currentStart).
			EndTime(endMilli).
			Limit(1000).
			Do(ctx)
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

			openTime := time.UnixMilli(k.OpenTime)
			open, _ := strconv.ParseFloat(k.Open, 64)
			high, _ := strconv.ParseFloat(k.High, 64)
			low, _ := strconv.ParseFloat(k.Low, 64)
			closePrice, _ := strconv.ParseFloat(k.Close, 64)
			vol, _ := strconv.ParseFloat(k.Volume, 64)

			candles = append(candles, dto.Candle{
				Time:  openTime,
				Open:  open,
				High:  high,
				Low:   low,
				Close: closePrice,
				Vol:   vol,
			})
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
