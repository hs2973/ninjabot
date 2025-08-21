package exchange

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	"github.com/jpillora/backoff"

	"github.com/rodrigo-brito/ninjabot/model"
	"github.com/rodrigo-brito/ninjabot/tools/log"
)

// MetadataFetchers defines a function type for fetching additional metadata
// that gets attached to candle data during real-time streaming.
type MetadataFetchers func(pair string, t time.Time) (string, float64)

/*
Binance implements the Exchange interface for Binance cryptocurrency exchange integration.
It provides comprehensive trading capabilities including order management, market data
streaming, account management, and both testnet and mainnet support.

Key Features:
  • Real-time market data streaming via WebSocket
  • Full order management (market, limit, stop-loss, OCO orders)
  • Account and position tracking
  • Heikin-Ashi candle conversion support
  • Custom metadata fetching for enhanced analysis
  • Automatic precision handling for assets
  • Testnet support for safe development

Configuration Options:
  • API credentials for authenticated trading
  • Custom endpoint configuration for private deployments
  • Heikin-Ashi candle transformation
  • Testnet vs mainnet operation mode
*/
type Binance struct {
	ctx        context.Context             // Request context for API calls
	client     *binance.Client             // Official Binance API client
	assetsInfo map[string]model.AssetInfo  // Asset trading rules and precision info
	HeikinAshi bool                        // Enable Heikin-Ashi candle conversion
	Testnet    bool                        // Use testnet endpoints

	// Authentication credentials for API access
	APIKey    string  // Binance API key
	APISecret string  // Binance API secret

	// Custom data enhancement during streaming
	MetadataFetchers []MetadataFetchers  // Functions to fetch additional candle metadata
}

// BinanceOption defines a function type for configuring Binance exchange options.
type BinanceOption func(*Binance)

// WithBinanceCredentials configures API credentials for authenticated trading operations.
// Both key and secret are required for any trading activities including order placement,
// account queries, and position management.
func WithBinanceCredentials(key, secret string) BinanceOption {
	return func(b *Binance) {
		b.APIKey = key
		b.APISecret = secret
	}
}

// WithBinanceHeikinAshiCandle enables automatic conversion of standard candles to Heikin-Ashi format.
// Heikin-Ashi candles provide smoother trend visualization by using modified OHLC calculations
// that reduce market noise and highlight trend direction more clearly.
func WithBinanceHeikinAshiCandle() BinanceOption {
	return func(b *Binance) {
		b.HeikinAshi = true
	}
}

// WithMetadataFetcher registers a custom function to fetch additional metadata for each candle.
// The fetcher function is called after receiving each complete candle and can attach
// custom indicators, external data, or computed values to the candle's metadata map.
//
// Example usage:
//   fetcher := func(pair string, t time.Time) (string, float64) {
//       return "volume_sma", calculateVolumeSMA(pair, t)
//   }
//   exchange := NewBinance(ctx, WithMetadataFetcher(fetcher))
func WithMetadataFetcher(fetcher MetadataFetchers) BinanceOption {
	return func(b *Binance) {
		b.MetadataFetchers = append(b.MetadataFetchers, fetcher)
	}
}

// WithTestNet activates Binance testnet mode for safe development and testing.
// Testnet provides a sandbox environment with virtual funds for testing trading
// strategies without risking real capital.
func WithTestNet() BinanceOption {
	return func(_ *Binance) {
		binance.UseTestnet = true
	}
}

// WithCustomMainAPIEndpoint configures custom endpoints for Binance mainnet API access.
// This is useful for routing through proxy servers or using region-specific endpoints.
// All three URLs (API, WebSocket, Combined) must be provided and non-empty.
func WithCustomMainAPIEndpoint(apiURL, wsURL, combinedURL string) BinanceOption {
	if apiURL == "" || wsURL == "" || combinedURL == "" {
		log.Fatal("missing url parameters for custom endpoint configuration")
	}

	return func(_ *Binance) {
		binance.BaseAPIMainURL = apiURL
		binance.BaseWsMainURL = wsURL
		binance.BaseCombinedMainURL = combinedURL
	}
}

// WithCustomTestnetAPIEndpoint configures custom endpoints for Binance testnet API access.
// Similar to mainnet configuration but applies to testnet infrastructure.
// All three URLs (API, WebSocket, Combined) must be provided and non-empty.
func WithCustomTestnetAPIEndpoint(apiURL, wsURL, combinedURL string) BinanceOption {
	if apiURL == "" || wsURL == "" || combinedURL == "" {
		log.Fatal("missing url parameters for custom endpoint configuration")
	}

	return func(_ *Binance) {
		binance.BaseAPITestnetURL = apiURL
		binance.BaseWsTestnetURL = wsURL
		binance.BaseCombinedTestnetURL = combinedURL
	}
}

