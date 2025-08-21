package indicator

import (
	"fmt"
	"time"

	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/plot"

	"github.com/markcheno/go-talib"
)

/*
EMA creates an Exponential Moving Average indicator for trend analysis.
EMA gives more weight to recent prices compared to Simple Moving Average,
making it more responsive to current market conditions.

EMA is commonly used for:
  • Trend identification and confirmation
  • Dynamic support/resistance levels
  • Entry/exit signal generation with crossovers
  • Reducing price noise while maintaining responsiveness

Parameters:
  • period: Number of periods for calculation (e.g., 20, 50, 200)
  • color: Line color for chart display

Returns configured EMA indicator as chart overlay.
*/
func EMA(period int, color string) plot.Indicator {
	return &ema{
		Period: period,
		Color:  color,
	}
}

// ema implements the Exponential Moving Average technical indicator.
type ema struct {
	Period int                    // Number of periods for EMA calculation
	Color  string                 // Display color for the moving average line
	Values model.Series[float64]  // Calculated EMA values
	Time   []time.Time            // Corresponding timestamps
}

// Warmup returns the minimum data points needed for stable EMA calculation.
func (e ema) Warmup() int {
	return e.Period
}

// Name returns the display name with period parameter.
func (e ema) Name() string {
	return fmt.Sprintf("EMA(%d)", e.Period)
}

// Overlay returns true since EMA is displayed over the main price chart.
func (e ema) Overlay() bool {
	return true
}

// Load calculates EMA values from the dataframe's closing prices.
func (e *ema) Load(dataframe *model.Dataframe) {
	if len(dataframe.Time) < e.Period {
		return
	}

	e.Values = talib.Ema(dataframe.Close, e.Period)[e.Period:]
	e.Time = dataframe.Time[e.Period:]
}

// Metrics returns the chart rendering configuration for the EMA line.
func (e ema) Metrics() []plot.IndicatorMetric {
	return []plot.IndicatorMetric{
		{
			Style:  "line",
			Color:  e.Color,
			Values: e.Values,
			Time:   e.Time,
		},
	}
}
