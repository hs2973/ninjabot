// Package model defines core data structures and types used throughout the ninjabot framework.
// It includes trading models like candles, orders, accounts, and dataframes, as well as
// configuration structures for bot settings and exchange connections.
package model

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// TelegramSettings holds configuration for Telegram bot notifications including
// the bot token and list of authorized user IDs who can receive notifications.
type TelegramSettings struct {
	Enabled bool
	Token   string
	Users   []int
}

// Settings contains the main bot configuration including trading pairs
// and notification settings. This structure is used to initialize the bot
// with the desired trading pairs and communication preferences.
type Settings struct {
	Pairs    []string
	Telegram TelegramSettings
}

// Balance represents an account balance for a specific asset including
// free (available) and locked (reserved for orders) amounts, plus leverage information.
type Balance struct {
	Asset    string
	Free     float64
	Lock     float64
	Leverage float64
}

// AssetInfo contains detailed information about a trading pair including
// price and quantity constraints, precision settings, and trading rules
// as defined by the exchange.
type AssetInfo struct {
	BaseAsset  string
	QuoteAsset string

	MinPrice    float64
	MaxPrice    float64
	MinQuantity float64
	MaxQuantity float64
	StepSize    float64
	TickSize    float64

	QuotePrecision     int
	BaseAssetPrecision int
}

// Dataframe represents a collection of market data organized in time series format.
// It contains OHLCV (Open, High, Low, Close, Volume) data along with timestamps
// and supports custom metadata for indicators and additional analysis data.
type Dataframe struct {
	Pair string

	Close  Series[float64]
	Open   Series[float64]
	High   Series[float64]
	Low    Series[float64]
	Volume Series[float64]

	Time       []time.Time
	LastUpdate time.Time

	// Custom user metadata
	Metadata map[string]Series[float64]
}

// Sample returns a new Dataframe containing only the last 'positions' number of data points.
// This is useful for limiting the dataset size or focusing on recent market activity.
// If positions exceeds the available data, the entire dataframe is returned.
func (df Dataframe) Sample(positions int) Dataframe {
	size := len(df.Time)
	start := size - positions
	if start <= 0 {
		return df
	}

	sample := Dataframe{
		Pair:       df.Pair,
		Close:      df.Close.LastValues(positions),
		Open:       df.Open.LastValues(positions),
		High:       df.High.LastValues(positions),
		Low:        df.Low.LastValues(positions),
		Volume:     df.Volume.LastValues(positions),
		Time:       df.Time[start:],
		LastUpdate: df.LastUpdate,
		Metadata:   make(map[string]Series[float64]),
	}

	for key := range df.Metadata {
		sample.Metadata[key] = df.Metadata[key].LastValues(positions)
	}

	return sample
}

// Candle represents a single market data point containing OHLCV information
// for a specific time period. It includes completion status and supports
// additional metadata for custom analysis or CSV imports.
type Candle struct {
	Pair      string
	Time      time.Time
	UpdatedAt time.Time
	Open      float64
	Close     float64
	Low       float64
	High      float64
	Volume    float64
	Complete  bool

	// Aditional collums from CSV inputs
	Metadata map[string]float64
}

// Empty returns true if the candle contains no meaningful data.
// This is useful for checking if a candle has been properly initialized
// or if it represents a null/empty state.
func (c Candle) Empty() bool {
	return c.Pair == "" && c.Close == 0 && c.Open == 0 && c.Volume == 0
}

// HeikinAshi maintains state for calculating Heikin-Ashi candles, which are
// modified candlesticks that help filter market noise and identify trends.
// It stores the previous Heikin-Ashi candle needed for continuous calculations.
type HeikinAshi struct {
	PreviousHACandle Candle
}

// NewHeikinAshi creates a new HeikinAshi calculator instance.
// This initializes the state needed for Heikin-Ashi candle calculations.
func NewHeikinAshi() *HeikinAshi {
	return &HeikinAshi{}
}

