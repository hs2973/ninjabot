package storage

import (
	"time"

	"github.com/samber/lo"
	"gorm.io/gorm"

	"github.com/rodrigo-brito/ninjabot/model"
)

/*
SQL implements the Storage interface using GORM for SQL database persistence.
It provides full CRUD operations for trading orders with support for various
SQL databases including PostgreSQL, MySQL, SQLite, and SQL Server.

Features:
  • Automatic database schema migration
  • Connection pooling with optimized settings
  • Transaction support through GORM
  • Flexible filtering with custom filter functions
  • Support for all GORM-compatible SQL dialects
*/
type SQL struct {
	db *gorm.DB  // GORM database instance with connection pooling
}

/*
FromSQL creates a new SQL storage instance with database connection and schema setup.

Initialization process:
  1. Establish database connection using provided dialect
  2. Configure connection pool with optimized settings
  3. Auto-migrate Order schema to ensure table structure
  4. Return configured storage interface

Connection pool settings:
  • Max idle connections: 10 (reduces resource usage)
  • Max open connections: 100 (handles concurrent access)
  • Connection lifetime: 1 hour (prevents stale connections)

Example usage:
  import "github.com/glebarez/sqlite"
  storage, err := storage.FromSQL(sqlite.Open("sqlite.db"), &gorm.Config{})
  if err != nil {
      log.Fatal(err)
  }

Parameters:
  • dialect: GORM dialector for specific database (SQLite, PostgreSQL, etc.)
  • opts: Additional GORM configuration options

Returns configured Storage interface or error if setup fails.
*/
func FromSQL(dialect gorm.Dialector, opts ...gorm.Option) (Storage, error) {
	// Open database connection with provided dialect
	db, err := gorm.Open(dialect, opts...)
	if err != nil {
		return nil, err
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Configure connection pool for optimal performance
	sqlDB.SetMaxIdleConns(10)           // Limit idle connections to save resources
	sqlDB.SetMaxOpenConns(100)          // Allow sufficient concurrent connections
	sqlDB.SetConnMaxLifetime(time.Hour) // Rotate connections to prevent staleness

	// Auto-migrate schema to ensure Order table exists with correct structure
	err = db.AutoMigrate(&model.Order{})
	if err != nil {
		return nil, err
	}

	return &SQL{
		db: db,
	}, nil
}

// CreateOrder persists a new trading order to the SQL database.
// The order ID is automatically generated and assigned by the database.
func (s *SQL) CreateOrder(order *model.Order) error {
	result := s.db.Create(order) // GORM automatically sets ID on successful insert
	return result.Error
}

/*
UpdateOrder modifies an existing order in the database.
It uses the order's ID to locate the existing record and updates all fields
with the provided order data.

Note: This implementation fetches the existing order first, then saves the
updated version to ensure proper handling of GORM hooks and validations.
*/
func (s *SQL) UpdateOrder(order *model.Order) error {
	// Find existing order by ID
	o := model.Order{ID: order.ID}
	s.db.First(&o)
	
	// Replace with updated order data
	o = *order
	
	// Save changes to database
	result := s.db.Save(&o)
	return result.Error
}

/*
Orders retrieves orders from the database with optional filtering.
The method supports client-side filtering using custom filter functions
for maximum flexibility in order queries.

Filtering Process:
  1. Fetch all orders from database
  2. Apply each provided filter function sequentially
  3. Return only orders that pass all filter criteria

Parameters:
  • filters: Variable number of OrderFilter functions for result filtering

Returns slice of orders matching all filter criteria or error if query fails.

Example usage:
  orders, err := storage.Orders(
      WithStatusIn(model.OrderStatusTypeFilled),
      func(order model.Order) bool { return order.Quantity > 1.0 },
  )
*/
func (s *SQL) Orders(filters ...OrderFilter) ([]*model.Order, error) {
	orders := make([]*model.Order, 0)

	// Fetch all orders from database
	result := s.db.Find(&orders)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return orders, result.Error
	}

	// Apply client-side filtering for maximum flexibility
	return lo.Filter(orders, func(order *model.Order, _ int) bool {
		// Order must pass all filters to be included
		for _, filter := range filters {
			if !filter(*order) {
				return false
			}
		}
		return true
	}), nil
}