/*
NewBinance creates a new Binance exchange instance with comprehensive initialization.

Initialization Process:
  1. Enable WebSocket keepalive for stable connections
  2. Apply all configuration options (credentials, endpoints, etc.)
  3. Initialize the official Binance API client
  4. Test connectivity with ping service
  5. Fetch exchange information and trading rules
  6. Parse asset precision and trading limits for all symbols
  7. Cache asset information for order validation

The function retrieves and caches detailed trading rules for each symbol including:
  • Minimum/maximum order quantities and step sizes
  • Price filters with tick sizes and ranges
  • Base/quote asset precision settings

Parameters:
  • ctx: Context for request lifecycle management
  • options: Functional options for exchange configuration

Returns initialized Binance exchange or error if setup fails.
*/
func NewBinance(ctx context.Context, options ...BinanceOption) (*Binance, error) {
	// Enable persistent WebSocket connections for real-time data
	binance.WebsocketKeepalive = true
	
	// Initialize exchange with context
	exchange := &Binance{ctx: ctx}
	
	// Apply all configuration options
	for _, option := range options {
		option(exchange)
	}

	// Create authenticated API client
	exchange.client = binance.NewClient(exchange.APIKey, exchange.APISecret)
	
	// Test API connectivity
	err := exchange.client.NewPingService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("binance ping fail: %w", err)
	}

	// Fetch exchange trading rules and symbol information
	results, err := exchange.client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize asset information cache with precision and trading limits
	exchange.assetsInfo = make(map[string]model.AssetInfo)
	for _, info := range results.Symbols {
		tradeLimits := model.AssetInfo{
			BaseAsset:          info.BaseAsset,
			QuoteAsset:         info.QuoteAsset,
			BaseAssetPrecision: info.BaseAssetPrecision,
			QuotePrecision:     info.QuotePrecision,
		}
		
		// Parse trading filters for order validation
		for _, filter := range info.Filters {
			if typ, ok := filter["filterType"]; ok {
				// Extract lot size constraints for quantity validation
				if typ == string(binance.SymbolFilterTypeLotSize) {
					tradeLimits.MinQuantity, _ = strconv.ParseFloat(filter["minQty"].(string), 64)
					tradeLimits.MaxQuantity, _ = strconv.ParseFloat(filter["maxQty"].(string), 64)
					tradeLimits.StepSize, _ = strconv.ParseFloat(filter["stepSize"].(string), 64)
				}

				// Extract price filter constraints for price validation
				if typ == string(binance.SymbolFilterTypePriceFilter) {
					tradeLimits.MinPrice, _ = strconv.ParseFloat(filter["minPrice"].(string), 64)
					tradeLimits.MaxPrice, _ = strconv.ParseFloat(filter["maxPrice"].(string), 64)
					tradeLimits.TickSize, _ = strconv.ParseFloat(filter["tickSize"].(string), 64)
				}
			}
		}
		exchange.assetsInfo[info.Symbol] = tradeLimits
	}

	log.Info("[SETUP] Using Binance exchange")

	return exchange, nil
}

// LastQuote retrieves the most recent closing price for the specified trading pair.
// It fetches the latest 1-minute candle and returns the closing price.
func (b *Binance) LastQuote(ctx context.Context, pair string) (float64, error) {
	candles, err := b.CandlesByLimit(ctx, pair, "1m", 1)
	if err != nil || len(candles) < 1 {
		return 0, err
	}
	return candles[0].Close, nil
}

// AssetsInfo returns the cached trading rules and precision information for a trading pair.
// This includes minimum/maximum quantities, price ranges, step sizes, and decimal precision.
func (b *Binance) AssetsInfo(pair string) model.AssetInfo {
	return b.assetsInfo[pair]
}

// validate checks if an order quantity complies with the exchange's trading rules.
// It verifies the asset exists and the quantity falls within min/max limits.
func (b *Binance) validate(pair string, quantity float64) error {
	info, ok := b.assetsInfo[pair]
	if !ok {
		return ErrInvalidAsset
	}

	if quantity > info.MaxQuantity || quantity < info.MinQuantity {
		return &OrderError{
			Err:      fmt.Errorf("%w: min: %f max: %f", ErrInvalidQuantity, info.MinQuantity, info.MaxQuantity),
			Pair:     pair,
			Quantity: quantity,
		}
	}

	return nil
}

