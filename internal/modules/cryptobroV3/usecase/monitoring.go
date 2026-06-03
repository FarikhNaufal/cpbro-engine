package usecase

import (
	"context"
	"sort"
	"time"

	"cpbro-engine/internal/modules/cryptobroV3/dto"
)

type MonitoringUsecase struct {
	marketDataProvider MarketDataProvider
	storageUsecase     *StorageUsecase
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

var executeMonitoringProfile = monitoringStatusProfile{
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
	ExpiredMonitoring: "Monitoring period expired (120 minutes elapsed) without hitting SL or TP1",
	ExpiredAfterTP1:   "Monitoring period expired (120 minutes elapsed) with TP1 success",
}

var watchMonitoringProfile = monitoringStatusProfile{
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
	ExpiredMonitoring: "Virtual monitoring period expired (120 minutes elapsed) without hitting virtual SL or TP1",
	ExpiredAfterTP1:   "Virtual monitoring period expired (120 minutes elapsed) with virtual TP1 success",
}

func NewMonitoringUsecase(provider MarketDataProvider, storage *StorageUsecase) *MonitoringUsecase {
	return &MonitoringUsecase{
		marketDataProvider: provider,
		storageUsecase:     storage,
	}
}

// MonitorVirtualPositions updates both actionable FINAL_EXECUTE signals and
// non-actionable FINAL_WATCH paper setups without ever upgrading watch to execute.
func (uc *MonitoringUsecase) MonitorVirtualPositions(ctx context.Context) error {
	if err := uc.monitorSignalJournal(ctx); err != nil {
		return err
	}
	if err := uc.monitorWatchJournal(ctx); err != nil {
		return err
	}
	return nil
}

func (uc *MonitoringUsecase) monitorSignalJournal(ctx context.Context) error {
	journal, err := uc.storageUsecase.LoadSignalJournal()
	if err != nil || len(journal) == 0 {
		return nil
	}

	now := time.Now()
	updatedJournal, changed := monitorJournalEntries(ctx, uc.marketDataProvider, journal, now, executeMonitoringProfile)
	if !changed {
		return nil
	}

	return uc.storageUsecase.UpdateSignalJournal(func(current []SignalJournal) ([]SignalJournal, error) {
		if len(current) == len(updatedJournal) {
			return updatedJournal, nil
		}
		for i := range current {
			if current[i].ID == "" {
				continue
			}
			for _, upd := range updatedJournal {
				if upd.ID == current[i].ID {
					current[i] = upd
					break
				}
			}
		}
		return current, nil
	})
}

func (uc *MonitoringUsecase) monitorWatchJournal(ctx context.Context) error {
	journal, err := uc.storageUsecase.LoadWatchJournal()
	if err != nil || len(journal) == 0 {
		return nil
	}

	now := time.Now()
	updatedJournal, changed := monitorJournalEntries(ctx, uc.marketDataProvider, journal, now, watchMonitoringProfile)
	if !changed {
		return nil
	}

	return uc.storageUsecase.UpdateWatchJournal(func(current []WatchJournal) ([]WatchJournal, error) {
		if len(current) == len(updatedJournal) {
			return updatedJournal, nil
		}
		for i := range current {
			if current[i].ID == "" {
				continue
			}
			for _, upd := range updatedJournal {
				if upd.ID == current[i].ID {
					current[i] = upd
					break
				}
			}
		}
		return current, nil
	})
}

func monitorJournalEntries(ctx context.Context, provider MarketDataProvider, journal []SignalJournal, now time.Time, profile monitoringStatusProfile) ([]SignalJournal, bool) {
	changed := false

	for i := range journal {
		item := journal[i]
		if !isActiveMonitoringStatus(item.Status, profile, now, item.ExpiresAt) {
			continue
		}

		candles, err := provider.FetchClosedCandles(ctx, item.Symbol, "15m", 20)
		if err == nil && len(candles) > 0 {
			sort.Slice(candles, func(a, b int) bool {
				return candles[a].Time.Before(candles[b].Time)
			})

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
					item.OutcomeReason = profile.CandleSLReason
					break
				}

				if item.Status == profile.Monitoring && isTP1Hit(item.Direction, c, item.TP1) {
					item.Status = profile.TP1Hit
					item.TimeToTP1 = c.Time.Sub(item.CreatedAt).String()
					item.OutcomeReason = profile.CandleTP1Reason
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
			price, err := provider.FetchLatestPrice(ctx, item.Symbol)
			if err == nil {
				item.LatestPrice = price
				updateLivePnl(&item, price)
				updateExcursionFromPrice(&item, price)

				if item.Direction == LONG {
					switch {
					case item.Status == profile.Monitoring && price <= item.StopLoss:
						item.Status = profile.SLHit
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveSLReason
					case item.Status == profile.TP1Hit && price <= item.StopLoss:
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveSLAfterTP1
						item.ExpiresAt = now.Add(-1 * time.Minute)
					case item.Status == profile.Monitoring && price >= item.TP1:
						item.Status = profile.TP1Hit
						item.TimeToTP1 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveTP1Reason
						if price >= item.TP2 {
							item.Status = profile.TP2Hit
							item.TimeToTP2 = time.Since(item.CreatedAt).String()
							item.OutcomeReason = profile.LiveTP2Reason
						}
					case item.Status == profile.TP1Hit && price >= item.TP2:
						item.Status = profile.TP2Hit
						item.TimeToTP2 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveTP2Reason
					}
				} else {
					switch {
					case item.Status == profile.Monitoring && price >= item.StopLoss:
						item.Status = profile.SLHit
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveSLReason
					case item.Status == profile.TP1Hit && price >= item.StopLoss:
						item.TimeToSL = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveSLAfterTP1
						item.ExpiresAt = now.Add(-1 * time.Minute)
					case item.Status == profile.Monitoring && price <= item.TP1:
						item.Status = profile.TP1Hit
						item.TimeToTP1 = time.Since(item.CreatedAt).String()
						item.OutcomeReason = profile.LiveTP1Reason
						if price <= item.TP2 {
							item.Status = profile.TP2Hit
							item.TimeToTP2 = time.Since(item.CreatedAt).String()
							item.OutcomeReason = profile.LiveTP2Reason
						}
					case item.Status == profile.TP1Hit && price <= item.TP2:
						item.Status = profile.TP2Hit
						item.TimeToTP2 = time.Since(item.CreatedAt).String()
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
					item.OutcomeReason = profile.ExpiredAfterTP1
				}
			}
		}

		item.UpdatedAt = now
		if item.Status != profile.Monitoring && item.Status != profile.TP1Hit {
			item.ClosedAt = now
		}
		journal[i] = item
		changed = true
	}

	return journal, changed
}

func isActiveMonitoringStatus(status Status, profile monitoringStatusProfile, now time.Time, expiresAt time.Time) bool {
	return status == profile.Monitoring || (status == profile.TP1Hit && now.Before(expiresAt))
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
	if item.Direction == LONG {
		item.PnlPercentage = ((price - item.EntryPrice) / item.EntryPrice) * 100
		return
	}
	if item.Direction == SHORT {
		item.PnlPercentage = ((item.EntryPrice - price) / item.EntryPrice) * 100
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
