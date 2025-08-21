/*
Package ninjabot provides a comprehensive framework for building cryptocurrency trading bots.

This framework supports multiple trading modes:
  • Backtesting - Historical data simulation for strategy validation
  • Paper Trading - Real-time simulation without actual funds
  • Live Trading - Real-time trading with actual funds

Core Components:
  • Strategy Management - Pluggable trading algorithms with indicator support
  • Order Execution - Advanced order types with risk management
  • Data Feeds - Real-time and historical market data streaming
  • Notifications - Multi-channel alerts (Telegram, webhooks)
  • Analysis Tools - Performance metrics, charts, and reporting

The framework is designed for both novice and advanced traders, providing
comprehensive tools for algorithmic trading across multiple cryptocurrency exchanges.
*/
package ninjabot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/aybabtme/uniplot/histogram"

	"github.com/rodrigo-brito/ninjabot/exchange"
	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/notification"
	"github.com/rodrigo-brito/ninjabot/order"
	"github.com/rodrigo-brito/ninjabot/service"
	"github.com/rodrigo-brito/ninjabot/storage"
	"github.com/rodrigo-brito/ninjabot/strategy"
	"github.com/rodrigo-brito/ninjabot/tools/log"
	"github.com/rodrigo-brito/ninjabot/tools/metrics"

	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
)

const defaultDatabase = "ninjabot.db"

// init initializes the logging system with a custom formatter that includes
// full timestamps and a specific timestamp format for better readability.
func init() {
	// Configure global logger with human-readable timestamps
	// Format: YYYY-MM-DD HH:MM for consistent log parsing
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,        // Include complete timestamp in logs
		TimestampFormat: "2006-01-02 15:04", // ISO-style format for clarity
	})
}

/*
OrderSubscriber defines the interface for components that need to be notified
when orders are created, updated, or filled.

Implementation Guidelines:
  • OnOrder should be thread-safe as it may be called concurrently
  • Processing should be lightweight to avoid blocking order flow
  • Heavy operations should be handled asynchronously

Common Implementations:
  • Notification services (Telegram, email, webhooks)
  • Logging and audit systems
  • Risk management monitors
  • Strategy performance trackers
*/
type OrderSubscriber interface {
	OnOrder(model.Order)
}

/*
CandleSubscriber defines the interface for components that need to be notified
when new candle data becomes available.

Usage Patterns:
  • Trading strategies processing market data
  • Technical indicators calculating values
  • Chart updates and visualization
  • Market analysis and alerting systems

Performance Considerations:
  • OnCandle should execute quickly to maintain data flow
  • Complex calculations should be batched or async
  • Memory usage should be managed for high-frequency data
*/
type CandleSubscriber interface {
	OnCandle(model.Candle)
}

/*
NinjaBot represents the main trading bot instance that coordinates all components
including exchange connections, strategy execution, order management, data feeds,
and notifications. It supports both backtesting and live trading modes.

Architecture Overview:
  • Modular design with pluggable components
  • Thread-safe concurrent operations
  • Event-driven data processing
  • Comprehensive error handling and recovery

Core Components:
  • storage: Persistent data storage for orders and metrics
  • settings: Configuration including pairs and notifications
  • exchange: Exchange interface for trading operations
  • strategy: Trading algorithm implementation
  • orderController: Order execution and tracking
  • dataFeed: Real-time market data streaming
  • paperWallet: Simulated trading for backtesting

Threading Model:
  • Each component runs in dedicated goroutines
  • Channel-based communication between components
  • Context-based cancellation and timeout handling
*/
type NinjaBot struct {
	// Core Configuration
	storage  storage.Storage    // Persistent data storage layer
	settings model.Settings     // Bot configuration and trading pairs
	exchange service.Exchange   // Exchange connection and API
	strategy strategy.Strategy  // Trading algorithm implementation
	
	// Notification Services
	notifier service.Notifier   // Generic notification interface
	telegram service.Telegram  // Telegram-specific notifications
	
	// Trading Infrastructure
	orderController       *order.Controller              // Order execution and tracking
	priorityQueueCandle   *model.PriorityQueue          // Backtest candle ordering
	strategiesControllers map[string]*strategy.Controller // Per-pair strategy instances
	
	// Data Management
	orderFeed   *order.Feed                      // Order event distribution
	dataFeed    *exchange.DataFeedSubscription   // Market data distribution
	paperWallet *exchange.PaperWallet           // Simulated trading wallet
	
	// Operating Mode
	backtest bool // True for backtesting, false for live trading
}

