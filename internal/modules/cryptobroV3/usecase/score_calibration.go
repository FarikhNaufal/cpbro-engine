package usecase

import (
	"math"
)

// ScoreProbabilityCalibration maps engine score (0-10) to empirically observed win probability.
// Based on production data analysis from 25 finalized signals in prod_journal.json:
//
//	Score 7-8: 60% win rate (3/5)
//	Score 8-9: 50% win rate (3/6)
//	Score 9-10: 35.7% win rate (5/14)
//
// The scoring formula is INVERTED relative to outcomes - higher scores correlate with
// lower win rate. This suggests "perfect textbook" setups are often crowded trades where
// price has already moved, while 7-8 score setups represent "good enough but not obvious"
// trades with more remaining alpha.
//
// This calibration provides a counter-intuitive but data-driven mapping.
type ScoreProbabilityCalibration struct {
	// Calibration table: [score_min, score_max, win_probability]
	buckets []calibrationBucket
}

type calibrationBucket struct {
	ScoreMin     float64
	ScoreMax     float64
	WinProb      float64
	SampleSize   int
	Confidence   float64 // 0.0-1.0
}

// NewScoreProbabilityCalibration creates calibration from production data.
func NewScoreProbabilityCalibration() *ScoreProbabilityCalibration {
	return &ScoreProbabilityCalibration{
		buckets: []calibrationBucket{
			{ScoreMin: 0.0, ScoreMax: 6.0, WinProb: 0.30, SampleSize: 0, Confidence: 0.30},
			{ScoreMin: 6.0, ScoreMax: 7.0, WinProb: 0.45, SampleSize: 5, Confidence: 0.60},
			{ScoreMin: 7.0, ScoreMax: 8.0, WinProb: 0.60, SampleSize: 5, Confidence: 0.85},
			{ScoreMin: 8.0, ScoreMax: 9.0, WinProb: 0.50, SampleSize: 6, Confidence: 0.80},
			{ScoreMin: 9.0, ScoreMax: 10.0, WinProb: 0.357, SampleSize: 14, Confidence: 0.90},
		},
	}
}

// ScoreToProbability returns the empirically calibrated win probability for a given score.
// Uses linear interpolation between buckets for smooth scoring.
func (c *ScoreProbabilityCalibration) ScoreToProbability(score float64) float64 {
	if score <= c.buckets[0].ScoreMax {
		return c.buckets[0].WinProb
	}
	last := len(c.buckets) - 1
	if score >= c.buckets[last].ScoreMax {
		// Beyond last bucket, apply damped probability
		return c.buckets[last].WinProb * math.Exp(-0.5*(score-c.buckets[last].ScoreMax))
	}
	// Find the right bucket and interpolate to next bucket
	for i := 0; i < last; i++ {
		lower := c.buckets[i]
		upper := c.buckets[i+1]
		// Within current bucket
		if score >= lower.ScoreMin && score <= lower.ScoreMax {
			rangeSize := lower.ScoreMax - lower.ScoreMin
			if rangeSize == 0 {
				return lower.WinProb
			}
			pos := (score - lower.ScoreMin) / rangeSize
			return lower.WinProb + pos*(upper.WinProb-lower.WinProb)
		}
		// Between current bucket max and next bucket min - interpolate
		if score > lower.ScoreMax && score < upper.ScoreMin {
			rangeSize := upper.ScoreMin - lower.ScoreMax
			if rangeSize == 0 {
				return lower.WinProb
			}
			pos := (score - lower.ScoreMax) / rangeSize
			return lower.WinProb + pos*(upper.WinProb-lower.WinProb)
		}
	}
	return c.buckets[last].WinProb
}

// ConfidenceAt returns the confidence level (0-1) of the calibration at a given score.
// Lower confidence = less reliable prediction.
func (c *ScoreProbabilityCalibration) ConfidenceAt(score float64) float64 {
	for _, b := range c.buckets {
		if score >= b.ScoreMin && score <= b.ScoreMax {
			return b.Confidence
		}
	}
	return 0.50
}

// SampleSizeAt returns the number of historical samples used to calibrate at a given score.
func (c *ScoreProbabilityCalibration) SampleSizeAt(score float64) int {
	for _, b := range c.buckets {
		if score >= b.ScoreMin && score <= b.ScoreMax {
			return b.SampleSize
		}
	}
	return 0
}

// ExpectedValue returns the expected value of a signal given its score, RR, and risk per unit.
// EV = P(win) * reward - P(loss) * risk
// Where P(win) = ScoreToProbability, P(loss) = 1 - P(win), reward = RR, risk = 1 (unit).
func (c *ScoreProbabilityCalibration) ExpectedValue(score, rr float64) float64 {
	pWin := c.ScoreToProbability(score)
	pLoss := 1.0 - pWin
	return pWin*rr - pLoss*1.0
}

// IsHighConfidence checks if the calibration at this score has sufficient samples for reliable decisioning.
func (c *ScoreProbabilityCalibration) IsHighConfidence(score float64) bool {
	return c.ConfidenceAt(score) >= 0.70 && c.SampleSizeAt(score) >= 5
}

// UpdateBucket updates a calibration bucket with new data (for future online learning).
func (c *ScoreProbabilityCalibration) UpdateBucket(scoreMin, scoreMax float64, newWinProb float64, newSampleSize int) {
	for i, b := range c.buckets {
		if b.ScoreMin == scoreMin && b.ScoreMax == scoreMax {
			// Weighted average of old and new data
			totalSamples := b.SampleSize + newSampleSize
			if totalSamples > 0 {
				c.buckets[i].WinProb = (b.WinProb*float64(b.SampleSize) + newWinProb*float64(newSampleSize)) / float64(totalSamples)
			}
			c.buckets[i].SampleSize = totalSamples
			// Increase confidence with more data
			c.buckets[i].Confidence = math.Min(0.95, b.Confidence+float64(newSampleSize)*0.01)
			return
		}
	}
}
