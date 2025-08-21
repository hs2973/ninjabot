/*
Package indicator provides chart indicator implementations for technical analysis visualization.
These indicators integrate with the plotting system to display technical analysis overlays
and oscillators on trading charts for strategy analysis and market insight.

All indicators implement the plot.Indicator interface enabling:
  • Automatic data loading from dataframes
  • Configurable visual styling (colors, line types)
  • Overlay vs separate panel positioning
  • Warmup period calculation for proper indicator initialization
*/
package indicator

import (
	"fmt"
	"time"

	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/plot"

	"github.com/markcheno/go-talib"
)

/*
RSI creates a Relative Strength Index indicator for momentum analysis.
RSI oscillates between 0-100 and helps identify overbought (>70) and oversold (<30) conditions.

The RSI measures the velocity and magnitude of price changes, making it useful for:
  • Identifying potential reversal points
  • Confirming trend strength
  • Spotting divergences between price and momentum
  • Generating buy/sell signals in ranging markets

Parameters:
  • period: Calculation period (typically 14)
  • color: Line color for chart display (e.g., "#ff0000")

Returns configured RSI indicator ready for chart plotting.
*/
func RSI(period int, color string) plot.Indicator {
	return &rsi{
		Period: period,
		Color:  color,
	}
}

// rsi implements the RSI (Relative Strength Index) technical indicator for chart display.
type rsi struct {
	Period int                    // Calculation period for RSI
	Color  string                 // Display color for the indicator line
	Values model.Series[float64]  // Calculated RSI values
	Time   []time.Time            // Corresponding timestamps
}

// Warmup returns the minimum number of data points needed before indicator produces valid values.
func (e rsi) Warmup() int {
	return e.Period
}

// Name returns the display name for the indicator including its parameters.
func (e rsi) Name() string {
	return fmt.Sprintf("RSI(%d)", e.Period)
}

// Overlay returns false since RSI is displayed in a separate panel below the main chart.
func (e rsi) Overlay() bool {
	return false
}

/*
Load calculates RSI values from the provided dataframe.
The method requires sufficient data points (warmup period) and computes RSI
using the TA-Lib implementation for accuracy and consistency.

Parameters:
  • dataframe: Market data containing close prices and timestamps
*/
func (e *rsi) Load(dataframe *model.Dataframe) {
	// Ensure sufficient data for calculation
	if len(dataframe.Time) < e.Period {
		return
	}

	// Calculate RSI and trim warmup period
	e.Values = talib.Rsi(dataframe.Close, e.Period)[e.Period:]
	e.Time = dataframe.Time[e.Period:]
}

// Metrics returns the plotting configuration for chart rendering.
func (e rsi) Metrics() []plot.IndicatorMetric {
	return []plot.IndicatorMetric{
		{
			Color:  e.Color,
			Style:  "line",
			Values: e.Values,
			Time:   e.Time,
		},
	}
}