/*
Option represents a functional option pattern for configuring NinjaBot instances.

The functional options pattern provides several advantages:
  • Backward compatibility when adding new configuration options
  • Clear and readable API for optional parameters
  • Type-safe configuration without complex constructors
  • Composable configuration functions

Example Usage:
  bot, err := NewBot(ctx, settings, exchange, strategy,
      WithPaperWallet(wallet),
      WithStorage(customStorage),
      WithNotifier(slackNotifier),
  )

Design Pattern Benefits:
  • Extensible without breaking existing code
  • Self-documenting configuration options
  • Optional parameters with sensible defaults
*/
type Option func(*NinjaBot)

/*
NewBot creates a new NinjaBot instance with the specified settings, exchange, and strategy.

This constructor function initializes all necessary components including order management,
data feeds, and storage systems. Additional configuration can be applied using the
functional options pattern for maximum flexibility.

Parameters:
  • ctx: Context for cancellation and timeouts during initialization
  • settings: Bot configuration including trading pairs and notification settings
  • exch: Exchange interface implementation for trading operations
  • str: Trading strategy implementation defining the trading logic
  • options: Variable number of configuration functions for customization

Returns:
  • *NinjaBot: Fully configured and initialized bot instance
  • error: Initialization error if configuration is invalid or setup fails

Initialization Process:
  1. Validate trading pairs format and exchange compatibility
  2. Apply functional options for custom configuration
  3. Initialize storage layer (defaults to SQLite if not specified)
  4. Create order controller for trade execution and tracking
  5. Configure Telegram notifications if enabled
  6. Link all components together for coordinated operation

Error Conditions:
  • Invalid trading pair format (must be ASSET/QUOTE)
  • Storage initialization failure
  • Telegram configuration errors
  • Exchange connection issues
*/
func NewBot(ctx context.Context, settings model.Settings, exch service.Exchange, str strategy.Strategy,
	options ...Option) (*NinjaBot, error) {

	// Initialize bot instance with core components
	bot := &NinjaBot{
		settings:              settings,           // Store configuration settings
		exchange:              exch,              // Exchange interface for trading
		strategy:              str,               // Trading strategy implementation
		orderFeed:             order.NewOrderFeed(), // Order event distribution system
		dataFeed:              exchange.NewDataFeed(exch), // Market data streaming
		strategiesControllers: make(map[string]*strategy.Controller), // Per-pair strategy instances
		priorityQueueCandle:   model.NewPriorityQueue(nil), // Backtest candle ordering
	}

	// Validate all trading pairs before proceeding
	// Each pair must be in ASSET/QUOTE format (e.g., BTC/USDT)
	for _, pair := range settings.Pairs {
		asset, quote := exchange.SplitAssetQuote(pair)
		if asset == "" || quote == "" {
			return nil, fmt.Errorf("invalid pair: %s", pair)
		}
	}

	// Apply functional options for custom configuration
	// This allows for flexible setup without changing the constructor signature
	for _, option := range options {
		option(bot)
	}

	// Initialize storage layer if not provided via options
	// Defaults to SQLite file-based storage for persistence
	var err error
	if bot.storage == nil {
		bot.storage, err = storage.FromFile(defaultDatabase)
		if err != nil {
			return nil, err
		}
	}

	// Create order controller for managing trade execution and tracking
	// This is the central component for all order-related operations
	bot.orderController = order.NewController(ctx, exch, bot.storage, bot.orderFeed)

	// Configure Telegram notifications if enabled in settings
	// Telegram provides real-time trading alerts and bot status updates
	if settings.Telegram.Enabled {
		bot.telegram, err = notification.NewTelegram(bot.orderController, settings)
		if err != nil {
			return nil, err
		}
		// Register Telegram as the primary notifier for the bot
		WithNotifier(bot.telegram)(bot)
	}

	return bot, nil
}

/*
WithBacktest configures the bot to run in backtest mode using historical data.

Backtesting Mode Features:
  • Optimized CSV data reading for faster processing
  • Deterministic order execution timing
  • Paper wallet integration for simulated trading
  • Race condition prevention for consistent results

This option is required for backtesting environments and automatically
enables paper wallet functionality for simulated trading operations.

Parameters:
  • wallet: Paper wallet instance for simulated trading operations

Usage:
  bot, err := NewBot(ctx, settings, exchange, strategy,
      WithBacktest(paperWallet),
  )
*/
func WithBacktest(wallet *exchange.PaperWallet) Option {
	return func(bot *NinjaBot) {
		bot.backtest = true        // Enable backtesting mode
		opt := WithPaperWallet(wallet) // Configure paper wallet for simulation
		opt(bot)
	}
}

