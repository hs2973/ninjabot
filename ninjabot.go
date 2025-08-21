// Package ninjabot provides a comprehensive framework for building cryptocurrency trading bots.
// It supports backtesting, paper trading, and live trading with various exchanges.
// The package includes strategy management, order execution, data feeds, notifications,
// and comprehensive analysis tools for algorithmic trading.
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
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04",
	})
}

// OrderSubscriber defines the interface for components that need to be notified
// when orders are created, updated, or filled. Implementing types can react
// to order events for logging, notifications, or strategy adjustments.
type OrderSubscriber interface {
	OnOrder(model.Order)
}

// CandleSubscriber defines the interface for components that need to be notified
// when new candle data becomes available. This is typically used by strategies
// and indicators that need to process market data as it arrives.
type CandleSubscriber interface {
	OnCandle(model.Candle)
}

// NinjaBot represents the main trading bot instance that coordinates all components
// including exchange connections, strategy execution, order management, data feeds,
// and notifications. It supports both backtesting and live trading modes.
type NinjaBot struct {
	storage  storage.Storage
	settings model.Settings
	exchange service.Exchange
	strategy strategy.Strategy
	notifier service.Notifier
	telegram service.Telegram

	orderController       *order.Controller
	priorityQueueCandle   *model.PriorityQueue
	strategiesControllers map[string]*strategy.Controller
	orderFeed             *order.Feed
	dataFeed              *exchange.DataFeedSubscription
	paperWallet           *exchange.PaperWallet

	backtest bool
}

// Option represents a functional option pattern for configuring NinjaBot instances.
// This allows for flexible and extensible configuration without breaking changes
// to the constructor function signature.
type Option func(*NinjaBot)

// NewBot creates a new NinjaBot instance with the specified settings, exchange, and strategy.
// It initializes all necessary components including order management, data feeds, and storage.
// Additional configuration can be applied using the functional options pattern.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - settings: Bot configuration including pairs and notifications
//   - exch: Exchange interface for trading operations
//   - str: Trading strategy to execute
//   - options: Optional configuration functions
//
// Returns:
//   - *NinjaBot: Configured bot instance
//   - error: Configuration or initialization error
func NewBot(ctx context.Context, settings model.Settings, exch service.Exchange, str strategy.Strategy,
	options ...Option) (*NinjaBot, error) {

	bot := &NinjaBot{
		settings:              settings,
		exchange:              exch,
		strategy:              str,
		orderFeed:             order.NewOrderFeed(),
		dataFeed:              exchange.NewDataFeed(exch),
		strategiesControllers: make(map[string]*strategy.Controller),
		priorityQueueCandle:   model.NewPriorityQueue(nil),
	}

	for _, pair := range settings.Pairs {
		asset, quote := exchange.SplitAssetQuote(pair)
		if asset == "" || quote == "" {
			return nil, fmt.Errorf("invalid pair: %s", pair)
		}
	}

	for _, option := range options {
		option(bot)
	}

	var err error
	if bot.storage == nil {
		bot.storage, err = storage.FromFile(defaultDatabase)
		if err != nil {
			return nil, err
		}
	}

	bot.orderController = order.NewController(ctx, exch, bot.storage, bot.orderFeed)

	if settings.Telegram.Enabled {
		bot.telegram, err = notification.NewTelegram(bot.orderController, settings)
		if err != nil {
			return nil, err
		}
		// register telegram as notifier
		WithNotifier(bot.telegram)(bot)
	}

	return bot, nil
}

// WithBacktest sets the bot to run in backtest mode, it is required for backtesting environments
// Backtest mode optimize the input read for CSV and deal with race conditions
func WithBacktest(wallet *exchange.PaperWallet) Option {
	return func(bot *NinjaBot) {
		bot.backtest = true
		opt := WithPaperWallet(wallet)
		opt(bot)
	}
}

// WithStorage sets the storage for the bot, by default it uses a local file called ninjabot.db
func WithStorage(storage storage.Storage) Option {
	return func(bot *NinjaBot) {
		bot.storage = storage
	}
}

