package tools

/*
TrailingStop implements a dynamic stop-loss mechanism that follows price movements
in the favorable direction while maintaining a fixed distance. When price moves
against the position, the stop level remains fixed until the stop price is hit.

This tool is essential for:
  • Protecting profits in trending markets
  • Automatic position management without constant monitoring
  • Reducing emotional trading decisions
  • Implementing systematic risk management

Key Features:
  • Dynamic stop adjustment as price moves favorably
  • Fixed stop protection when price reverses
  • Configurable trailing distance
  • Simple activation/deactivation controls
*/
type TrailingStop struct {
	current float64  // Current market price being tracked
	stop    float64  // Current stop-loss price level
	active  bool     // Whether trailing stop is currently active
}

// NewTrailingStop creates a new trailing stop instance in inactive state.
func NewTrailingStop() *TrailingStop {
	return &TrailingStop{}
}

/*
Start activates the trailing stop with initial price and stop level.
The trailing distance is calculated as the difference between current and stop prices.

Parameters:
  • current: Current market price to start tracking
  • stop: Initial stop-loss price level

Example for long position:
  start(100.0, 95.0) // 5-point trailing distance
*/
func (t *TrailingStop) Start(current, stop float64) {
	t.stop = stop
	t.current = current
	t.active = true
}

// Stop deactivates the trailing stop mechanism.
func (t *TrailingStop) Stop() {
	t.active = false
}

// Active returns whether the trailing stop is currently enabled.
func (t TrailingStop) Active() bool {
	return t.active
}

/*
Update processes a new price tick and adjusts the trailing stop if needed.
The method maintains the trailing distance when price moves favorably
and triggers when price hits the stop level.

Price Movement Logic:
  • If price improves: adjust stop to maintain trailing distance
  • If price worsens: keep stop at current level
  • If price hits stop: return true to signal exit

Parameters:
  • current: New market price to process

Returns:
  • true: Stop level hit, position should be closed
  • false: Continue tracking, no action needed

Example for long position:
  Price rises 100→105: stop moves 95→100 (maintains 5-point distance)
  Price falls 105→97: stop stays at 100
  Price falls 97→99: returns false (above stop)
  Price falls 99→100: returns true (stop hit)
*/
func (t *TrailingStop) Update(current float64) bool {
	if !t.active {
		return false
	}

	// If price moved favorably, adjust trailing stop
	if current > t.current {
		// Move stop by the same amount price improved
		t.stop = t.stop + (current - t.current)
		t.current = current
		return false
	}

	// Update current price tracking
	t.current = current
	
	// Check if stop level was hit
	return current <= t.stop
}