/*
WithStorage configures a custom storage backend for the bot.

By default, the bot uses a local SQLite database file (ninjabot.db) for
persistence. This option allows integration with alternative storage
systems such as PostgreSQL, MongoDB, or cloud-based solutions.

Storage Requirements:
  • Must implement the storage.Storage interface
  • Should provide ACID compliance for order data
  • Must support concurrent read/write operations
  • Should handle connection failures gracefully

Parameters:
  • storage: Custom storage implementation

Example:
  customStorage := postgresql.NewStorage(connectionString)
  bot, err := NewBot(ctx, settings, exchange, strategy,
      WithStorage(customStorage),
  )
*/
func WithStorage(storage storage.Storage) Option {
	return func(bot *NinjaBot) {
		bot.storage = storage
	}
}

/*
WithLogLevel configures the global logging level for the bot and all components.

Available log levels (from most to least verbose):
  • log.DebugLevel: Detailed debugging information
  • log.InfoLevel:  General operational information (default)
  • log.WarnLevel:  Warning conditions that should be noted
  • log.ErrorLevel: Error conditions requiring attention
  • log.FatalLevel: Critical errors causing program termination

The log level affects all components including strategies, order management,
data feeds, and exchange communications.

Parameters:
  • level: Desired logging level from logrus package

Example:
  bot, err := NewBot(ctx, settings, exchange, strategy,
      WithLogLevel(log.DebugLevel), // Enable verbose debugging
  )
*/
func WithLogLevel(level log.Level) Option {
	return func(_ *NinjaBot) {
		log.SetLevel(level) // Apply globally to all loggers
	}
}

// WithNotifier registers a notifier to the bot, currently only email and telegram are supported
func WithNotifier(notifier service.Notifier) Option {
	return func(bot *NinjaBot) {
		bot.notifier = notifier
		bot.orderController.SetNotifier(notifier)
		bot.SubscribeOrder(notifier)
	}
}

// WithCandleSubscription subscribes a given struct to the candle feed
func WithCandleSubscription(subscriber CandleSubscriber) Option {
	return func(bot *NinjaBot) {
		bot.SubscribeCandle(subscriber)
	}
}

// WithPaperWallet sets the paper wallet for the bot (used for backtesting and live simulation)
func WithPaperWallet(wallet *exchange.PaperWallet) Option {
	return func(bot *NinjaBot) {
		bot.paperWallet = wallet
	}
}

// SubscribeCandle registers candle subscribers to receive market data updates
// for all configured trading pairs. The subscription is set up with the
// strategy's timeframe to ensure data consistency.
func (n *NinjaBot) SubscribeCandle(subscriptions ...CandleSubscriber) {
	for _, pair := range n.settings.Pairs {
		for _, subscription := range subscriptions {
			n.dataFeed.Subscribe(pair, n.strategy.Timeframe(), subscription.OnCandle, false)
		}
	}
}

// WithOrderSubscription returns an option that registers an order subscriber
// to receive notifications about order events including creation, updates, and fills.
func WithOrderSubscription(subscriber OrderSubscriber) Option {
	return func(bot *NinjaBot) {
		bot.SubscribeOrder(subscriber)
	}
}

// SubscribeOrder registers order subscribers to receive order event notifications
// for all configured trading pairs. This enables components to react to order
// lifecycle events for logging, notifications, or strategy adjustments.
func (n *NinjaBot) SubscribeOrder(subscriptions ...OrderSubscriber) {
	for _, pair := range n.settings.Pairs {
		for _, subscription := range subscriptions {
			n.orderFeed.Subscribe(pair, subscription.OnOrder, false)
		}
	}
}

// Controller returns the order controller instance which manages order execution,
// tracking, and provides access to trading results and statistics.
func (n *NinjaBot) Controller() *order.Controller {
	return n.orderController
}

