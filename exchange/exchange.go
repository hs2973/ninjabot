/*
Package exchange provides data feed management, order execution interfaces,
and exchange-specific implementations for cryptocurrency trading.

Core Functionality:
  • Real-time Market Data - WebSocket streams and REST API integration
  • Order Execution - Buy/sell order placement and tracking across exchanges
  • Data Feed Management - Subscription handling and event distribution
  • Paper Trading - Simulated trading environment for backtesting

Supported Features:
  • Multiple exchange connectivity (Binance, etc.)
  • Real-time candle data streaming
  • Order book and trade data
  • Comprehensive error handling and retry logic
  • Exchange-agnostic interface design

Architecture:
  • DataFeedSubscription manages multi-pair data streams
  • PaperWallet provides realistic trading simulation
  • Exchange interface abstracts specific exchange implementations
*/
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

// ═══════════════════════════════════════════════════════════════════
// Exchange Error Definitions
// ═══════════════════════════════════════════════════════════════════

// Common exchange errors that can occur during trading operations.
// These standardized errors enable consistent error handling across
// different exchange implementations and trading strategies.
var (
	ErrInvalidQuantity   = errors.New("invalid quantity")    // Order quantity violates exchange rules
	ErrInsufficientFunds = errors.New("insufficient funds or locked") // Not enough balance for order
	ErrInvalidAsset      = errors.New("invalid asset")       // Asset not supported or malformed
)

// ═══════════════════════════════════════════════════════════════════
// Data Feed Core Types
// ═══════════════════════════════════════════════════════════════════

/*
DataFeed represents a channel-based data feed for market data streaming.

Channel Architecture:
  • Data: Receives model.Candle objects for market data
  • Err: Receives error objects for connection/data issues

Usage Pattern:
  feed := &DataFeed{
      Data: make(chan model.Candle, 100),
      Err:  make(chan error, 10),
  }
  
  // Consumer goroutine
  go func() {
      for {
          select {
          case candle := <-feed.Data:
              // Process market data
          case err := <-feed.Err:
              // Handle feed errors
          }
      }
  }()
*/
type DataFeed struct {
	Data chan model.Candle // Channel for receiving market data candles
	Err  chan error        // Channel for receiving feed errors
}

/*
DataFeedSubscription manages multiple data feed subscriptions and their consumers.

Architecture Overview:
  • Centralized subscription management for multiple trading pairs
  • Fan-out distribution to multiple consumers per data feed
  • Connection pooling and lifecycle management for exchange streams
  • Thread-safe subscription/unsubscription operations

Key Components:
  • exchange: Target exchange interface for data retrieval
  • Feeds: Ordered set of active feed identifiers
  • DataFeeds: Map of feed keys to DataFeed channels
  • SubscriptionsByDataFeed: Consumer callbacks grouped by feed

Threading Model:
  • Each data feed runs in its own goroutine
  • Consumers are called synchronously (should be lightweight)
  • Connection management handled asynchronously
*/
type DataFeedSubscription struct {
	exchange                service.Exchange                    // Exchange interface for data access
	Feeds                   *set.LinkedHashSetString           // Ordered set of active feed keys
	DataFeeds               map[string]*DataFeed               // Feed channels by key
	SubscriptionsByDataFeed map[string][]Subscription          // Consumer subscriptions by feed
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
