package usecase

import (
	"math"
	"testing"
)

func TestScoreProbabilityCalibration_BasicMapping(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// Test known mappings from production data (with linear interpolation between buckets)
	tests := []struct {
		score    float64
		expected float64
		delta    float64
	}{
		{5.0, 0.30, 0.01},
		// 6.5 is between bucket[1] (6.0-7.0, 0.45) and bucket[2] (7.0-8.0, 0.60)
		// Linear interp: 0.45 + 0.5*(0.60-0.45) = 0.525
		{6.5, 0.525, 0.01},
		// 7.5 is within bucket[2] (7.0-8.0, 0.60) → interpolate toward 8.0-9.0
		// Linear: 0.60 + 0.5*(0.50-0.60) = 0.55
		{7.5, 0.55, 0.01},
		// 8.5 is within bucket[3] (8.0-9.0, 0.50) → interpolate toward 9.0-10.0
		// Linear: 0.50 + 0.5*(0.357-0.50) = 0.4285
		{8.5, 0.4285, 0.01},
		// 9.5 is within bucket[4] (9.0-10.0, 0.357)
		{9.5, 0.357, 0.01},
	}

	for _, tc := range tests {
		got := cal.ScoreToProbability(tc.score)
		if math.Abs(got-tc.expected) > tc.delta {
			t.Errorf("ScoreToProbability(%v) = %v, want %v ± %v", tc.score, got, tc.expected, tc.delta)
		}
	}
}

func TestScoreProbabilityCalibration_Confidence(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// High sample sizes should have high confidence
	if conf := cal.ConfidenceAt(9.5); conf < 0.85 {
		t.Errorf("Expected high confidence at score 9.5 (n=14), got %v", conf)
	}

	// Low sample sizes should have low confidence
	if conf := cal.ConfidenceAt(5.0); conf > 0.50 {
		t.Errorf("Expected low confidence at score 5.0 (n=0), got %v", conf)
	}
}

func TestScoreProbabilityCalibration_IsHighConfidence(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// Score 9.5 should be high confidence (n=14)
	if !cal.IsHighConfidence(9.5) {
		t.Error("Expected score 9.5 to be high confidence")
	}

	// Score 5.0 should not be high confidence (n=0)
	if cal.IsHighConfidence(5.0) {
		t.Error("Expected score 5.0 to NOT be high confidence")
	}
}

func TestScoreProbabilityCalibration_ExpectedValue(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// Score 7.5 with RR 2.0
	// P(win) = 0.55 (with interpolation), EV = 0.55*2 - 0.45*1 = 0.65
	ev := cal.ExpectedValue(7.5, 2.0)
	expected := 0.55*2.0 - 0.45*1.0
	if math.Abs(ev-expected) > 0.01 {
		t.Errorf("ExpectedValue(7.5, 2.0) = %v, want %v", ev, expected)
	}

	// Score 9.5 with RR 1.5
	// P(win) = 0.357, EV = 0.357*1.5 - 0.643*1 = -0.1075 (NEGATIVE)
	ev2 := cal.ExpectedValue(9.5, 1.5)
	if ev2 >= 0 {
		t.Errorf("ExpectedValue(9.5, 1.5) should be negative (high score but low win rate), got %v", ev2)
	}
}

func TestScoreProbabilityCalibration_UpdateBucket(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// Initial: 7.0-8.0 bucket has 5 samples, 0.60 win prob
	initialSamples := cal.SampleSizeAt(7.5)

	// Add 10 new samples with 0.70 win prob
	cal.UpdateBucket(7.0, 8.0, 0.70, 10)

	// New weighted average: (5*0.6 + 10*0.7) / 15 = 0.6667
	// At score 7.5 (mid-bucket), interpolation toward 8.0 gives:
	// 0.6667 + 0.5*(0.50-0.6667) = 0.5833
	newWinProb := cal.ScoreToProbability(7.5)
	if math.Abs(newWinProb-0.5833) > 0.01 {
		t.Errorf("After update, ScoreToProbability(7.5) = %v, want ~0.5833", newWinProb)
	}

	if cal.SampleSizeAt(7.5) != initialSamples+10 {
		t.Errorf("After update, SampleSizeAt(7.5) = %v, want %v", cal.SampleSizeAt(7.5), initialSamples+10)
	}
}

func TestScoreProbabilityCalibration_DampedExtrapolation(t *testing.T) {
	cal := NewScoreProbabilityCalibration()

	// Above 10.0, probability should decay
	p10 := cal.ScoreToProbability(10.0)
	p12 := cal.ScoreToProbability(12.0)

	if p12 >= p10 {
		t.Errorf("Expected p12 < p10 (damped extrapolation), got p10=%v, p12=%v", p10, p12)
	}
}