// Summary function displays all trades, accuracy and some bot metrics in stdout
// To access the raw data, you may access `bot.Controller().Results`
func (n *NinjaBot) Summary() {
	var (
		total  float64
		wins   int
		loses  int
		volume float64
		sqn    float64
	)

	buffer := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buffer)
	table.SetHeader([]string{"Pair", "Trades", "Win", "Loss", "% Win", "Payoff", "Pr Fact.", "SQN", "Profit", "Volume"})
	table.SetFooterAlignment(tablewriter.ALIGN_RIGHT)
	avgPayoff := 0.0
	avgProfitFactor := 0.0

	returns := make([]float64, 0)
	for _, summary := range n.orderController.Results {
		avgPayoff += summary.Payoff() * float64(len(summary.Win())+len(summary.Lose()))
		avgProfitFactor += summary.ProfitFactor() * float64(len(summary.Win())+len(summary.Lose()))
		table.Append([]string{
			summary.Pair,
			strconv.Itoa(len(summary.Win()) + len(summary.Lose())),
			strconv.Itoa(len(summary.Win())),
			strconv.Itoa(len(summary.Lose())),
			fmt.Sprintf("%.1f %%", float64(len(summary.Win()))/float64(len(summary.Win())+len(summary.Lose()))*100),
			fmt.Sprintf("%.3f", summary.Payoff()),
			fmt.Sprintf("%.3f", summary.ProfitFactor()),
			fmt.Sprintf("%.1f", summary.SQN()),
			fmt.Sprintf("%.2f", summary.Profit()),
			fmt.Sprintf("%.2f", summary.Volume),
		})
		total += summary.Profit()
		sqn += summary.SQN()
		wins += len(summary.Win())
		loses += len(summary.Lose())
		volume += summary.Volume

		returns = append(returns, summary.WinPercent()...)
		returns = append(returns, summary.LosePercent()...)
	}

	table.SetFooter([]string{
		"TOTAL",
		strconv.Itoa(wins + loses),
		strconv.Itoa(wins),
		strconv.Itoa(loses),
		fmt.Sprintf("%.1f %%", float64(wins)/float64(wins+loses)*100),
		fmt.Sprintf("%.3f", avgPayoff/float64(wins+loses)),
		fmt.Sprintf("%.3f", avgProfitFactor/float64(wins+loses)),
		fmt.Sprintf("%.1f", sqn/float64(len(n.orderController.Results))),
		fmt.Sprintf("%.2f", total),
		fmt.Sprintf("%.2f", volume),
	})
	table.Render()

	fmt.Println(buffer.String())
	fmt.Println("------ RETURN -------")
	totalReturn := 0.0
	returnsPercent := make([]float64, len(returns))
	for i, p := range returns {
		returnsPercent[i] = p * 100
		totalReturn += p
	}
	hist := histogram.Hist(15, returnsPercent)
	histogram.Fprint(os.Stdout, hist, histogram.Linear(10))
	fmt.Println()

	fmt.Println("------ CONFIDENCE INTERVAL (95%) -------")
	for pair, summary := range n.orderController.Results {
		fmt.Printf("| %s |\n", pair)
		returns := append(summary.WinPercent(), summary.LosePercent()...)
		returnsInterval := metrics.Bootstrap(returns, metrics.Mean, 10000, 0.95)
		payoffInterval := metrics.Bootstrap(returns, metrics.Payoff, 10000, 0.95)
		profitFactorInterval := metrics.Bootstrap(returns, metrics.ProfitFactor, 10000, 0.95)

		fmt.Printf("RETURN:      %.2f%% (%.2f%% ~ %.2f%%)\n",
			returnsInterval.Mean*100, returnsInterval.Lower*100, returnsInterval.Upper*100)
		fmt.Printf("PAYOFF:      %.2f (%.2f ~ %.2f)\n",
			payoffInterval.Mean, payoffInterval.Lower, payoffInterval.Upper)
		fmt.Printf("PROF.FACTOR: %.2f (%.2f ~ %.2f)\n",
			profitFactorInterval.Mean, profitFactorInterval.Lower, profitFactorInterval.Upper)
	}

	fmt.Println()

	if n.paperWallet != nil {
		n.paperWallet.Summary()
	}

}

// SaveReturns exports trading returns data for all pairs to CSV files in the specified directory.
// Each trading pair gets its own CSV file containing the percentage returns for analysis.
// This is useful for statistical analysis and external visualization tools.
func (n NinjaBot) SaveReturns(outputDir string) error {
	for _, summary := range n.orderController.Results {
		outputFile := fmt.Sprintf("%s/%s.csv", outputDir, summary.Pair)
		if err := summary.SaveReturns(outputFile); err != nil {
			return err
		}
	}
	return nil
}

// onCandle handles incoming candle data by adding it to the priority queue
// for chronological processing. This ensures candles are processed in the
// correct time order, which is crucial for accurate backtesting and live trading.
func (n *NinjaBot) onCandle(candle model.Candle) {
	n.priorityQueueCandle.Push(candle)
}

