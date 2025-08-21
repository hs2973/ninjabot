/*
Package tools provides utility components for trading automation including
conditional order scheduling, trailing stops, and market analysis helpers.
These tools extend the core ninjabot functionality with higher-level
trading abstractions for strategy development.
*/
package tools

import (
	"github.com/rodrigo-brito/ninjabot"
	"github.com/rodrigo-brito/ninjabot/service"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
)

/*
OrderCondition defines a conditional trading rule that triggers order execution
when specific market conditions are met. This enables event-driven trading
strategies based on technical indicators, price levels, or custom logic.

Fields:
  • Condition: Function that evaluates market data and returns true when order should execute
  • Size: Order quantity in base asset units
  • Side: Buy or sell direction for the order
*/
type OrderCondition struct {
	Condition func(df *ninjabot.Dataframe) bool  // Market condition evaluation function
	Size      float64                           // Order size in base asset
	Side      ninjabot.SideType                 // Order direction (buy/sell)
}

/*
Scheduler manages conditional order execution for a specific trading pair.
It maintains a list of pending order conditions and automatically executes
market orders when conditions are satisfied.

Use cases:
  • Breakout trading: Buy/sell when price crosses specific levels
  • Indicator-based entries: Trade on moving average crossovers, RSI extremes
  • Time-based trading: Execute orders at specific times or intervals
  • Multi-condition strategies: Complex logic combining multiple indicators
*/
type Scheduler struct {
	pair            string           // Trading pair for all scheduled orders
	orderConditions []OrderCondition // List of pending conditional orders
}

// NewScheduler creates a new conditional order scheduler for the specified trading pair.
func NewScheduler(pair string) *Scheduler {
	return &Scheduler{pair: pair}
}

/*
SellWhen schedules a sell order that executes when the specified condition becomes true.
The condition function receives current market data and should return true to trigger
the order. Once executed, the condition is removed from the scheduler.

Parameters:
  • size: Order quantity in base asset units
  • condition: Function that evaluates market data to determine if order should execute

Example usage:
  scheduler.SellWhen(0.1, func(df *ninjabot.Dataframe) bool {
      return df.Close.Last(0) > 50000  // Sell when price above $50,000
  })
*/
func (s *Scheduler) SellWhen(size float64, condition func(df *ninjabot.Dataframe) bool) {
	s.orderConditions = append(
		s.orderConditions,
		OrderCondition{Condition: condition, Size: size, Side: ninjabot.SideTypeSell},
	)
}

/*
BuyWhen schedules a buy order that executes when the specified condition becomes true.
Similar to SellWhen but for buy-side orders. The condition is evaluated on each
market data update and triggers a market buy order when satisfied.

Parameters:
  • size: Order quantity in base asset units  
  • condition: Function that evaluates market data to determine if order should execute

Example usage:
  scheduler.BuyWhen(0.1, func(df *ninjabot.Dataframe) bool {
      rsi := df.Metadata["rsi"]
      return rsi < 30  // Buy when RSI indicates oversold
  })
*/
func (s *Scheduler) BuyWhen(size float64, condition func(df *ninjabot.Dataframe) bool) {
	s.orderConditions = append(
		s.orderConditions,
		OrderCondition{Condition: condition, Size: size, Side: ninjabot.SideTypeBuy},
	)
}

/*
Update evaluates all pending order conditions against current market data.
When a condition is satisfied, it immediately executes a market order and
removes the condition from the scheduler. Failed orders are logged but
the condition remains active for retry on next update.

This method should be called on each candle/tick update from the strategy.

Parameters:
  • df: Current market dataframe with price and indicator data
  • broker: Exchange interface for order execution
*/
func (s *Scheduler) Update(df *ninjabot.Dataframe, broker service.Broker) {
	// Filter conditions: remove those that execute successfully
	s.orderConditions = lo.Filter[OrderCondition](s.orderConditions, func(oc OrderCondition, _ int) bool {
		// Check if condition is satisfied
		if oc.Condition(df) {
			// Execute market order
			_, err := broker.CreateOrderMarket(oc.Side, s.pair, oc.Size)
			if err != nil {
				log.Error(err)
				return true  // Keep condition for retry
			}
			return false  // Remove condition after successful execution
		}
		return true  // Keep condition for future evaluation
	})
}
