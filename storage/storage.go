// Package storage provides interfaces and implementations for persistent data storage
// of trading orders and related data. It supports multiple storage backends including
// SQL databases and embedded databases with filtering capabilities for order queries.
package storage

import (
	"time"

	"github.com/rodrigo-brito/ninjabot/model"
)

// OrderFilter defines a function type for filtering orders based on specific criteria.
type OrderFilter func(model.Order) bool

// Storage defines the interface for persistent storage of trading orders
// with support for creation, updates, and filtered queries.
type Storage interface {
	CreateOrder(order *model.Order) error
	UpdateOrder(order *model.Order) error
	Orders(filters ...OrderFilter) ([]*model.Order, error)
}

// WithStatusIn creates a filter that matches orders with any of the specified statuses.
func WithStatusIn(status ...model.OrderStatusType) OrderFilter {
	return func(order model.Order) bool {
		for _, s := range status {
			if s == order.Status {
				return true
			}
		}
		return false
	}
}

// WithStatus creates a filter that matches orders with a specific status.
func WithStatus(status model.OrderStatusType) OrderFilter {
	return func(order model.Order) bool {
		return order.Status == status
	}
}

// WithPair creates a filter that matches orders for a specific trading pair.
func WithPair(pair string) OrderFilter {
	return func(order model.Order) bool {
		return order.Pair == pair
	}
}

// WithUpdateAtBeforeOrEqual creates a filter that matches orders updated before or at the specified time.
func WithUpdateAtBeforeOrEqual(time time.Time) OrderFilter {
	return func(order model.Order) bool {
		return !order.UpdatedAt.After(time)
	}
}
