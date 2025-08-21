/*
Package strategy defines interfaces and controllers for trading strategy implementation.

Strategy Framework Features:
  • Pluggable Strategy Architecture - Custom algorithms via interface implementation
  • Technical Indicator Integration - Built-in and custom indicator support
  • High-Frequency Trading Support - Partial candle processing for quick reactions
  • Multi-Timeframe Analysis - Different strategies can use different timeframes
  • Risk Management Integration - Order sizing and position management

Core Interfaces:
  • Strategy: Base interface for all trading algorithms
  • HighFrequencyStrategy: Extended interface for intra-candle trading
  • ChartIndicator: Technical analysis indicator integration

Strategy Lifecycle:
  1. Timeframe Definition - Specify required data granularity
  2. Warmup Period - Historical data requirements for indicators
  3. Indicator Setup - Configure technical analysis tools
  4. Trading Logic - Process market data and make trading decisions

Implementation Guidelines:
  • Keep OnCandle processing lightweight for real-time performance
  • Use indicators for technical analysis rather than raw calculations
  • Implement proper error handling and edge case management
  • Consider market conditions and volatility in decision making
*/
package strategy

import (
	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/service"
)

/*
Strategy defines the interface that all trading strategies must implement.

Required Methods Overview:
  • Timeframe(): Defines the candlestick interval for strategy execution
  • WarmupPeriod(): Specifies historical data requirements for indicators
  • Indicators(): Configures technical analysis tools and calculations
  • OnCandle(): Main trading logic executed for each complete candle

Implementation Considerations:
  • Timeframe should match your analysis requirements (1m, 5m, 1h, 1d, etc.)
  • WarmupPeriod must provide sufficient data for indicator calculations
  • Indicators are calculated before OnCandle for each new candle
  • OnCandle receives complete market data after candle close

Performance Guidelines:
  • Keep OnCandle execution time minimal for real-time trading
  • Use provided indicators rather than calculating from scratch
  • Avoid blocking operations that could delay processing
  • Handle edge cases gracefully (insufficient data, market gaps)
*/
type Strategy interface {
	/*
	Timeframe specifies the candlestick interval for strategy execution.
	
	Common Timeframes:
	  • "1m", "5m", "15m" - High-frequency/scalping strategies
	  • "1h", "4h" - Intraday trading strategies  
	  • "1d", "1w" - Position trading strategies
	
	The timeframe determines:
	  • How often OnCandle is called
	  • Granularity of market data analysis
	  • Strategy reaction speed to market changes
	*/
	Timeframe() string
	
	/*
	WarmupPeriod specifies the number of historical candles needed before trading begins.
	
	Purpose:
	  • Initialize technical indicators with sufficient data
	  • Establish baseline values for moving averages, oscillators
	  • Ensure statistical validity of analysis tools
	
	Guidelines:
	  • SMA(20) requires at least 20 periods
	  • EMA converges faster but benefits from 2-3x the period
	  • Complex indicators may need 100+ periods for accuracy
	  • Higher values increase reliability but delay strategy start
	*/
	WarmupPeriod() int
	
	/*
	Indicators configures technical analysis tools executed before OnCandle.
	
	Execution Order:
	  1. New candle data arrives
	  2. All indicators are calculated/updated
	  3. OnCandle is called with updated dataframe
	
	Best Practices:
	  • Return indicators in dependency order (base indicators first)
	  • Use built-in indicators when possible for performance
	  • Avoid expensive calculations in this method
	  • Configure indicators once, don't recreate each call
	*/
	Indicators(df *model.Dataframe) []ChartIndicator
	
	/*
	OnCandle contains the main trading logic executed after each complete candle.
	
	This method is called after:
	  • A complete candle has formed (candle.Complete == true)
	  • All indicators have been calculated and updated
	  • Dataframe contains the latest market data
	
	Trading Operations:
	  • Use broker interface for order placement
	  • Access indicator values from dataframe metadata
	  • Implement risk management and position sizing
	  • Log important decisions and market observations
	
	Threading Considerations:
	  • Called synchronously - keep execution time minimal
	  • Don't block on external API calls
	  • Use context for timeout/cancellation handling
	*/
	OnCandle(df *model.Dataframe, broker service.Broker)
}

/*
HighFrequencyStrategy extends the basic Strategy interface with support for
partial candle processing.

Advanced Trading Features:
  • Intra-candle price movement analysis
  • Quick reaction to market changes before candle completion
  • Enhanced market microstructure analysis
  • Support for scalping and arbitrage strategies

Use Cases:
  • Market making and liquidity provision
  • Arbitrage opportunities across exchanges
  • News-based trading with quick reactions
  • Stop-loss and take-profit adjustments

Performance Critical:
  • OnPartialCandle called frequently (potentially every trade)
  • Must execute extremely quickly to maintain real-time performance
  • Consider queuing heavy operations for background processing
*/
type HighFrequencyStrategy interface {
	Strategy // Embed base strategy interface

	/*
	OnPartialCandle processes incomplete candle data for high-frequency trading.
	
	Partial Candle Characteristics:
	  • candle.Complete == false
	  • OHLC values may change before candle completion
	  • Volume increases throughout the candle period
	  • Close price reflects the most recent trade
	
	Use Cases:
	  • Quick stop-loss execution
	  • Market making bid/ask adjustments  
	  • Arbitrage opportunity detection
	  • Momentum-based entry signals
	
	Performance Requirements:
	  • Execution time should be < 1ms for real-time trading
	  • Avoid complex calculations or indicator updates
	  • Consider caching frequently accessed data
	  • Use simple price/volume comparisons when possible
	*/
	OnPartialCandle(df *model.Dataframe, broker service.Broker)
}