// WithLogLevel sets the log level. eg: log.DebugLevel, log.InfoLevel, log.WarnLevel, log.ErrorLevel, log.FatalLevel
func WithLogLevel(level log.Level) Option {
	return func(_ *NinjaBot) {
		log.SetLevel(level)
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

// processCandle handles individual candle processing by updating the paper wallet
// (if enabled) and notifying the appropriate strategy controller. Complete candles
// trigger full strategy processing and order controller updates.
func (n *NinjaBot) processCandle(candle model.Candle) {
	if n.paperWallet != nil {
		n.paperWallet.OnCandle(candle)
	}

	n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
	if candle.Complete {
		n.strategiesControllers[candle.Pair].OnCandle(candle)
		n.orderController.OnCandle(candle)
	}
}

// processCandles continuously processes pending candles from the priority queue buffer.
// This method runs in production mode to handle real-time candle updates as they arrive
// from the exchange data feed, ensuring proper chronological order.
func (n *NinjaBot) processCandles() {
	for item := range n.priorityQueueCandle.PopLock() {
		n.processCandle(item.(model.Candle))
	}
}

// backtestCandles processes all candles in chronological order for backtesting mode.
// It displays a progress bar and processes candles sequentially without real-time delays.
// This method is optimized for historical data processing and strategy validation.
func (n *NinjaBot) backtestCandles() {
	log.Info("[SETUP] Starting backtesting")

	progressBar := progressbar.Default(int64(n.priorityQueueCandle.Len()))
	for n.priorityQueueCandle.Len() > 0 {
		item := n.priorityQueueCandle.Pop()

		candle := item.(model.Candle)
		if n.paperWallet != nil {
			n.paperWallet.OnCandle(candle)
		}

		n.strategiesControllers[candle.Pair].OnPartialCandle(candle)
		if candle.Complete {
			n.strategiesControllers[candle.Pair].OnCandle(candle)
		}

		if err := progressBar.Add(1); err != nil {
			log.Warnf("update progressbar fail: %v", err)
		}
	}
}

// preload fetches and processes historical candle data to warm up strategy indicators
// before starting live trading or backtesting. The warmup period is determined by
// the strategy's requirements and ensures indicators have sufficient data for accurate calculations.
func (n *NinjaBot) preload(ctx context.Context, pair string) error {
	if n.backtest {
		return nil
	}

	candles, err := n.exchange.CandlesByLimit(ctx, pair, n.strategy.Timeframe(), n.strategy.WarmupPeriod())
	if err != nil {
		return err
	}

	for _, candle := range candles {
		n.processCandle(candle)
	}

	n.dataFeed.Preload(pair, n.strategy.Timeframe(), candles)

	return nil
}

// Run initializes and starts the trading bot for all configured pairs.
// It sets up strategy controllers, preloads historical data for indicator warmup,
// starts data feeds and order processing, and begins the main trading loop.
// The method supports both backtesting and live trading modes.
func (n *NinjaBot) Run(ctx context.Context) error {
	for _, pair := range n.settings.Pairs {
		// setup and subscribe strategy to data feed (candles)
		n.strategiesControllers[pair] = strategy.NewStrategyController(pair, n.strategy, n.orderController)

		// preload candles for warmup period
		err := n.preload(ctx, pair)
		if err != nil {
			return err
		}

		// link to ninja bot controller
		n.dataFeed.Subscribe(pair, n.strategy.Timeframe(), n.onCandle, false)

		// start strategy controller
		n.strategiesControllers[pair].Start()
	}

	// start order feed and controller
	n.orderFeed.Start()
	n.orderController.Start()
	defer n.orderController.Stop()
	if n.telegram != nil {
		n.telegram.Start()
	}

	// start data feed and receives new candles
	n.dataFeed.Start(n.backtest)

	// start processing new candles for production or backtesting environment
	if n.backtest {
		n.backtestCandles()
	} else {
		n.processCandles()
	}

	return nil
}
