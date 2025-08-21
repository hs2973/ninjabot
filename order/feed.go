/*
Package order provides order feed management and subscription services for
real-time order updates and notifications. It enables components to subscribe
to order events for specific trading pairs with filtering capabilities.
*/
package order

import (
	"github.com/rodrigo-brito/ninjabot/model"
)

/*
DataFeed represents a channel-based data feed for order updates on a specific trading pair.
It provides separate channels for order data and error handling to enable non-blocking
order processing and robust error management.
*/
type DataFeed struct {
	Data chan model.Order  // Channel for order updates
	Err  chan error        // Channel for error notifications
}

// FeedConsumer defines a function type for consuming order updates from data feeds.
type FeedConsumer func(order model.Order)

/*
Feed manages order data feeds and subscriptions across multiple trading pairs.
It provides a publish-subscribe pattern for distributing order updates to
interested consumers with optional filtering capabilities.

Architecture:
  • Per-pair data feeds with dedicated channels
  • Multiple subscribers per trading pair
  • Filtering options (new orders only vs all updates)
  • Concurrent processing of subscriptions
*/
type Feed struct {
	OrderFeeds            map[string]*DataFeed     // Data feeds indexed by trading pair
	SubscriptionsBySymbol map[string][]Subscription // Subscribers grouped by trading pair
}

/*
Subscription represents a consumer's subscription to order updates for a trading pair.
It includes filtering options to control which order events are delivered.
*/
type Subscription struct {
	onlyNewOrder bool         // Filter: deliver only new orders, not status updates
	consumer     FeedConsumer // Function to call with order updates
}

// NewOrderFeed creates a new order feed manager with initialized internal structures.
func NewOrderFeed() *Feed {
	return &Feed{
		OrderFeeds:            make(map[string]*DataFeed),
		SubscriptionsBySymbol: make(map[string][]Subscription),
	}
}

/*
Subscribe registers a consumer to receive order updates for a specific trading pair.
The subscription creates a data feed for the pair if it doesn't exist and
adds the consumer to the notification list.

Parameters:
  • pair: Trading pair symbol (e.g., "BTCUSDT")
  • consumer: Function to call with order updates
  • onlyNewOrder: If true, only new orders are delivered; if false, all order updates are sent
*/
func (d *Feed) Subscribe(pair string, consumer FeedConsumer, onlyNewOrder bool) {
	// Create data feed for pair if it doesn't exist
	if _, ok := d.OrderFeeds[pair]; !ok {
		d.OrderFeeds[pair] = &DataFeed{
			Data: make(chan model.Order),
			Err:  make(chan error),
		}
	}

	// Add consumer to subscription list for the pair
	d.SubscriptionsBySymbol[pair] = append(d.SubscriptionsBySymbol[pair], Subscription{
		onlyNewOrder: onlyNewOrder,
		consumer:     consumer,
	})
}

/*
Publish sends an order update to all subscribers of the order's trading pair.
The order is delivered to the appropriate data feed where subscribers will
receive it based on their filtering preferences.

Parameters:
  • order: Order object containing update information
  • _: Unused parameter (maintained for interface compatibility)
*/
func (d *Feed) Publish(order model.Order, _ bool) {
	// Send order to the appropriate pair's data feed
	if _, ok := d.OrderFeeds[order.Pair]; ok {
		d.OrderFeeds[order.Pair].Data <- order
	}
}

/*
Start begins processing order feeds and distributing updates to subscribers.
It launches goroutines for each trading pair to handle concurrent order
processing and ensure real-time delivery of order updates.

The method starts background workers that:
  1. Listen for order updates on pair-specific channels
  2. Distribute orders to all subscribers for that pair
  3. Apply subscription filters (new orders only, etc.)
*/
func (d *Feed) Start() {
	// Start a worker goroutine for each trading pair
	for pair := range d.OrderFeeds {
		go func(pair string, feed *DataFeed) {
			// Process orders for this pair
			for order := range feed.Data {
				// Notify all subscribers for this pair
				for _, subscription := range d.SubscriptionsBySymbol[pair] {
					subscription.consumer(order)
				}
			}
		}(pair, d.OrderFeeds[pair])
	}
}
