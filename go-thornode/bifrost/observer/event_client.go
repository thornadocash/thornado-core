package observer

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
)

// EventClient manages a single subscription to events
type EventClient struct {
	logger     zerolog.Logger
	client     ebifrost.LocalhostBifrostClient
	eventTypes []string
	handlers   map[string]func(*ebifrost.EventNotification)

	// Subscription state
	mu         sync.RWMutex
	isActive   bool
	ctx        context.Context
	cancelFunc context.CancelFunc
	done       chan struct{}
	closeOnce  sync.Once      // used to safely close the done channel
	wg         sync.WaitGroup // used to track active subscription goroutines
}

// NewEventClient creates a new client with no active subscription
func NewEventClient(client ebifrost.LocalhostBifrostClient) *EventClient {
	return &EventClient{
		logger:   log.With().Str("module", "event_client").Logger(),
		client:   client,
		handlers: make(map[string]func(*ebifrost.EventNotification)),
		done:     make(chan struct{}),
	}
}

// RegisterHandler adds a handler for an event type (will cancel active subscription)
func (ec *EventClient) RegisterHandler(eventType string, handler func(*ebifrost.EventNotification)) {
	ec.mu.Lock()

	// Store state and prepare for possible cancellation
	var needsCancel bool
	var cancelFunc context.CancelFunc

	if ec.isActive {
		needsCancel = true
		cancelFunc = ec.cancelFunc
		ec.isActive = false
	}

	// Update handlers and event types while holding the lock
	ec.handlers[eventType] = handler

	// Update event types list
	found := false
	for _, t := range ec.eventTypes {
		if t == eventType {
			found = true
			break
		}
	}
	if !found {
		ec.eventTypes = append(ec.eventTypes, eventType)
	}

	ec.mu.Unlock()

	// Cancel and wait outside the lock to prevent deadlock
	if needsCancel {
		cancelFunc()

		// Wait with timeout to prevent test hangs
		waitChan := make(chan struct{})
		go func() {
			ec.wg.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			// Goroutine exited successfully
		case <-time.After(3 * time.Second):
			// Timeout - log a warning but continue
			ec.logger.Warn().Msg("Timed out waiting for subscription goroutine to exit")
		}
	}
}

// Start begins a new subscription, canceling any previous one
func (ec *EventClient) Start() {
	ec.mu.Lock()

	// Cancel existing subscription if active
	var needsCancel bool
	var cancelFunc context.CancelFunc

	if ec.isActive {
		needsCancel = true
		cancelFunc = ec.cancelFunc
	}

	// Release lock while waiting for cancellation to complete
	if needsCancel {
		ec.mu.Unlock()

		cancelFunc()

		// Wait with timeout to prevent test hangs
		waitChan := make(chan struct{})
		go func() {
			ec.wg.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			// Goroutine exited successfully
		case <-time.After(3 * time.Second):
			// Timeout - log a warning but continue
			ec.logger.Warn().Msg("Timed out waiting for subscription goroutine to exit")
		}

		// Re-acquire lock to set up new subscription
		ec.mu.Lock()
	}

	// Create new context for this subscription
	ec.ctx, ec.cancelFunc = context.WithCancel(context.Background())
	ec.isActive = true

	// Launch subscription in background
	ec.wg.Add(1) // Track the goroutine
	go ec.subscribeWithRetry(ec.ctx)

	ec.mu.Unlock()
}

// Stop ends the current subscription if active
func (ec *EventClient) Stop() {
	ec.mu.Lock()

	if ec.isActive {
		cancelFunc := ec.cancelFunc // Store locally to use after unlock
		ec.isActive = false
		ec.mu.Unlock()

		// Cancel context outside of the lock to prevent deadlock
		// if the subscription goroutine tries to acquire a lock
		cancelFunc()

		// Wait with timeout to prevent test hangs
		waitChan := make(chan struct{})
		go func() {
			ec.wg.Wait()
			close(waitChan)
		}()

		select {
		case <-waitChan:
			// Goroutine exited successfully
		case <-time.After(3 * time.Second):
			// Timeout - log a warning but continue
			ec.logger.Warn().Msg("Timed out waiting for subscription goroutine to exit")
		}

		return
	}

	ec.mu.Unlock()
}

// CleanShutdown stops all activity and closes channels
func (ec *EventClient) CleanShutdown() {
	ec.Stop()
	ec.closeOnce.Do(func() { close(ec.done) }) // Safely close the channel once
}

func (ec *EventClient) subscribeWithRetry(ctx context.Context) {
	defer ec.wg.Done() // Signal when this goroutine exits

	backoff := time.Second
	maxBackoff := 2 * time.Minute

	for {
		// Check context before attempting to subscribe
		select {
		case <-ctx.Done():
			return // This subscription has been canceled
		case <-ec.done:
			return // Client is shutting down
		default:
			// Continue with subscription attempt
		}

		err := ec.subscribe(ctx)

		// After subscribe returns (either normally or with error),
		// check context again before potential backoff
		select {
		case <-ctx.Done():
			return // Exit if context was canceled during subscribe
		case <-ec.done:
			return // Exit if client is shutting down
		default:
			// Handle error or retry logic
			if err != nil {
				ec.logger.Error().Err(err).Msg("Subscription error")

				// Wait with backoff before retry, but be interruptible
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-ec.done:
					timer.Stop()
					return
				case <-timer.C:
					// Increase backoff for next attempt, with a maximum
					backoff = time.Duration(math.Min(
						float64(backoff*2),
						float64(maxBackoff),
					))
				}
			} else {
				// This is an unexpected clean exit, retry with minimal backoff
				backoff = time.Second
			}
		}
	}
}

func (ec *EventClient) subscribe(ctx context.Context) error {
	// Check context before starting to make sure it's not already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue with subscription
	}

	// Get a copy of current event types and handlers
	ec.mu.RLock()
	eventTypes := make([]string, len(ec.eventTypes))
	copy(eventTypes, ec.eventTypes)
	ec.mu.RUnlock()

	// Start the subscription
	stream, err := ec.client.SubscribeToEvents(ctx, &ebifrost.SubscribeRequest{
		EventTypes: eventTypes,
	})
	if err != nil {
		return err
	}

	// Process events until error or cancellation
	for {
		// Use a cancelable stream.Recv() equivalent to avoid blocking indefinitely
		recvDone := make(chan struct{})
		var event *ebifrost.EventNotification
		var recvErr error

		go func() {
			event, recvErr = stream.Recv()
			close(recvDone)
		}()

		// Wait for either Recv() to complete or context to be canceled
		select {
		case <-ctx.Done():
			// Context was canceled while waiting for Recv()
			return ctx.Err()
		case <-recvDone:
			// Recv() completed (with or without error)
			if recvErr != nil {
				return recvErr
			}
		}

		// Get the appropriate handler
		ec.mu.RLock()
		handler, exists := ec.handlers[event.EventType]
		ec.mu.RUnlock()

		if exists {
			// Use a separate goroutine for handler to avoid blocking
			go func(evt *ebifrost.EventNotification, h func(*ebifrost.EventNotification)) {
				h(evt)
			}(event, handler)
		}
	}
}