/*
CreateOrderOCO creates an OCO (One-Cancels-Other) order consisting of a limit order and a stop-limit order.
OCO orders are useful for profit-taking and stop-loss strategies where only one side should execute.

Parameters:
  • side: Buy or sell direction
  • pair: Trading pair symbol (e.g., "BTCUSDT")  
  • quantity: Order size in base asset units
  • price: Limit order price
  • stop: Stop trigger price
  • stopLimit: Stop-limit order execution price

The function validates quantity against exchange rules and returns both orders created.
If either order executes, the other is automatically cancelled by the exchange.
*/
func (b *Binance) CreateOrderOCO(side model.SideType, pair string,
	quantity, price, stop, stopLimit float64) ([]model.Order, error) {

	// Validate order quantity against exchange trading rules
	err := b.validate(pair, quantity)
	if err != nil {
		return nil, err
	}

	// Submit OCO order to exchange
	ocoOrder, err := b.client.NewCreateOCOService().
		Side(binance.SideType(side)).
		Quantity(b.formatQuantity(pair, quantity)).
		Price(b.formatPrice(pair, price)).
		StopPrice(b.formatPrice(pair, stop)).
		StopLimitPrice(b.formatPrice(pair, stopLimit)).
		StopLimitTimeInForce(binance.TimeInForceTypeGTC).
		Symbol(pair).
		Do(b.ctx)
	if err != nil {
		return nil, err
	}

	// Convert exchange response to internal order format
	orders := make([]model.Order, 0, len(ocoOrder.Orders))
	for _, order := range ocoOrder.OrderReports {
		price, _ := strconv.ParseFloat(order.Price, 64)
		quantity, _ := strconv.ParseFloat(order.OrigQuantity, 64)
		item := model.Order{
			ExchangeID: order.OrderID,
			CreatedAt:  time.Unix(0, ocoOrder.TransactionTime*int64(time.Millisecond)),
			UpdatedAt:  time.Unix(0, ocoOrder.TransactionTime*int64(time.Millisecond)),
			Pair:       pair,
			Side:       model.SideType(order.Side),
			Type:       model.OrderType(order.Type),
			Status:     model.OrderStatusType(order.Status),
			Price:      price,
			Quantity:   quantity,
			GroupID:    &order.OrderListID,
		}

		// Add stop price for stop-loss orders
		if item.Type == model.OrderTypeStopLossLimit || item.Type == model.OrderTypeStopLoss {
			item.Stop = &stop
		}

		orders = append(orders, item)
	}

	return orders, nil
}

func (b *Binance) CreateOrderStop(pair string, quantity float64, limit float64) (model.Order, error) {
	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	order, err := b.client.NewCreateOrderService().Symbol(pair).
		Type(binance.OrderTypeStopLoss).
		TimeInForce(binance.TimeInForceTypeGTC).
		Side(binance.SideTypeSell).
		Quantity(b.formatQuantity(pair, quantity)).
		Price(b.formatPrice(pair, limit)).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	price, _ := strconv.ParseFloat(order.Price, 64)
	quantity, _ = strconv.ParseFloat(order.OrigQuantity, 64)

	return model.Order{
		ExchangeID: order.OrderID,
		CreatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		UpdatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		Pair:       pair,
		Side:       model.SideType(order.Side),
		Type:       model.OrderType(order.Type),
		Status:     model.OrderStatusType(order.Status),
		Price:      price,
		Quantity:   quantity,
	}, nil
}

