// Package exchange provides data feed management, order execution interfaces,
// and exchange-specific implementations for cryptocurrency trading.
// It includes support for real-time data subscriptions, paper trading,
// and various exchange protocols.
package exchange

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/StudioSol/set"

	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/service"
	"github.com/rodrigo-brito/ninjabot/tools/log"
)

// Common exchange errors
var (
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrInsufficientFunds = errors.New("insufficient funds or locked")
	ErrInvalidAsset      = errors.New("invalid asset")
)

// DataFeed represents a channel-based data feed for market data streaming.
// It provides separate channels for data and error handling.
type DataFeed struct {
	Data chan model.Candle
	Err  chan error
}

// DataFeedSubscription manages multiple data feed subscriptions and their consumers.
// It coordinates market data distribution to multiple subscribers and handles
// connection management for real-time data feeds.
type DataFeedSubscription struct {
	exchange                service.Exchange
	Feeds                   *set.LinkedHashSetString
	DataFeeds               map[string]*DataFeed
	SubscriptionsByDataFeed map[string][]Subscription
}

// Subscription represents a single data feed subscription with its configuration
// and consumer callback function for processing incoming market data.
type Subscription struct {
	onCandleClose bool
	consumer      DataFeedConsumer
}

// OrderError provides detailed error information for order-related failures
// including the specific pair and quantity that caused the error.
type OrderError struct {
	Err      error
	Pair     string
	Quantity float64
}

// Error returns a formatted error message for the OrderError.
func (o *OrderError) Error() string {
	return fmt.Sprintf("order error: %v", o.Err)
}

// DataFeedConsumer defines the function signature for processing incoming market data.
type DataFeedConsumer func(model.Candle)

// NewDataFeed creates a new DataFeedSubscription instance for managing multiple
// data feed subscriptions from the specified exchange.
func NewDataFeed(exchange service.Exchange) *DataFeedSubscription {
	return &DataFeedSubscription{
		exchange:                exchange,
		Feeds:                   set.NewLinkedHashSetString(),
		DataFeeds:               make(map[string]*DataFeed),
		SubscriptionsByDataFeed: make(map[string][]Subscription),
	}
}

// feedKey generates a unique identifier for a data feed based on pair and timeframe.
func (d *DataFeedSubscription) feedKey(pair, timeframe string) string {
	return fmt.Sprintf("%s--%s", pair, timeframe)
}

// pairTimeframeFromKey extracts pair and timeframe from a feed key.
func (d *DataFeedSubscription) pairTimeframeFromKey(key string) (pair, timeframe string) {
	parts := strings.Split(key, "--")
	return parts[0], parts[1]
}

// Subscribe registers a consumer function to receive market data for a specific
// pair and timeframe. The onCandleClose parameter determines if the consumer
// should only receive complete candles or all candle updates.
func (d *DataFeedSubscription) Subscribe(pair, timeframe string, consumer DataFeedConsumer, onCandleClose bool) {
	key := d.feedKey(pair, timeframe)
	d.Feeds.Add(key)
	d.SubscriptionsByDataFeed[key] = append(d.SubscriptionsByDataFeed[key], Subscription{
		onCandleClose: onCandleClose,
		consumer:      consumer,
	})
}

// Preload processes historical candle data for warming up strategies and indicators.
// This ensures that technical indicators have sufficient historical data before
// starting live trading or backtesting.
func (d *DataFeedSubscription) Preload(pair, timeframe string, candles []model.Candle) {
	log.Infof("[SETUP] preloading %d candles for %s-%s", len(candles), pair, timeframe)
	key := d.feedKey(pair, timeframe)
	for _, candle := range candles {
		if !candle.Complete {
			continue
		}

		for _, subscription := range d.SubscriptionsByDataFeed[key] {
			subscription.consumer(candle)
		}
	}
}

// Connect establishes connections to exchange data feeds for all registered subscriptions.
// This method initiates the WebSocket or other real-time connections needed for live data.
func (d *DataFeedSubscription) Connect() {
	log.Infof("Connecting to the exchange.")
	for feed := range d.Feeds.Iter() {
		pair, timeframe := d.pairTimeframeFromKey(feed)
		ccandle, cerr := d.exchange.CandlesSubscription(context.Background(), pair, timeframe)
		d.DataFeeds[feed] = &DataFeed{
			Data: ccandle,
			Err:  cerr,
		}
	}
}

// Start begins processing data from all connected feeds and distributing
// it to registered consumers. The loadSync parameter determines whether
// to wait for all feeds to complete (backtesting) or run continuously (live trading).
func (d *DataFeedSubscription) Start(loadSync bool) {
	d.Connect()
	wg := new(sync.WaitGroup)
	for key, feed := range d.DataFeeds {
		wg.Add(1)
		go func(key string, feed *DataFeed) {
			for {
				select {
				case candle, ok := <-feed.Data:
					if !ok {
						wg.Done()
						return
					}
					for _, subscription := range d.SubscriptionsByDataFeed[key] {
						if subscription.onCandleClose && !candle.Complete {
							continue
						}
						subscription.consumer(candle)
					}
				case err := <-feed.Err:
					if err != nil {
						log.Error("dataFeedSubscription/start: ", err)
					}
				}
			}
		}(key, feed)
	}

	log.Infof("Data feed connected.")
	if loadSync {
		wg.Wait()
	}
}
