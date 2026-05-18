package observer_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gitlab.com/thorchain/thornode/v3/bifrost/observer"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MockBifrostClient implements ebifrost.LocalhostBifrostClient for testing
type MockBifrostClient struct {
	mock.Mock
}

// MockBifrostStream implements the ebifrost.Localhost_SubscribeToEventsClient interface
type MockBifrostStream struct {
	mock.Mock
	events []*ebifrost.EventNotification
	index  int
	ctx    context.Context
}

func (m *MockBifrostStream) Recv() (*ebifrost.EventNotification, error) {
	// Check if this call is expected by the mock framework
	if len(m.Mock.ExpectedCalls) > 0 {
		args := m.Called()
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		// nolint:forcetypeassert
		return args.Get(0).(*ebifrost.EventNotification), args.Error(1)
	}

	// Fall back to event-based implementation if no mock expectations
	if m.events != nil && m.index < len(m.events) {
		event := m.events[m.index]
		m.index++
		return event, nil
	}

	// Block until context is done if no events
	if m.ctx != nil {
		<-m.ctx.Done()
	}
	return nil, errors.New("stream closed")
}

func (m *MockBifrostStream) Header() (metadata.MD, error) {
	args := m.Called()
	// nolint:forcetypeassert
	return args.Get(0).(metadata.MD), args.Error(1)
}

func (m *MockBifrostStream) Trailer() metadata.MD {
	args := m.Called()
	// nolint:forcetypeassert
	return args.Get(0).(metadata.MD)
}

func (m *MockBifrostStream) CloseSend() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockBifrostStream) Context() context.Context {
	return m.ctx
}

func (m *MockBifrostStream) SendMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockBifrostStream) RecvMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockBifrostClient) SendQuorumTx(ctx context.Context, in *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(*ebifrost.SendQuorumTxResult), args.Error(1)
}

func (m *MockBifrostClient) SendQuorumNetworkFee(ctx context.Context, in *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(*ebifrost.SendQuorumNetworkFeeResult), args.Error(1)
}

func (m *MockBifrostClient) SendQuorumSolvency(ctx context.Context, in *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(*ebifrost.SendQuorumSolvencyResult), args.Error(1)
}

func (m *MockBifrostClient) SendQuorumErrataTx(ctx context.Context, in *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(*ebifrost.SendQuorumErrataTxResult), args.Error(1)
}

func (m *MockBifrostClient) SendQuorumPriceFeedBatch(ctx context.Context, in *common.QuorumPriceFeedBatch, opts ...grpc.CallOption) (*ebifrost.SendQuorumPriceFeedBatchResult, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(*ebifrost.SendQuorumPriceFeedBatchResult), args.Error(1)
}

func (m *MockBifrostClient) SubscribeToEvents(ctx context.Context, in *ebifrost.SubscribeRequest, opts ...grpc.CallOption) (ebifrost.LocalhostBifrost_SubscribeToEventsClient, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// nolint:forcetypeassert
	return args.Get(0).(ebifrost.LocalhostBifrost_SubscribeToEventsClient), args.Error(1)
}

func TestNewEventClient(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	assert.NotNil(t, client, "EventClient should not be nil")
}

func TestRegisterHandler(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Create a test handler
	testHandler := func(event *ebifrost.EventNotification) {}

	// Register the handler
	client.RegisterHandler("test_event", testHandler)

	// We can't directly test if the handler was registered since handlers map is private
	// But we can test if it's correctly called when an event arrives (in another test)
}