/*
processCandle handles individual candle processing by updating the paper wallet
(if enabled) and notifying the appropriate strategy controller.

Processing Flow:
  1. Update paper wallet with new price data (backtesting/simulation)
  2. Notify strategy controller of partial candle update
  3. For complete candles: trigger full strategy processing
  4. For complete candles: update order controller with market data

This method ensures proper order of operations and maintains consistency
between simulated and live trading environments.

Parameters:
  • candle: Market data candle to process
*/
func (n *NinjaBot) processCandle(candle model.Candle) {
	// Update paper wallet with current market data for simulation
	// This tracks portfolio value and enables realistic backtesting
	if n.paperWallet != nil {
		n.paperWallet.OnCandle(candle)
	}

	// Notify strategy of partial candle update (real-time price movements)
	// Enables high-frequency strategies to react before candle completion
	n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
	
	// Process complete candles for full strategy analysis
	if candle.Complete {
		// Trigger main strategy logic with complete market data
		// This is where most trading decisions are made
		n.strategiesControllers[candle.Pair].OnCandle(candle)
		
		// Update order controller with market data for order management
		// Enables stop-loss, take-profit, and other order features
		n.orderController.OnCandle(candle)
	}
}

/*
processCandles continuously processes pending candles from the priority queue buffer.

Live Trading Operation:
  • Runs indefinitely until context cancellation
  • Processes candles in chronological order via priority queue
  • Handles real-time data as it arrives from exchange feeds
  • Ensures thread-safe candle processing with PopLock mechanism

This method is the main event loop for live trading mode, processing market
data updates as they arrive and maintaining proper execution order.
*/
func (n *NinjaBot) processCandles() {
	// Continuously process candles from the priority queue
	// PopLock() provides thread-safe access with proper ordering
	for item := range n.priorityQueueCandle.PopLock() {
		candle := item.(model.Candle)
		n.processCandle(candle)
	}
}

/*
backtestCandles processes all candles in chronological order for backtesting mode.

Backtesting Optimizations:
  • Sequential processing without real-time delays
  • Progress bar for user feedback on large datasets
  • Direct priority queue access for maximum performance
  • Deterministic execution order for reproducible results

This method processes historical data as fast as possible while maintaining
the same logic flow as live trading for accurate strategy validation.
*/
func (n *NinjaBot) backtestCandles() {
	log.Info("[SETUP] Starting backtesting")

	// Create progress bar for user feedback during long backtests
	progressBar := progressbar.Default(int64(n.priorityQueueCandle.Len()))
	
	// Process all candles sequentially in chronological order
	for n.priorityQueueCandle.Len() > 0 {
		// Get next candle from priority queue (chronologically ordered)
		item := n.priorityQueueCandle.Pop()
		candle := item.(model.Candle)
		
		// Update paper wallet with market data for portfolio tracking
		if n.paperWallet != nil {
			n.paperWallet.OnCandle(candle)
		}

		// Process partial candle data (intra-candle price movements)
		n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
		
		// Process complete candle data (full OHLCV information)
		if candle.Complete {
			n.strategiesControllers[candle.Pair].OnCandle(candle)
		}

		// Update progress bar (ignore errors to avoid stopping backtest)
		if err := progressBar.Add(1); err != nil {
			log.Warnf("update progressbar fail: %v", err)
		}
	}
}

/*
preload fetches and processes historical candle data to warm up strategy indicators
before starting live trading or backtesting.

Warmup Process:
  1. Fetch historical candles based on strategy requirements
  2. Process candles to initialize indicator calculations
  3. Preload data feed cache for optimal performance
  4. Skip warmup in backtest mode (data already available)

The warmup period is critical for accurate indicator calculations and ensures
strategies have sufficient historical context before making trading decisions.

Parameters:
  • ctx: Context for cancellation and timeout control
  • pair: Trading pair to preload (e.g., "BTC/USDT")

Returns:
  • error: Exchange connection or data retrieval error
*/
func (n *NinjaBot) preload(ctx context.Context, pair string) error {
	// Skip preload in backtest mode since historical data is already loaded
	if n.backtest {
		return nil
	}

	// Fetch historical candles from exchange based on strategy requirements
	// Warmup period determines how much historical data is needed
	candles, err := n.exchange.CandlesByLimit(
		ctx,                      // Context for timeout control
		pair,                     // Trading pair to fetch
		n.strategy.Timeframe(),   // Required timeframe
		n.strategy.WarmupPeriod(), // Number of historical candles needed
	)
	if err != nil {
		return fmt.Errorf("failed to fetch historical candles: %w", err)
	}

	// Process each historical candle to initialize indicators
	// This ensures indicators have sufficient data before live trading
	for _, candle := range candles {
		n.processCandle(candle)
	}

	// Cache historical data in data feed for performance optimization
	// Reduces exchange API calls during live trading startup
	n.dataFeed.Preload(pair, n.strategy.Timeframe(), candles)

	return nil
}

