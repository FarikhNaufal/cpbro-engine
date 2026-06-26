# Next Move Handoff

Last updated: 2026-06-14

## Current reality

- Production `/api/v3/evaluation` showed `23` signals with `39.13%` win rate.
- Playbook distribution is dominated by `COMPRESSION_BREAKOUT_RETEST` and that playbook is underperforming at `35.71%` win rate.
- Live `/api/v3/latest` at the time of audit showed `total_playbook_eligible=0` and `total_quant_candidates=0` while market regime was forced into `COMPRESSION`.
- `/health` was degraded because `gemini` timed out and Binance websocket was disconnected.

## Best next moves

### P1 — Fix regime lock before tuning more thresholds

Why:
- Current regime classification can force the system into `COMPRESSION` from broad BTC-only conditions.
- When that happens, selector pressure becomes too narrow and good alt setups can be missed.

Primary code areas:
- `internal/modules/cryptobroV3/usecase/scanner.go`
- `internal/modules/cryptobroV3/usecase/market_policy.go`
- `internal/modules/cryptobroV3/usecase/strategy_selector.go`

Target outcome:
- `COMPRESSION` is no longer the default sink when BTC is quiet.
- Selector can degrade gracefully to non-compression setups when symbol-level evidence does not support breakout conditions.

### P2 — Replace fake compression proxy with true Bollinger width

Why:
- Current compression checks still rely on ATR-style approximation in part of the pipeline.
- That makes `COMPRESSION_BREAKOUT_RETEST` less trustworthy and can both miss valid setups and admit weak ones.

Primary code areas:
- `internal/modules/cryptobroV3/usecase/helpers.go`
- `internal/modules/cryptobroV3/usecase/playbook_eligibility.go`

Target outcome:
- Compression eligibility uses actual Bollinger bandwidth / contraction math.
- Funnel blockers become more meaningful.

### P3 — Wire volume thresholds consistently

Why:
- Some runtime profile knobs already exist, but several eligibility checks still use hardcoded volume comparisons.
- This makes tuning misleading because config changes do not fully propagate.

Primary code areas:
- `internal/modules/cryptobroV3/usecase/playbook_eligibility.go`
- `internal/modules/cryptobroV3/usecase/helpers.go`
- `internal/modules/cryptobroV3/usecase/threshold_profile.go`

Target outcome:
- `MinVolumeRatio` and related knobs drive actual decision behavior consistently across compression and sweep logic.

### P4 — Fix analytics truth layer

Why:
- Recommendations can become stale when evaluation logic still reflects old assumptions.
- `decision_audit` is not yet persisted to PocketBase, which limits reliable post-mortem analysis.

Primary code areas:
- `internal/modules/cryptobroV3/usecase/feedback.go`
- `internal/modules/cryptobroV3/service/pocketbase_storage.go`
- `internal/modules/cryptobroV3/transport/http/handler.go`

Target outcome:
- Evaluation recommendations match live config.
- Decision audit survives restarts and becomes queryable.

### P5 — Stabilize data plane before judging new tuning

Why:
- If Gemini times out or websocket disconnects, strategy quality gets mixed with infrastructure noise.

Primary code areas:
- deploy / infra / runtime config

Target outcome:
- `gemini_availability=OK`
- `binance_websocket=connected`
- stable fresh market feed during scan windows

## What the user should do

1. Deploy the latest code currently in local workspace to staging or production.
2. After deploy, verify:
   - `/health`
   - `/api/v3/latest`
   - `/api/v3/evaluation`
3. Confirm two runtime issues are gone first:
   - Gemini timeout
   - Binance websocket disconnected state
4. Let the system run for at least `48-72` hours after the next structural fixes before re-tuning thresholds again.
5. Send back these artifacts for the next audit:
   - latest `/api/v3/latest`
   - latest `/api/v3/evaluation`
   - 5 latest execute signals
   - 5 latest watch signals
   - `/health`

## What we should not do yet

- Do not keep tightening thresholds blindly while regime selection is still biased.
- Do not judge strategy quality only from execute count while infrastructure is degraded.
- Do not add many new indicators before the existing regime, compression, and volume wiring issues are fixed.

## Success criteria for the next cycle

- More than one playbook contributes signals.
- `COMPRESSION_BREAKOUT_RETEST` is no longer the overwhelming majority by count.
- Long/short performance gap narrows.
- Eligible candidates no longer collapse to zero in quiet BTC sessions.
- Win rate improves from the current `39.13%` without killing signal flow.