// ToSlice converts the candle data to a string slice format suitable for CSV output.
// The precision parameter controls the number of decimal places for floating-point values.
func (c Candle) ToSlice(precision int) []string {
	return []string{
		fmt.Sprintf("%d", c.Time.Unix()),
		strconv.FormatFloat(c.Open, 'f', precision, 64),
		strconv.FormatFloat(c.Close, 'f', precision, 64),
		strconv.FormatFloat(c.Low, 'f', precision, 64),
		strconv.FormatFloat(c.High, 'f', precision, 64),
		strconv.FormatFloat(c.Volume, 'f', precision, 64),
	}
}

// ToHeikinAshi converts a regular candle to a Heikin-Ashi candle using the provided
// HeikinAshi calculator. This creates smoothed candles that help identify trends
// by reducing market noise in the visualization.
func (c Candle) ToHeikinAshi(ha *HeikinAshi) Candle {
	haCandle := ha.CalculateHeikinAshi(c)

	return Candle{
		Pair:      c.Pair,
		Open:      haCandle.Open,
		High:      haCandle.High,
		Low:       haCandle.Low,
		Close:     haCandle.Close,
		Volume:    c.Volume,
		Complete:  c.Complete,
		Time:      c.Time,
		UpdatedAt: c.UpdatedAt,
	}
}

// Less compares two candles for ordering in priority queues or sorting operations.
// Candles are ordered first by time, then by update time, and finally by pair name.
// This ensures proper chronological processing of market data.
func (c Candle) Less(j Item) bool {
	diff := j.(Candle).Time.Sub(c.Time)
	if diff < 0 {
		return false
	}
	if diff > 0 {
		return true
	}

	diff = j.(Candle).UpdatedAt.Sub(c.UpdatedAt)
	if diff < 0 {
		return false
	}
	if diff > 0 {
		return true
	}

	return c.Pair < j.(Candle).Pair
}

// Account represents a trading account containing balance information for various assets.
// It provides methods to query specific balances and calculate total equity.
type Account struct {
	Balances []Balance
}

// Balance retrieves the balance information for the specified asset and quote currencies.
// This is commonly used to check available funds before placing orders or calculating
// portfolio positions for a trading pair.
func (a Account) Balance(assetTick, quoteTick string) (Balance, Balance) {
	var assetBalance, quoteBalance Balance
	var isSetAsset, isSetQuote bool

	for _, balance := range a.Balances {
		switch balance.Asset {
		case assetTick:
			assetBalance = balance
			isSetAsset = true
		case quoteTick:
			quoteBalance = balance
			isSetQuote = true
		}

		if isSetAsset && isSetQuote {
			break
		}
	}

	return assetBalance, quoteBalance
}

// Equity calculates the total equity (value) of the account by summing all
// free and locked balances across all assets. This provides a snapshot
// of the account's total value at a given time.
func (a Account) Equity() float64 {
	var total float64

	for _, balance := range a.Balances {
		total += balance.Free
		total += balance.Lock
	}

	return total
}

// CalculateHeikinAshi computes a Heikin-Ashi candle from a regular candle using
// the stored previous Heikin-Ashi candle state. This implementation follows
// the standard Heikin-Ashi calculation formula to produce smoothed candlesticks.
func (ha *HeikinAshi) CalculateHeikinAshi(c Candle) Candle {
	var hkCandle Candle

	openValue := ha.PreviousHACandle.Open
	closeValue := ha.PreviousHACandle.Close

	// First HA candle is calculated using current candle
	if ha.PreviousHACandle.Empty() {
		openValue = c.Open
		closeValue = c.Close
	}

	hkCandle.Open = (openValue + closeValue) / 2
	hkCandle.Close = (c.Open + c.High + c.Low + c.Close) / 4
	hkCandle.High = math.Max(c.High, math.Max(hkCandle.Open, hkCandle.Close))
	hkCandle.Low = math.Min(c.Low, math.Min(hkCandle.Open, hkCandle.Close))
	ha.PreviousHACandle = hkCandle

	return hkCandle
}