func TestStartAndStop(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Register a dummy handler to ensure we have at least one event type
	client.RegisterHandler("test_event", func(event *ebifrost.EventNotification) {})

	// Setup a mock stream
	ctx, cancel := context.WithCancel(context.Background())
	mockStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{},
		ctx:    ctx,
	}

	// Make the mock client return our mock stream using AnyOfType for proper type checking
	// The function should accept empty slices as well
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.MatchedBy(func(req *ebifrost.SubscribeRequest) bool {
			// Accept any request, including empty event types
			return true
		}),
		mock.Anything).Return(mockStream, nil)

	// Start client
	client.Start()

	// Allow some time for goroutines to start
	time.Sleep(50 * time.Millisecond)

	// Stop client
	client.Stop()
	cancel() // Cancel the context to unblock the stream

	// Allow some time for goroutines to stop
	time.Sleep(50 * time.Millisecond)

	mockClient.AssertExpectations(t)
}

func TestEventHandling(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Create a channel to signal when the handler is called
	handlerCalled := make(chan struct{})

	// Register a handler that will signal on the channel
	client.RegisterHandler("transaction", func(event *ebifrost.EventNotification) {
		assert.Equal(t, "transaction", event.EventType)
		assert.Equal(t, "test_data", string(event.Payload))
		close(handlerCalled)
	})

	// Setup a mock stream with a test event
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{
			{
				EventType: "transaction",
				Payload:   []byte("test_data"),
			},
		},
		ctx: ctx,
	}

	// Make the mock client return our mock stream
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.MatchedBy(func(req *ebifrost.SubscribeRequest) bool {
			// Verify transaction is in the event types
			for _, t := range req.EventTypes {
				if t == "transaction" {
					return true
				}
			}
			return false
		}),
		mock.Anything).Return(mockStream, nil)

	// Start client
	client.Start()

	// Wait for handler to be called (with timeout)
	select {
	case <-handlerCalled:
		// Handler was called successfully
	case <-time.After(time.Second):
		t.Fatal("Handler was not called within timeout")
	}

	// Stop client
	client.Stop()

	mockClient.AssertExpectations(t)
}

func TestSubscriptionRetry(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// First call fails
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil, errors.New("connection error")).Once()

	// Second call succeeds
	ctx, cancel := context.WithCancel(context.Background())
	mockStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{},
		ctx:    ctx,
	}
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(mockStream, nil).Once()

	// Start client
	client.Start()

	// Allow time for retry to happen (greater than 1 second backoff)
	time.Sleep(1500 * time.Millisecond)

	// Stop client
	client.Stop()
	cancel() // Cancel context to unblock stream

	// Allow time for goroutines to stop
	time.Sleep(50 * time.Millisecond)

	mockClient.AssertNumberOfCalls(t, "SubscribeToEvents", 2)
}

func TestMultipleHandlers(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Create channels to signal when handlers are called
	txHandlerCalled := make(chan struct{})
	blockHandlerCalled := make(chan struct{})

	// Register handlers
	client.RegisterHandler("transaction", func(event *ebifrost.EventNotification) {
		assert.Equal(t, "transaction", event.EventType)
		close(txHandlerCalled)
	})

	client.RegisterHandler("block", func(event *ebifrost.EventNotification) {
		assert.Equal(t, "block", event.EventType)
		close(blockHandlerCalled)
	})

	// Setup a mock stream with test events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{
			{
				EventType: "transaction",
				Payload:   []byte("tx_data"),
			},
			{
				EventType: "block",
				Payload:   []byte("block_data"),
			},
		},
		ctx: ctx,
	}

	// Make the mock client return our mock stream
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.MatchedBy(func(req *ebifrost.SubscribeRequest) bool {
			// Verify both event types are present
			txFound, blockFound := false, false
			for _, t := range req.EventTypes {
				if t == "transaction" {
					txFound = true
				}
				if t == "block" {
					blockFound = true
				}
			}
			return txFound && blockFound
		}),
		mock.Anything).Return(mockStream, nil)

	// Start client
	client.Start()

	// Wait for both handlers to be called
	select {
	case <-txHandlerCalled:
		// Transaction handler called
	case <-time.After(time.Second):
		t.Fatal("Transaction handler was not called within timeout")
	}

	select {
	case <-blockHandlerCalled:
		// Block handler called
	case <-time.After(time.Second):
		t.Fatal("Block handler was not called within timeout")
	}

	// Stop client
	client.Stop()

	mockClient.AssertExpectations(t)
}

