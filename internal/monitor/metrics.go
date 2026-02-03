package monitor

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	// VMsCreated tracks total VMs created
	VMsCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "firecracker_vms_created_total",
		Help: "Total number of VMs created",
	})

	// VMsRunning tracks currently running VMs
	VMsRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "firecracker_vms_running",
		Help: "Number of VMs currently running",
	})

	// VMOperationDuration tracks operation durations
	VMOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "firecracker_vm_operation_duration_seconds",
			Help:    "Duration of VM operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// GRPCRequestsTotal tracks gRPC requests
	GRPCRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "firecracker_grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method", "status"},
	)
)

func init() {
	// Register metrics
	prometheus.MustRegister(VMsCreated)
	prometheus.MustRegister(VMsRunning)
	prometheus.MustRegister(VMOperationDuration)
	prometheus.MustRegister(GRPCRequestsTotal)
}

// MetricsServer serves Prometheus metrics
type MetricsServer struct {
	port int
	log  *logrus.Logger
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(port int, log *logrus.Logger) *MetricsServer {
	return &MetricsServer{
		port: port,
		log:  log,
	}
}

// Start starts the metrics HTTP server
func (s *MetricsServer) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf(":%d", s.port)
	s.log.WithField("address", addr).Info("Metrics server starting")

	return http.ListenAndServe(addr, mux)
}
