package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/usecase"
)

type BinanceHotRankService struct {
	client   *http.Client
	cacheTTL time.Duration

	mu       sync.RWMutex
	cached   []usecase.HotSymbol
	cachedAt time.Time
}

func NewBinanceHotRankService() *BinanceHotRankService {
	return &BinanceHotRankService{
		client:   &http.Client{Timeout: 10 * time.Second},
		cacheTTL: 10 * time.Minute,
	}
}

func (s *BinanceHotRankService) FetchHotSymbols(ctx context.Context) ([]usecase.HotSymbol, error) {
	s.mu.RLock()
	if len(s.cached) > 0 && time.Since(s.cachedAt) < s.cacheTTL {
		defer s.mu.RUnlock()
		copied := make([]usecase.HotSymbol, len(s.cached))
		copy(copied, s.cached)
		return copied, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check cache in write lock
	if len(s.cached) > 0 && time.Since(s.cachedAt) < s.cacheTTL {
		copied := make([]usecase.HotSymbol, len(s.cached))
		copy(copied, s.cached)
		return copied, nil
	}

	slog.Info("Fetching hot symbols from Binance Web3 APIs...")

	var allHot []usecase.HotSymbol

	// 1. Trending & Top Search for chains "1", "56", "8453"
	targetChains := []string{"1", "56", "8453"}
	for _, chain := range targetChains {
		trending, err := s.fetchFromBapi(ctx, 10, chain, "Trending")
		if err == nil {
			allHot = append(allHot, trending...)
		} else {
			slog.Warn("Failed to fetch trending symbols from BAPI", "chainId", chain, "error", err)
		}

		topSearch, err := s.fetchFromBapi(ctx, 11, chain, "Top Search")
		if err == nil {
			allHot = append(allHot, topSearch...)
		} else {
			slog.Warn("Failed to fetch top search symbols from BAPI", "chainId", chain, "error", err)
		}
	}

	// 2. Social Hype for chains "56", "8453"
	socialChains := []string{"56", "8453"}
	for _, chain := range socialChains {
		hypeList, err := s.fetchSocialHype(ctx, chain)
		if err == nil {
			allHot = append(allHot, hypeList...)
		} else {
			slog.Warn("Failed to fetch social hype symbols from BAPI", "chainId", chain, "error", err)
		}
	}

	// 3. Smart Money Inflow for chains "56", "8453"
	smartMoneyChains := []string{"56", "8453"}
	for _, chain := range smartMoneyChains {
		inflowList, err := s.fetchSmartMoneyInflow(ctx, chain)
		if err == nil {
			allHot = append(allHot, inflowList...)
		} else {
			slog.Warn("Failed to fetch smart money inflow symbols from BAPI", "chainId", chain, "error", err)
		}
	}

	// Merge lists by symbol
	mergedMap := make(map[string]usecase.HotSymbol)
	for _, t := range allHot {
		symUpper := strings.ToUpper(strings.TrimSpace(t.Symbol))
		if symUpper == "" {
			continue
		}
		if existing, ok := mergedMap[symUpper]; ok {
			if !strings.Contains(existing.Source, t.Source) {
				existing.Source = existing.Source + ", " + t.Source
			}
			if t.Score > existing.Score {
				existing.Score = t.Score
			}
			mergedMap[symUpper] = existing
		} else {
			t.Symbol = symUpper
			mergedMap[symUpper] = t
		}
	}

	var result []usecase.HotSymbol
	for _, v := range mergedMap {
		result = append(result, v)
	}

	s.cached = result
	s.cachedAt = time.Now()

	slog.Info("Binance Web3 hot symbols fetched successfully", "count", len(result))

	copied := make([]usecase.HotSymbol, len(result))
	copy(copied, result)
	return copied, nil
}

type bapiPayload struct {
	RankType int    `json:"rankType"`
	ChainID  string `json:"chainId"`
	Page     int    `json:"page"`
	Size     int    `json:"size"`
}

type bapiToken struct {
	Symbol string `json:"symbol"`
}

type bapiData struct {
	Tokens []bapiToken `json:"tokens"`
}

type bapiResponse struct {
	Code    string   `json:"code"`
	Success bool     `json:"success"`
	Data    bapiData `json:"data"`
}

func (s *BinanceHotRankService) fetchFromBapi(ctx context.Context, rankType int, chainID string, sourceName string) ([]usecase.HotSymbol, error) {
	url := "https://web3.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/market/token/pulse/unified/rank/list/ai"
	payload := bapiPayload{
		RankType: rankType,
		ChainID:  chainID,
		Page:     1,
		Size:     100,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res bapiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success || res.Code != "000000" {
		return nil, fmt.Errorf("BAPI response failed with code: %s", res.Code)
	}

	var list []usecase.HotSymbol
	for i, t := range res.Data.Tokens {
		if t.Symbol == "" {
			continue
		}
		score := float64(100 - i)
		if score < 1.0 {
			score = 1.0
		}
		list = append(list, usecase.HotSymbol{
			Symbol:   t.Symbol,
			Score:    score,
			Source:   sourceName,
			RankType: rankType,
		})
	}

	return list, nil
}

type socialHypeMeta struct {
	Symbol string `json:"symbol"`
}

type socialHypeInfo struct {
	SocialHype float64 `json:"socialHype"`
}

type socialHypeItem struct {
	MetaInfo       socialHypeMeta `json:"metaInfo"`
	SocialHypeInfo socialHypeInfo `json:"socialHypeInfo"`
}

type socialHypeResponse struct {
	Code    string               `json:"code"`
	Success bool                 `json:"success"`
	Data    socialHypeDataBundle `json:"data"`
}

type socialHypeDataBundle struct {
	LeaderBoardList []socialHypeItem `json:"leaderBoardList"`
}

type smartMoneyItem struct {
	TokenName string  `json:"tokenName"`
	Ca        string  `json:"ca"`
	Inflow    float64 `json:"inflow"`
}

type smartMoneyResponse struct {
	Code    string           `json:"code"`
	Success bool             `json:"success"`
	Data    []smartMoneyItem `json:"data"`
}

type smartMoneyPayload struct {
	ChainID string `json:"chainId"`
	TagType int    `json:"tagType"`
}

func (s *BinanceHotRankService) fetchSocialHype(ctx context.Context, chainID string) ([]usecase.HotSymbol, error) {
	url := fmt.Sprintf("https://web3.binance.com/bapi/defi/v1/public/wallet-direct/buw/wallet/market/token/pulse/social/hype/rank/leaderboard?chainId=%s&sentiment=All&socialLanguage=ALL&targetLanguage=en&timeRange=1", chainID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "binance-web3/2.0 (Skill)")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res socialHypeResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success || res.Code != "000000" {
		return nil, fmt.Errorf("BAPI social-hype failed with code: %s", res.Code)
	}

	var list []usecase.HotSymbol
	for i, item := range res.Data.LeaderBoardList {
		sym := strings.ToUpper(strings.TrimSpace(item.MetaInfo.Symbol))
		if sym == "" {
			continue
		}
		score := float64(100 - i)
		if score < 1.0 {
			score = 1.0
		}
		list = append(list, usecase.HotSymbol{
			Symbol:   sym,
			Score:    score,
			Source:   "Social Hype",
			RankType: 30,
		})
	}

	return list, nil
}

func (s *BinanceHotRankService) fetchSmartMoneyInflow(ctx context.Context, chainID string) ([]usecase.HotSymbol, error) {
	url := "https://web3.binance.com/bapi/defi/v1/public/wallet-direct/tracker/wallet/token/inflow/rank/query/ai"
	payload := smartMoneyPayload{
		ChainID: chainID,
		TagType: 2,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "binance-web3/3.0 (Skill)")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var res smartMoneyResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if !res.Success || res.Code != "000000" {
		return nil, fmt.Errorf("BAPI smart-money inflow failed with code: %s", res.Code)
	}

	var list []usecase.HotSymbol
	for i, item := range res.Data {
		sym := strings.ToUpper(strings.TrimSpace(item.TokenName))
		if sym == "" {
			continue
		}
		score := float64(100 - i)
		if score < 1.0 {
			score = 1.0
		}
		list = append(list, usecase.HotSymbol{
			Symbol:   sym,
			Score:    score,
			Source:   "Smart Money Inflow",
			RankType: 40,
		})
	}

	return list, nil
}