func TestStreamError(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Setup a mock stream that will return an error
	mockStream := new(MockBifrostStream)
	mockStream.On("Recv").Return(nil, errors.New("stream error"))

	// Make the mock client return our mock stream
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.MatchedBy(func(req *ebifrost.SubscribeRequest) bool {
			return true // Accept any request
		}),
		mock.Anything).Return(mockStream, nil).Once()

	// Second call gets a working stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workingStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{},
		ctx:    ctx,
	}

	// The second mock needs to have proper context and events initialized
	workingStream.On("Recv").Return(
		&ebifrost.EventNotification{EventType: "test", Payload: []byte("data")},
		nil,
	).Once()

	// After first recv, return error to end test
	workingStream.On("Recv").Return(
		nil,
		errors.New("end test"),
	).Once()

	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(workingStream, nil).Once()

	// Start client
	client.Start()

	// Allow time for retry to happen
	time.Sleep(1500 * time.Millisecond)

	// Stop client
	client.Stop()

	// Allow time for goroutines to stop
	time.Sleep(50 * time.Millisecond)

	mockClient.AssertNumberOfCalls(t, "SubscribeToEvents", 2)
	mockStream.AssertExpectations(t)
}

func TestConcurrentHandlerRegistration(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Register handlers concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			eventType := "event_" + string(rune('A'+i))
			client.RegisterHandler(eventType, func(event *ebifrost.EventNotification) {})
		}(i)
	}

	wg.Wait()

	// No direct way to check results, but if no race detector warnings, it's good
}

func TestHandlersNotDuplicatedAfterReconnection(t *testing.T) {
	mockClient := new(MockBifrostClient)
	client := observer.NewEventClient(mockClient)

	// Channel to signal handler execution and collect invocation count
	handlerCalled := make(chan struct{}, 5) // Buffer to catch potential duplicates

	// Register a handler
	client.RegisterHandler("transaction", func(event *ebifrost.EventNotification) {
		assert.Equal(t, "transaction", event.EventType)
		assert.Equal(t, "test_data", string(event.Payload))
		handlerCalled <- struct{}{}
	})

	// First subscription fails
	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.Anything,
		mock.Anything).Return(nil, errors.New("connection error")).Once()

	// Second subscription succeeds
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock stream with ONE event that will be delivered
	mockStream := &MockBifrostStream{
		events: []*ebifrost.EventNotification{
			{
				EventType: "transaction",
				Payload:   []byte("test_data"),
			},
		},
		ctx: ctx,
	}

	mockClient.On("SubscribeToEvents",
		mock.Anything,
		mock.MatchedBy(func(req *ebifrost.SubscribeRequest) bool {
			// Verify transaction is in the event types EXACTLY ONCE
			count := 0
			for _, t := range req.EventTypes {
				if t == "transaction" {
					count++
				}
			}
			return count == 1
		}),
		mock.Anything).Return(mockStream, nil).Once()

	// Start client
	client.Start()

	// Allow time for retry to happen (greater than 1 second backoff)
	time.Sleep(1500 * time.Millisecond)

	// Wait for handler to be called with timeout
	timer := time.NewTimer(time.Second)
	select {
	case <-handlerCalled:
		// Handler was called at least once
	case <-timer.C:
		t.Fatal("Handler was not called within timeout")
	}

	// Check if there are any more calls after a brief wait (there shouldn't be)
	timer.Reset(300 * time.Millisecond)
	var extraCalls int

waitLoop:
	for {
		select {
		case <-handlerCalled:
			extraCalls++
		case <-timer.C:
			break waitLoop
		}
	}

	// Verify no extra calls occurred
	assert.Equal(t, 0, extraCalls, "Handler should not be called more than once for a single event")

	// Stop client
	client.Stop()

	mockClient.AssertExpectations(t)
}
