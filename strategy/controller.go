package strategy

import (
	log "github.com/sirupsen/logrus"

	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/service"
)

/*
Controller manages strategy execution for a specific trading pair, handling data flow
between market feeds and strategy implementations. It maintains a dataframe buffer,
manages strategy lifecycle, and provides both complete and partial candle processing
for high-frequency and standard trading strategies.

Key Responsibilities:
  • Market data aggregation and buffering
  • Strategy warmup period management
  • Indicator calculation coordination
  • Trading signal execution via broker interface
  • Support for both standard and high-frequency strategies
*/
type Controller struct {
	strategy  Strategy         // Trading strategy implementation
	dataframe *model.Dataframe // Market data buffer for the trading pair
	broker    service.Broker   // Exchange interface for order execution
	started   bool             // Flag indicating if strategy is actively trading
}

/*
NewStrategyController creates a new strategy controller for a specific trading pair.
It initializes the dataframe buffer and links the strategy with the broker for
order execution.

Parameters:
  • pair: Trading pair symbol (e.g., "BTCUSDT")
  • strategy: Strategy implementation to execute
  • broker: Exchange interface for order placement

Returns configured controller ready for market data processing.
*/
func NewStrategyController(pair string, strategy Strategy, broker service.Broker) *Controller {
	// Initialize dataframe with pair info and metadata storage
	dataframe := &model.Dataframe{
		Pair:     pair,
		Metadata: make(map[string]model.Series[float64]),
	}

	return &Controller{
		dataframe: dataframe,
		strategy:  strategy,
		broker:    broker,
	}
}

// Start enables active trading for the strategy. Before calling Start, the strategy
// receives data for indicator calculation but does not execute trades.
func (s *Controller) Start() {
	s.started = true
}

/*
OnPartialCandle processes incomplete candle updates for high-frequency trading strategies.
This method enables strategies to react to intra-candle price movements for
scalping and other time-sensitive trading approaches.

Processing Logic:
  1. Verify candle is incomplete and warmup period is satisfied
  2. Check if strategy implements HighFrequencyStrategy interface
  3. Update dataframe with partial candle data
  4. Recalculate indicators with latest data
  5. Allow strategy to process partial candle for rapid decision-making

Parameters:
  • candle: Incomplete candle with current OHLCV data
*/
func (s *Controller) OnPartialCandle(candle model.Candle) {
	// Only process incomplete candles after warmup period
	if !candle.Complete && len(s.dataframe.Close) >= s.strategy.WarmupPeriod() {
		// Check if strategy supports high-frequency updates
		if str, ok := s.strategy.(HighFrequencyStrategy); ok {
			s.updateDataFrame(candle)
			str.Indicators(s.dataframe)
			str.OnPartialCandle(s.dataframe, s.broker)
		}
	}
}

/*
updateDataFrame manages the dataframe buffer by either updating the last candle
or appending a new candle based on timestamps.

Update Logic:
  • If candle time matches last dataframe entry: update in-place (partial candle update)
  • If candle time is newer: append as new entry (new complete candle)
  • All OHLCV data and metadata are properly synchronized

This approach efficiently handles both partial candle updates and new candle additions
while maintaining data integrity and temporal ordering.
*/
func (s *Controller) updateDataFrame(candle model.Candle) {
	// Check if this is an update to the last candle or a new candle
	if len(s.dataframe.Time) > 0 && candle.Time.Equal(s.dataframe.Time[len(s.dataframe.Time)-1]) {
		// Update existing candle data in-place
		last := len(s.dataframe.Time) - 1
		s.dataframe.Close[last] = candle.Close
		s.dataframe.Open[last] = candle.Open
		s.dataframe.High[last] = candle.High
		s.dataframe.Low[last] = candle.Low
		s.dataframe.Volume[last] = candle.Volume
		s.dataframe.Time[last] = candle.Time
		
		// Update metadata values
		for k, v := range candle.Metadata {
			s.dataframe.Metadata[k][last] = v
		}
	} else {
		// Append new candle data
		s.dataframe.Close = append(s.dataframe.Close, candle.Close)
		s.dataframe.Open = append(s.dataframe.Open, candle.Open)
		s.dataframe.High = append(s.dataframe.High, candle.High)
		s.dataframe.Low = append(s.dataframe.Low, candle.Low)
		s.dataframe.Volume = append(s.dataframe.Volume, candle.Volume)
		s.dataframe.Time = append(s.dataframe.Time, candle.Time)
		s.dataframe.LastUpdate = candle.Time
		
		// Append metadata values
		for k, v := range candle.Metadata {
			s.dataframe.Metadata[k] = append(s.dataframe.Metadata[k], v)
		}
	}
}

/*
OnCandle processes complete candle data for standard strategy execution.
This is the primary entry point for most trading strategies.

Processing Flow:
  1. Validate candle timestamp ordering (reject late candles)
  2. Update dataframe with complete candle data
  3. Check if sufficient warmup data is available
  4. Calculate indicators using windowed dataframe sample
  5. Execute strategy logic if trading is enabled

Parameters:
  • candle: Complete candle with final OHLCV data
*/
func (s *Controller) OnCandle(candle model.Candle) {
	// Reject out-of-order candles to maintain data integrity
	if len(s.dataframe.Time) > 0 && candle.Time.Before(s.dataframe.Time[len(s.dataframe.Time)-1]) {
		log.Errorf("late candle received: %#v", candle)
		return
	}

	// Update dataframe with new candle
	s.updateDataFrame(candle)

	// Process strategy if sufficient data is available
	if len(s.dataframe.Close) >= s.strategy.WarmupPeriod() {
		// Create windowed sample for strategy processing
		sample := s.dataframe.Sample(s.strategy.WarmupPeriod())
		
		// Calculate indicators on the sample
		s.strategy.Indicators(&sample)
		
		// Execute strategy logic if trading is active
		if s.started {
			s.strategy.OnCandle(&sample, s.broker)
		}
	}
}
