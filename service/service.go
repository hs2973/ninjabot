//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

// Package service defines core interfaces for trading bot components including
// exchange connections, data feeds, order execution, and notification systems.
// These interfaces enable dependency injection and testing through mock implementations.
package service

import (
	"context"
	"time"

	"github.com/rodrigo-brito/ninjabot/model"
)

// Exchange combines the Broker and Feeder interfaces to provide a complete
// trading interface that supports both market data access and order execution.
type Exchange interface {
	Broker
	Feeder
}

// Feeder interface provides access to market data including asset information,
// price quotes, historical candles, and real-time market data subscriptions.
type Feeder interface {
	AssetsInfo(pair string) model.AssetInfo
	LastQuote(ctx context.Context, pair string) (float64, error)
	CandlesByPeriod(ctx context.Context, pair, period string, start, end time.Time) ([]model.Candle, error)
	CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error)
	CandlesSubscription(ctx context.Context, pair, timeframe string) (chan model.Candle, chan error)
}

// Broker interface provides trading operations including account management,
// position tracking, order creation and management across different order types.
type Broker interface {
	Account() (model.Account, error)
	Position(pair string) (asset, quote float64, err error)
	Order(pair string, id int64) (model.Order, error)
	CreateOrderOCO(side model.SideType, pair string, size, price, stop, stopLimit float64) ([]model.Order, error)
	CreateOrderLimit(side model.SideType, pair string, size float64, limit float64) (model.Order, error)
	CreateOrderMarket(side model.SideType, pair string, size float64) (model.Order, error)
	CreateOrderMarketQuote(side model.SideType, pair string, quote float64) (model.Order, error)
	CreateOrderStop(pair string, quantity float64, limit float64) (model.Order, error)
	Cancel(model.Order) error
}

// Notifier interface defines methods for sending notifications about trading events,
// order updates, and error conditions to external systems or users.
type Notifier interface {
	Notify(string)
	OnOrder(order model.Order)
	OnError(err error)
}

// Telegram interface extends Notifier with start functionality for Telegram bot operations.
// It provides interactive trading notifications and commands through Telegram messaging.
type Telegram interface {
	Notifier
	Start()
}