func (b *Binance) formatPrice(pair string, value float64) string {
	if info, ok := b.assetsInfo[pair]; ok {
		value = common.AmountToLotSize(info.TickSize, info.QuotePrecision, value)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (b *Binance) formatQuantity(pair string, value float64) string {
	if info, ok := b.assetsInfo[pair]; ok {
		value = common.AmountToLotSize(info.StepSize, info.BaseAssetPrecision, value)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (b *Binance) CreateOrderLimit(side model.SideType, pair string,
	quantity float64, limit float64) (model.Order, error) {

	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		Type(binance.OrderTypeLimit).
		TimeInForce(binance.TimeInForceTypeGTC).
		Side(binance.SideType(side)).
		Quantity(b.formatQuantity(pair, quantity)).
		Price(b.formatPrice(pair, limit)).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	price, err := strconv.ParseFloat(order.Price, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.OrigQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID: order.OrderID,
		CreatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		UpdatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		Pair:       pair,
		Side:       model.SideType(order.Side),
		Type:       model.OrderType(order.Type),
		Status:     model.OrderStatusType(order.Status),
		Price:      price,
		Quantity:   quantity,
	}, nil
}

func (b *Binance) CreateOrderMarket(side model.SideType, pair string, quantity float64) (model.Order, error) {
	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		Type(binance.OrderTypeMarket).
		Side(binance.SideType(side)).
		Quantity(b.formatQuantity(pair, quantity)).
		NewOrderRespType(binance.NewOrderRespTypeFULL).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	cost, err := strconv.ParseFloat(order.CummulativeQuoteQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.ExecutedQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID: order.OrderID,
		CreatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		UpdatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		Pair:       order.Symbol,
		Side:       model.SideType(order.Side),
		Type:       model.OrderType(order.Type),
		Status:     model.OrderStatusType(order.Status),
		Price:      cost / quantity,
		Quantity:   quantity,
	}, nil
}

func (b *Binance) CreateOrderMarketQuote(side model.SideType, pair string, quantity float64) (model.Order, error) {
	err := b.validate(pair, quantity)
	if err != nil {
		return model.Order{}, err
	}

	order, err := b.client.NewCreateOrderService().
		Symbol(pair).
		Type(binance.OrderTypeMarket).
		Side(binance.SideType(side)).
		QuoteOrderQty(b.formatQuantity(pair, quantity)).
		NewOrderRespType(binance.NewOrderRespTypeFULL).
		Do(b.ctx)
	if err != nil {
		return model.Order{}, err
	}

	cost, err := strconv.ParseFloat(order.CummulativeQuoteQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	quantity, err = strconv.ParseFloat(order.ExecutedQuantity, 64)
	if err != nil {
		return model.Order{}, err
	}

	return model.Order{
		ExchangeID: order.OrderID,
		CreatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		UpdatedAt:  time.Unix(0, order.TransactTime*int64(time.Millisecond)),
		Pair:       order.Symbol,
		Side:       model.SideType(order.Side),
		Type:       model.OrderType(order.Type),
		Status:     model.OrderStatusType(order.Status),
		Price:      cost / quantity,
		Quantity:   quantity,
	}, nil
}

func (b *Binance) Cancel(order model.Order) error {
	_, err := b.client.NewCancelOrderService().
		Symbol(order.Pair).
		OrderID(order.ExchangeID).
		Do(b.ctx)
	return err
}

func (b *Binance) Orders(pair string, limit int) ([]model.Order, error) {
	result, err := b.client.NewListOrdersService().
		Symbol(pair).
		Limit(limit).
		Do(b.ctx)

	if err != nil {
		return nil, err
	}

	orders := make([]model.Order, 0)
	for _, order := range result {
		orders = append(orders, newOrder(order))
	}
	return orders, nil
}

func (b *Binance) Order(pair string, id int64) (model.Order, error) {
	order, err := b.client.NewGetOrderService().
		Symbol(pair).
		OrderID(id).
		Do(b.ctx)

	if err != nil {
		return model.Order{}, err
	}

	return newOrder(order), nil
}

func newOrder(order *binance.Order) model.Order {
	var price float64
	cost, _ := strconv.ParseFloat(order.CummulativeQuoteQuantity, 64)
	quantity, _ := strconv.ParseFloat(order.ExecutedQuantity, 64)
	if cost > 0 && quantity > 0 {
		price = cost / quantity
	} else {
		price, _ = strconv.ParseFloat(order.Price, 64)
		quantity, _ = strconv.ParseFloat(order.OrigQuantity, 64)
	}

	return model.Order{
		ExchangeID: order.OrderID,
		Pair:       order.Symbol,
		CreatedAt:  time.Unix(0, order.Time*int64(time.Millisecond)),
		UpdatedAt:  time.Unix(0, order.UpdateTime*int64(time.Millisecond)),
		Side:       model.SideType(order.Side),
		Type:       model.OrderType(order.Type),
		Status:     model.OrderStatusType(order.Status),
		Price:      price,
		Quantity:   quantity,
	}
}

func (b *Binance) Account() (model.Account, error) {
	acc, err := b.client.NewGetAccountService().Do(b.ctx)
	if err != nil {
		return model.Account{}, err
	}

	balances := make([]model.Balance, 0)
	for _, balance := range acc.Balances {
		free, err := strconv.ParseFloat(balance.Free, 64)
		if err != nil {
			return model.Account{}, err
		}
		locked, err := strconv.ParseFloat(balance.Locked, 64)
		if err != nil {
			return model.Account{}, err
		}
		balances = append(balances, model.Balance{
			Asset: balance.Asset,
			Free:  free,
			Lock:  locked,
		})
	}

	return model.Account{
		Balances: balances,
	}, nil
}

func (b *Binance) Position(pair string) (asset, quote float64, err error) {
	assetTick, quoteTick := SplitAssetQuote(pair)
	acc, err := b.Account()
	if err != nil {
		return 0, 0, err
	}

	assetBalance, quoteBalance := acc.Balance(assetTick, quoteTick)

	return assetBalance.Free + assetBalance.Lock, quoteBalance.Free + quoteBalance.Lock, nil
}

func (b *Binance) CandlesSubscription(ctx context.Context, pair, period string) (chan model.Candle, chan error) {
	ccandle := make(chan model.Candle)
	cerr := make(chan error)
	ha := model.NewHeikinAshi()

	go func() {
		ba := &backoff.Backoff{
			Min: 100 * time.Millisecond,
			Max: 1 * time.Second,
		}

		for {
			done, _, err := binance.WsKlineServe(pair, period, func(event *binance.WsKlineEvent) {
				ba.Reset()
				candle := CandleFromWsKline(pair, event.Kline)

				if candle.Complete && b.HeikinAshi {
					candle = candle.ToHeikinAshi(ha)
				}

				if candle.Complete {
					// fetch aditional data if needed
					for _, fetcher := range b.MetadataFetchers {
						key, value := fetcher(pair, candle.Time)
						candle.Metadata[key] = value
					}
				}

				ccandle <- candle

			}, func(err error) {
				cerr <- err
			})
			if err != nil {
				cerr <- err
				close(cerr)
				close(ccandle)
				return
			}

			select {
			case <-ctx.Done():
				close(cerr)
				close(ccandle)
				return
			case <-done:
				time.Sleep(ba.Duration())
			}
		}
	}()

	return ccandle, cerr
}

func (b *Binance) CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error) {
	candles := make([]model.Candle, 0)
	klineService := b.client.NewKlinesService()
	ha := model.NewHeikinAshi()

	data, err := klineService.Symbol(pair).
		Interval(period).
		Limit(limit + 1).
		Do(ctx)

	if err != nil {
		return nil, err
	}

	for _, d := range data {
		candle := CandleFromKline(pair, *d)

		if b.HeikinAshi {
			candle = candle.ToHeikinAshi(ha)
		}

		candles = append(candles, candle)
	}

	// discard last candle, because it is incomplete
	return candles[:len(candles)-1], nil
}

func (b *Binance) CandlesByPeriod(ctx context.Context, pair, period string,
	start, end time.Time) ([]model.Candle, error) {

	candles := make([]model.Candle, 0)
	klineService := b.client.NewKlinesService()
	ha := model.NewHeikinAshi()

	data, err := klineService.Symbol(pair).
		Interval(period).
		StartTime(start.UnixNano() / int64(time.Millisecond)).
		EndTime(end.UnixNano() / int64(time.Millisecond)).
		Do(ctx)

	if err != nil {
		return nil, err
	}

	for _, d := range data {
		candle := CandleFromKline(pair, *d)

		if b.HeikinAshi {
			candle = candle.ToHeikinAshi(ha)
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

func CandleFromKline(pair string, k binance.Kline) model.Candle {
	t := time.Unix(0, k.OpenTime*int64(time.Millisecond))
	candle := model.Candle{Pair: pair, Time: t, UpdatedAt: t}
	candle.Open, _ = strconv.ParseFloat(k.Open, 64)
	candle.Close, _ = strconv.ParseFloat(k.Close, 64)
	candle.High, _ = strconv.ParseFloat(k.High, 64)
	candle.Low, _ = strconv.ParseFloat(k.Low, 64)
	candle.Volume, _ = strconv.ParseFloat(k.Volume, 64)
	candle.Complete = true
	candle.Metadata = make(map[string]float64)
	return candle
}

func CandleFromWsKline(pair string, k binance.WsKline) model.Candle {
	t := time.Unix(0, k.StartTime*int64(time.Millisecond))
	candle := model.Candle{Pair: pair, Time: t, UpdatedAt: t}
	candle.Open, _ = strconv.ParseFloat(k.Open, 64)
	candle.Close, _ = strconv.ParseFloat(k.Close, 64)
	candle.High, _ = strconv.ParseFloat(k.High, 64)
	candle.Low, _ = strconv.ParseFloat(k.Low, 64)
	candle.Volume, _ = strconv.ParseFloat(k.Volume, 64)
	candle.Complete = k.IsFinal
	candle.Metadata = make(map[string]float64)
	return candle
}
