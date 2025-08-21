package model

import (
	"strconv"
	"strings"

	"golang.org/x/exp/constraints"
)

// Series represents a generic time series of ordered values with utility methods
// for accessing recent values, statistical operations, and data manipulation.
type Series[T constraints.Ordered] []T

// Values returns the underlying slice of values in the series.
func (s Series[T]) Values() []T {
	return s
}

// Length returns the total number of values in the series.
func (s Series[T]) Length() int {
	return len(s)
}

// Last returns a value from the series counting backwards from the most recent value.
// Position 0 returns the most recent value, position 1 returns the previous value, etc.
func (s Series[T]) Last(position int) T {
	return s[len(s)-1-position]
}

// LastValues returns the most recent values from the series up to the specified size.
// If the series has fewer values than requested, returns the entire series.
func (s Series[T]) LastValues(size int) []T {
	if l := len(s); l > size {
		return s[l-size:]
	}
	return s
}

// Crossover returns true if the last value of the series is greater than the last value of the reference series
func (s Series[T]) Crossover(ref Series[T]) bool {
	return s.Last(0) > ref.Last(0) && s.Last(1) <= ref.Last(1)
}

// Crossunder returns true if the last value of the series is less than the last value of the reference series
func (s Series[T]) Crossunder(ref Series[T]) bool {
	return s.Last(0) <= ref.Last(0) && s.Last(1) > ref.Last(1)
}

// Cross returns true if the last value of the series is greater than the last value of the
// reference series or less than the last value of the reference series
func (s Series[T]) Cross(ref Series[T]) bool {
	return s.Crossover(ref) || s.Crossunder(ref)
}

// NumDecPlaces returns the number of decimal places of a float64
func NumDecPlaces(v float64) int64 {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	i := strings.IndexByte(s, '.')
	if i > -1 {
		return int64(len(s) - i - 1)
	}
	return 0
}
