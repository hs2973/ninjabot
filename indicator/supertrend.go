package indicator

import "github.com/markcheno/go-talib"

/*
SuperTrend calculates the SuperTrend indicator, a trend-following indicator that uses
Average True Range (ATR) to determine dynamic support and resistance levels.

The SuperTrend indicator helps identify trend direction and potential reversal points
by creating adaptive bands around price action. When price is above the SuperTrend line,
it indicates an uptrend; when below, it indicates a downtrend.

Algorithm:
  1. Calculate ATR using the specified period
  2. Compute basic upper and lower bands using HL2 +/- (ATR * factor)
  3. Apply trend rules to create final bands that don't reverse against trend
  4. Generate SuperTrend line based on price position relative to bands

Parameters:
  • high: Array of high prices
  • low: Array of low prices  
  • close: Array of closing prices
  • atrPeriod: Period for ATR calculation (typically 10)
  • factor: Multiplier for ATR (typically 2.0-3.0)

Returns array of SuperTrend values where positive values indicate uptrend
and negative values indicate downtrend.
*/
func SuperTrend(high, low, close []float64, atrPeriod int, factor float64) []float64 {
	// Calculate ATR for the specified period
	atr := talib.Atr(high, low, close, atrPeriod)
	
	// Initialize band arrays
	basicUpperBand := make([]float64, len(atr))
	basicLowerBand := make([]float64, len(atr))
	finalUpperBand := make([]float64, len(atr))
	finalLowerBand := make([]float64, len(atr))
	superTrend := make([]float64, len(atr))

	for i := 1; i < len(basicLowerBand); i++ {
		// Calculate basic bands using HL2 (typical price) +/- ATR * factor
		basicUpperBand[i] = (high[i]+low[i])/2.0 + atr[i]*factor
		basicLowerBand[i] = (high[i]+low[i])/2.0 - atr[i]*factor

		// Final upper band: prevent band from falling during uptrend
		if basicUpperBand[i] < finalUpperBand[i-1] ||
			close[i-1] > finalUpperBand[i-1] {
			finalUpperBand[i] = basicUpperBand[i]
		} else {
			finalUpperBand[i] = finalUpperBand[i-1]
		}

		// Final lower band: prevent band from rising during downtrend
		if basicLowerBand[i] > finalLowerBand[i-1] ||
			close[i-1] < finalLowerBand[i-1] {
			finalLowerBand[i] = basicLowerBand[i]
		} else {
			finalLowerBand[i] = finalLowerBand[i-1]
		}

		// Determine SuperTrend line based on price position and previous trend
		// Trend reversal logic: switch between upper and lower bands
		if finalUpperBand[i-1] == superTrend[i-1] {
			// Previous trend was down (following upper band)
			if close[i] > finalUpperBand[i] {
				// Price breaks above upper band - trend changes to up
				superTrend[i] = finalLowerBand[i]
			} else {
				// Continue downtrend
				superTrend[i] = finalUpperBand[i]
			}
		} else {
			// Previous trend was up (following lower band)
			if close[i] < finalLowerBand[i] {
				// Price breaks below lower band - trend changes to down
				superTrend[i] = finalUpperBand[i]
			} else {
				// Continue uptrend
				superTrend[i] = finalLowerBand[i]
			}
		}
	}

	return superTrend
}
