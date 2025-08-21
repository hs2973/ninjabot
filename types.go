// Package ninjabot provides type aliases and constants for convenient access to model types.
// This file centralizes commonly used types from the model package to simplify imports
// and provide a cleaner API for users of the ninjabot library.
package ninjabot

import (
	"github.com/rodrigo-brito/ninjabot/model"
)

// Type aliases for commonly used model types to provide a cleaner API
// and reduce the need for importing the model package directly.
type (
	Settings         = model.Settings
	TelegramSettings = model.TelegramSettings
	Dataframe        = model.Dataframe
	Series           = model.Series[float64]
	SideType         = model.SideType
	OrderType        = model.OrderType
	OrderStatusType  = model.OrderStatusType
)

// Constants for side types, order types, and order statuses
// These provide convenient access to model constants without importing the model package.
var (
	SideTypeBuy                    = model.SideTypeBuy
	SideTypeSell                   = model.SideTypeSell
	OrderTypeLimit                 = model.OrderTypeLimit
	OrderTypeMarket                = model.OrderTypeMarket
	OrderTypeLimitMaker            = model.OrderTypeLimitMaker
	OrderTypeStopLoss              = model.OrderTypeStopLoss
	OrderTypeStopLossLimit         = model.OrderTypeStopLossLimit
	OrderTypeTakeProfit            = model.OrderTypeTakeProfit
	OrderTypeTakeProfitLimit       = model.OrderTypeTakeProfitLimit
	OrderStatusTypeNew             = model.OrderStatusTypeNew
	OrderStatusTypePartiallyFilled = model.OrderStatusTypePartiallyFilled
	OrderStatusTypeFilled          = model.OrderStatusTypeFilled
	OrderStatusTypeCanceled        = model.OrderStatusTypeCanceled
	OrderStatusTypePendingCancel   = model.OrderStatusTypePendingCancel
	OrderStatusTypeRejected        = model.OrderStatusTypeRejected
	OrderStatusTypeExpired         = model.OrderStatusTypeExpired
)
