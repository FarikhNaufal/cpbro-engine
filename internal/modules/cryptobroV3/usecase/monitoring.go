package usecase

import (
	"context"
	"sort"
	"time"
)

type MonitoringUsecase struct {
	marketDataProvider MarketDataProvider
	storageUsecase     *StorageUsecase
}

func NewMonitoringUsecase(provider MarketDataProvider, storage *StorageUsecase) *MonitoringUsecase {
	return &MonitoringUsecase{
		marketDataProvider: provider,
		storageUsecase:     storage,
	}
}

// MonitorVirtualPositions updates open recommendations (SL/TP trigger detection) using latest prices.
func (uc *MonitoringUsecase) MonitorVirtualPositions(ctx context.Context) error {
	journal, err := uc.storageUsecase.LoadSignalJournal()
	if err != nil {
		// Handle empty or missing file gracefully
		return nil
	}

	if len(journal) == 0 {
		return nil
	}

	now := time.Now()
	updated := false

	for i, item := range journal {
		// Only monitor active positions: either MONITORING, or TP1_HIT that has not expired yet.
		isActive := item.Status == MONITORING || (item.Status == TP1_HIT && now.Before(item.ExpiresAt))
		if !isActive {
			continue
		}

		updated = true

		// 1. Fetch closed candles to check historical outcomes (covers gaps/app restarts)
		candles, err := uc.marketDataProvider.FetchClosedCandles(ctx, item.Symbol, "15m", 20)
		if err == nil && len(candles) > 0 {
			// Sort candles chronologically
			sort.Slice(candles, func(a, b int) bool {
				return candles[a].Time.Before(candles[b].Time)
			})

			// Evaluate each candle that occurred within the monitoring window
			for _, c := range candles {
				// Only evaluate candles that closed AFTER the signal was created, and before expiry
				if c.Time.Before(item.CreatedAt) || c.Time.Equal(item.CreatedAt) {
					continue
				}
				if c.Time.After(item.ExpiresAt) {
					break
				}

				// Update MFE & MAE based on candle high/low
				if item.Direction == LONG {
					floatingProfitPercent := ((c.High - item.EntryPrice) / item.EntryPrice) * 100
					if floatingProfitPercent > item.MFE {
						item.MFE = floatingProfitPercent
					}
					floatingLossPercent := ((item.EntryPrice - c.Low) / item.EntryPrice) * 100
					if floatingLossPercent > item.MAE {
						item.MAE = floatingLossPercent
					}
				} else { // SHORT
					floatingProfitPercent := ((item.EntryPrice - c.Low) / item.EntryPrice) * 100
					if floatingProfitPercent > item.MFE {
						item.MFE = floatingProfitPercent
					}
					floatingLossPercent := ((c.High - item.EntryPrice) / item.EntryPrice) * 100
					if floatingLossPercent > item.MAE {
						item.MAE = floatingLossPercent
					}
				}

				// Check Stop Loss hit (SL check takes precedence)
				if item.Direction == LONG && c.Low <= item.StopLoss {
					item.Status = SL_HIT
					item.TimeToSL = c.Time.Sub(item.CreatedAt).String()
					item.OutcomeReason = "Stop Loss hit during candle evaluation"
					break
				}
				if item.Direction == SHORT && c.High >= item.StopLoss {
					item.Status = SL_HIT
					item.TimeToSL = c.Time.Sub(item.CreatedAt).String()
					item.OutcomeReason = "Stop Loss hit during candle evaluation"
					break
				}

				// Check Take Profit 1
				if item.Status == MONITORING {
					if item.Direction == LONG && c.High >= item.TP1 {
						item.Status = TP1_HIT
						item.TimeToTP1 = c.Time.Sub(item.CreatedAt).String()
						item.OutcomeReason = "TP1 hit during candle evaluation"
					}
					if item.Direction == SHORT && c.Low <= item.TP1 {
						item.Status = TP1_HIT
						item.TimeToTP1 = c.Time.Sub(item.CreatedAt).String()
						item.OutcomeReason = "TP1 hit during candle evaluation"
					}
				}

				// Check Take Profit 2 (only if TP1 has been hit)
				if item.Status == TP1_HIT {
					if item.Direction == LONG && c.High >= item.TP2 {
						item.Status = TP2_HIT
						item.TimeToTP2 = c.Time.Sub(item.CreatedAt).String()
						item.OutcomeReason = "TP2 hit during candle evaluation"
						break
					}
					if item.Direction == SHORT && c.Low <= item.TP2 {
						item.Status = TP2_HIT
						item.TimeToTP2 = c.Time.Sub(item.CreatedAt).String()
						item.OutcomeReason = "TP2 hit during candle evaluation"
						break
					}
				}
			}
		}

		// 2. Fetch latest live price for real-time validation (only if not already finalized to TP2/SL)
		if item.Status == MONITORING || item.Status == TP1_HIT {
			price, err := uc.marketDataProvider.FetchLatestPrice(ctx, item.Symbol)
			if err == nil {
				item.LatestPrice = price

				// Calculate live simulated PnL percentage
				if item.EntryPrice > 0 {
					if item.Direction == LONG {
						item.PnlPercentage = ((price - item.EntryPrice) / item.EntryPrice) * 100
					} else if item.Direction == SHORT {
						item.PnlPercentage = ((item.EntryPrice - price) / item.EntryPrice) * 100
					}
				}

				// Update live MFE & MAE
				if item.Direction == LONG {
					floatingProfitPercent := ((price - item.EntryPrice) / item.EntryPrice) * 100
					if floatingProfitPercent > item.MFE {
						item.MFE = floatingProfitPercent
					}
					floatingLossPercent := ((item.EntryPrice - price) / item.EntryPrice) * 100
					if floatingLossPercent > item.MAE {
						item.MAE = floatingLossPercent
					}
				} else { // SHORT
					floatingProfitPercent := ((item.EntryPrice - price) / item.EntryPrice) * 100
					if floatingProfitPercent > item.MFE {
						item.MFE = floatingProfitPercent
					}
					floatingLossPercent := ((price - item.EntryPrice) / item.EntryPrice) * 100
					if floatingLossPercent > item.MAE {
						item.MAE = floatingLossPercent
					}
				}

				// Check live SL/TP rules
				if item.Direction == LONG {
					if item.Status == MONITORING && price <= item.StopLoss {
						item.Status = SL_HIT
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "Stop Loss hit live"
					} else if item.Status == TP1_HIT && price <= item.StopLoss {
						// Checked out after TP1, remains TP1_HIT but halts monitoring
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "SL hit live after TP1 (partial success)"
						item.ExpiresAt = now.Add(-1 * time.Minute) // force expire
					} else if item.Status == MONITORING && price >= item.TP1 {
						item.Status = TP1_HIT
						item.TimeToTP1 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "TP1 hit live"
						if price >= item.TP2 {
							item.Status = TP2_HIT
							item.TimeToTP2 = time.Since(item.CreatedAt).String()
							item.OutcomeReason = "TP2 hit live"
						}
					} else if item.Status == TP1_HIT && price >= item.TP2 {
						item.Status = TP2_HIT
						item.TimeToTP2 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "TP2 hit live"
					}
				} else { // SHORT
					if item.Status == MONITORING && price >= item.StopLoss {
						item.Status = SL_HIT
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "Stop Loss hit live"
					} else if item.Status == TP1_HIT && price >= item.StopLoss {
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "SL hit live after TP1 (partial success)"
						item.ExpiresAt = now.Add(-1 * time.Minute) // force expire
					} else if item.Status == MONITORING && price <= item.TP1 {
						item.Status = TP1_HIT
						item.TimeToTP1 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "TP1 hit live"
						if price <= item.TP2 {
							item.Status = TP2_HIT
							item.TimeToTP2 = time.Since(item.CreatedAt).String()
							item.OutcomeReason = "TP2 hit live"
						}
					} else if item.Status == TP1_HIT && price <= item.TP2 {
						item.Status = TP2_HIT
						item.TimeToTP2 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = "TP2 hit live"
					}
				}
			}
		}

		// 3. Expiration Check (2 hours / 8 candles M15 elapsed)
		if item.Status == MONITORING || item.Status == TP1_HIT {
			// If we are past ExpiresAt or 120 minutes since creation
			if now.After(item.ExpiresAt) || now.Sub(item.CreatedAt) >= 120*time.Minute {
				if item.Status == MONITORING {
					item.Status = EXPIRED
					item.OutcomeReason = "Monitoring period expired (120 minutes elapsed) without hitting SL or TP1"
				} else if item.Status == TP1_HIT {
					// Stays TP1_HIT (partial success) but stop monitoring
					item.OutcomeReason = "Monitoring period expired (120 minutes elapsed) with TP1 success"
				}
			}
		}

		// Update fields
		item.UpdatedAt = now
		if item.Status != MONITORING && item.Status != TP1_HIT {
			item.ClosedAt = now
		}
		journal[i] = item
	}

	if updated {
		return uc.storageUsecase.SaveSignalJournal(journal)
	}

	return nil
}