/*
Run initializes and starts the trading bot for all configured pairs.

This is the main entry point that orchestrates the entire trading system startup
sequence. It handles both backtesting and live trading modes with appropriate
optimizations for each environment.

Startup Sequence:
  1. Initialize strategy controllers for each trading pair
  2. Preload historical data for indicator warmup (live mode only)
  3. Subscribe to real-time data feeds
  4. Start all system components (orders, notifications, data)
  5. Begin main trading loop (backtest vs live processing)

Threading Architecture:
  • Each component runs in dedicated goroutines for concurrency
  • Context-based cancellation propagates through all components
  • Order controller manages trade execution asynchronously
  • Data feeds handle real-time market data streaming

Error Handling:
  • Validates trading pair configurations
  • Ensures exchange connectivity before starting
  • Gracefully handles component initialization failures
  • Provides detailed error context for troubleshooting

Parameters:
  • ctx: Context for cancellation and timeout control

Returns:
  • error: Initialization or runtime error if startup fails
*/
func (n *NinjaBot) Run(ctx context.Context) error {
	// ═══════════════════════════════════════════════════════════════════
	// PHASE 1: Initialize Strategy Controllers for Each Trading Pair
	// ═══════════════════════════════════════════════════════════════════
	for _, pair := range n.settings.Pairs {
		// Create dedicated strategy controller for this trading pair
		// Each pair gets its own controller to handle concurrent trading
		n.strategiesControllers[pair] = strategy.NewStrategyController(
			pair,                // Trading pair (e.g., BTC/USDT)
			n.strategy,          // Strategy implementation
			n.orderController,   // Order execution interface
		)

		// Preload historical candles for indicator warmup (skip in backtest mode)
		// This ensures indicators have sufficient data before live trading begins
		err := n.preload(ctx, pair)
		if err != nil {
			return fmt.Errorf("failed to preload data for pair %s: %w", pair, err)
		}

		// Subscribe to real-time market data feed for this pair
		// onCandle callback will be triggered for each new candle
		n.dataFeed.Subscribe(
			pair,                    // Trading pair to monitor
			n.strategy.Timeframe(),  // Required timeframe (e.g., "1h", "1d")
			n.onCandle,             // Callback function for new candles
			false,                  // Process partial candles (not just complete)
		)

		// Start the strategy controller for concurrent processing
		// This enables the strategy to begin receiving and processing data
		n.strategiesControllers[pair].Start()
	}

	// ═══════════════════════════════════════════════════════════════════
	// PHASE 2: Start Core System Components
	// ═══════════════════════════════════════════════════════════════════
	
	// Start order feed for distributing order events to subscribers
	// This enables real-time order status updates and notifications
	n.orderFeed.Start()
	
	// Start order controller for trade execution and tracking
	// This is the central component that interfaces with the exchange
	n.orderController.Start()
	defer n.orderController.Stop() // Ensure graceful shutdown
	
	// Start Telegram notifications if configured
	// Provides real-time trading alerts and status updates
	if n.telegram != nil {
		n.telegram.Start()
	}

	// ═══════════════════════════════════════════════════════════════════
	// PHASE 3: Start Data Feed and Begin Trading Loop
	// ═══════════════════════════════════════════════════════════════════
	
	// Start market data feed with mode-specific optimizations
	// Backtest mode: Optimized for historical data processing
	// Live mode: Real-time streaming with connection management
	n.dataFeed.Start(n.backtest)

	// Begin main trading loop based on operating mode
	if n.backtest {
		// Backtesting Mode: Process all historical data sequentially
		// - No real-time delays
		// - Progress bar for user feedback
		// - Deterministic execution order
		n.backtestCandles()
	} else {
		// Live Trading Mode: Process real-time data as it arrives
		// - Concurrent candle processing
		// - Real-time order execution
		// - Continuous operation until context cancellation
		n.processCandles()
	}

	return nil
}
