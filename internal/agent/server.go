package agent

import (
	"context"
	"sync"
	"time"

	pb "github.com/spluca/firecracker-agent/api/proto/firecracker/v1"
	"github.com/spluca/firecracker-agent/internal/firecracker"
	"github.com/spluca/firecracker-agent/pkg/config"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Server implements the FirecrackerAgent gRPC service
type Server struct {
	pb.UnimplementedFirecrackerAgentServer

	cfg         *config.Config
	log         *logrus.Logger
	fcManager   *firecracker.Manager
	startTime   time.Time
	eventStream *EventStream
	mu          sync.RWMutex
}

// NewServer creates a new agent server
func NewServer(cfg *config.Config, log *logrus.Logger, startTime time.Time) (*Server, error) {
	// Create Firecracker manager
	fcManager, err := firecracker.NewManager(cfg, log)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:         cfg,
		log:         log,
		fcManager:   fcManager,
		startTime:   startTime,
		eventStream: NewEventStream(),
	}, nil
}

// Register registers the gRPC service
func (s *Server) Register(grpcServer *grpc.Server) {
	pb.RegisterFirecrackerAgentServer(grpcServer, s)
	s.log.Info("gRPC service registered")
}

// LoggingInterceptor logs all gRPC requests
func LoggingInterceptor(log *logrus.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)

		fields := logrus.Fields{
			"method":   info.FullMethod,
			"duration": duration.String(),
		}

		if err != nil {
			fields["error"] = err.Error()
			log.WithFields(fields).Error("gRPC call failed")
		} else {
			log.WithFields(fields).Info("gRPC call completed")
		}

		return resp, err
	}
}

// EventStream manages VM event subscribers
type EventStream struct {
	subscribers map[string]chan *pb.VMEvent
	mu          sync.RWMutex
}

// NewEventStream creates a new event stream
func NewEventStream() *EventStream {
	return &EventStream{
		subscribers: make(map[string]chan *pb.VMEvent),
	}
}

// Subscribe adds a new subscriber
func (es *EventStream) Subscribe(id string) chan *pb.VMEvent {
	es.mu.Lock()
	defer es.mu.Unlock()

	ch := make(chan *pb.VMEvent, 100)
	es.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber
func (es *EventStream) Unsubscribe(id string) {
	es.mu.Lock()
	defer es.mu.Unlock()

	if ch, exists := es.subscribers[id]; exists {
		close(ch)
		delete(es.subscribers, id)
	}
}

// Broadcast sends an event to all subscribers
func (es *EventStream) Broadcast(event *pb.VMEvent) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	for _, ch := range es.subscribers {
		select {
		case ch <- event:
		default:
			// Skip if channel is full
		}
	}
}
