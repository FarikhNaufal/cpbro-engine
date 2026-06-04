package usecase

func resolvePlaybookPriority(regime MarketRegime, dir Direction, playbook Playbook) int {
	switch regime {
	case BTC_CHAOS:
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 1
		case CROWDED_POSITIONING_SQUEEZE:
			return 2
		default:
			return 100
		}
	case CHOP_RANGE:
		switch playbook {
		case RANGE_EDGE_REVERSAL:
			return 1
		case LIQUIDITY_SWEEP_REVERSAL:
			return 2
		case TREND_PULLBACK:
			return 3
		case CROWDED_POSITIONING_SQUEEZE:
			return 4
		case COMPRESSION_BREAKOUT_RETEST:
			return 5
		default:
			return 100
		}
	case RISK_OFF:
		if dir == LONG {
			switch playbook {
			case LIQUIDITY_SWEEP_REVERSAL:
				return 1
			case RANGE_EDGE_REVERSAL:
				return 2
			case CROWDED_POSITIONING_SQUEEZE:
				return 3
			case TREND_PULLBACK:
				return 4
			case COMPRESSION_BREAKOUT_RETEST:
				return 5
			default:
				return 100
			}
		}
		switch playbook {
		case LIQUIDITY_SWEEP_REVERSAL:
			return 1
		case RANGE_EDGE_REVERSAL:
			return 2
		case TREND_PULLBACK:
			return 3
		case CROWDED_POSITIONING_SQUEEZE:
			return 4
		case COMPRESSION_BREAKOUT_RETEST:
			return 5
		default:
			return 100
		}
	case ALT_SUPPORTIVE:
		if dir == SHORT {
			switch playbook {
			case LIQUIDITY_SWEEP_REVERSAL:
				return 1
			case RANGE_EDGE_REVERSAL:
				return 2
			case CROWDED_POSITIONING_SQUEEZE:
				return 3
			case TREND_PULLBACK:
				return 4
			case COMPRESSION_BREAKOUT_RETEST:
				return 5
			default:
				return 100
			}
		}
		switch playbook {
		case TREND_PULLBACK:
			return 1
		case COMPRESSION_BREAKOUT_RETEST:
			return 2
		case LIQUIDITY_SWEEP_REVERSAL:
			return 3
		case CROWDED_POSITIONING_SQUEEZE:
			return 4
		case RANGE_EDGE_REVERSAL:
			return 5
		default:
			return 100
		}
	case BTC_DOMINANCE:
		if dir == SHORT {
			switch playbook {
			case LIQUIDITY_SWEEP_REVERSAL:
				return 1
			case RANGE_EDGE_REVERSAL:
				return 2
			case CROWDED_POSITIONING_SQUEEZE:
				return 3
			case TREND_PULLBACK:
				return 4
			case COMPRESSION_BREAKOUT_RETEST:
				return 5
			default:
				return 100
			}
		}
		switch playbook {
		case TREND_PULLBACK:
			return 1
		case COMPRESSION_BREAKOUT_RETEST:
			return 2
		case LIQUIDITY_SWEEP_REVERSAL:
			return 3
		case RANGE_EDGE_REVERSAL:
			return 4
		case CROWDED_POSITIONING_SQUEEZE:
			return 5
		default:
			return 100
		}
	case COMPRESSION:
		switch playbook {
		case COMPRESSION_BREAKOUT_RETEST:
			return 1
		case LIQUIDITY_SWEEP_REVERSAL:
			return 2
		case TREND_PULLBACK:
			return 3
		case RANGE_EDGE_REVERSAL:
			return 4
		case CROWDED_POSITIONING_SQUEEZE:
			return 5
		default:
			return 100
		}
	default:
		switch playbook {
		case TREND_PULLBACK:
			return 1
		case LIQUIDITY_SWEEP_REVERSAL:
			return 2
		case COMPRESSION_BREAKOUT_RETEST:
			return 3
		case CROWDED_POSITIONING_SQUEEZE:
			return 4
		case RANGE_EDGE_REVERSAL:
			return 5
		default:
			return 100
		}
	}
}
