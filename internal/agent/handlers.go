package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	pb "github.com/apardo/firecracker-agent/api/proto/firecracker/v1"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateVM creates a new Firecracker VM
func (s *Server) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.CreateVMResponse, error) {
	s.log.WithField("vm_id", req.VmId).Info("Creating VM")

	// Validate request
	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.VcpuCount < 1 || req.VcpuCount > 32 {
		return nil, status.Error(codes.InvalidArgument, "vcpu_count must be between 1 and 32")
	}
	if req.MemoryMb < 128 {
		return nil, status.Error(codes.InvalidArgument, "memory_mb must be at least 128")
	}

	// Create VM using Firecracker manager
	vmInfo, err := s.fcManager.CreateVM(ctx, req)
	if err != nil {
		s.log.WithError(err).Error("Failed to create VM")

		// Broadcast error event
		s.eventStream.Broadcast(&pb.VMEvent{
			VmId:      req.VmId,
			State:     pb.VMState_VM_STATE_ERROR,
			Message:   err.Error(),
			Timestamp: time.Now().Unix(),
			Type:      pb.EventType_EVENT_TYPE_ERROR,
		})

		return &pb.CreateVMResponse{
			VmId:         req.VmId,
			State:        pb.VMState_VM_STATE_ERROR,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Broadcast created event
	s.eventStream.Broadcast(&pb.VMEvent{
		VmId:      req.VmId,
		State:     pb.VMState_VM_STATE_RUNNING,
		Message:   "VM created successfully",
		Timestamp: time.Now().Unix(),
		Type:      pb.EventType_EVENT_TYPE_CREATED,
	})

	return &pb.CreateVMResponse{
		VmId:       vmInfo.VmId,
		State:      vmInfo.State,
		SocketPath: vmInfo.SocketPath,
		CreatedAt:  vmInfo.CreatedAt,
	}, nil
}

// StartVM starts an existing VM
func (s *Server) StartVM(ctx context.Context, req *pb.StartVMRequest) (*pb.StartVMResponse, error) {
	s.log.WithField("vm_id", req.VmId).Info("Starting VM")

	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	err := s.fcManager.StartVM(ctx, req.VmId)
	if err != nil {
		s.log.WithError(err).Error("Failed to start VM")

		s.eventStream.Broadcast(&pb.VMEvent{
			VmId:      req.VmId,
			State:     pb.VMState_VM_STATE_ERROR,
			Message:   err.Error(),
			Timestamp: time.Now().Unix(),
			Type:      pb.EventType_EVENT_TYPE_ERROR,
		})

		return &pb.StartVMResponse{
			VmId:         req.VmId,
			State:        pb.VMState_VM_STATE_ERROR,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Broadcast started event
	s.eventStream.Broadcast(&pb.VMEvent{
		VmId:      req.VmId,
		State:     pb.VMState_VM_STATE_RUNNING,
		Message:   "VM started",
		Timestamp: time.Now().Unix(),
		Type:      pb.EventType_EVENT_TYPE_STARTED,
	})

	return &pb.StartVMResponse{
		VmId:  req.VmId,
		State: pb.VMState_VM_STATE_RUNNING,
	}, nil
}

// StopVM stops a running VM
func (s *Server) StopVM(ctx context.Context, req *pb.StopVMRequest) (*pb.StopVMResponse, error) {
	s.log.WithField("vm_id", req.VmId).Info("Stopping VM")

	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	err := s.fcManager.StopVM(ctx, req.VmId, req.Force)
	if err != nil {
		s.log.WithError(err).Error("Failed to stop VM")

		s.eventStream.Broadcast(&pb.VMEvent{
			VmId:      req.VmId,
			State:     pb.VMState_VM_STATE_ERROR,
			Message:   err.Error(),
			Timestamp: time.Now().Unix(),
			Type:      pb.EventType_EVENT_TYPE_ERROR,
		})

		return &pb.StopVMResponse{
			VmId:         req.VmId,
			State:        pb.VMState_VM_STATE_ERROR,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Broadcast stopped event
	s.eventStream.Broadcast(&pb.VMEvent{
		VmId:      req.VmId,
		State:     pb.VMState_VM_STATE_STOPPED,
		Message:   "VM stopped",
		Timestamp: time.Now().Unix(),
		Type:      pb.EventType_EVENT_TYPE_STOPPED,
	})

	return &pb.StopVMResponse{
		VmId:  req.VmId,
		State: pb.VMState_VM_STATE_STOPPED,
	}, nil
}

// DeleteVM deletes a VM
func (s *Server) DeleteVM(ctx context.Context, req *pb.DeleteVMRequest) (*pb.DeleteVMResponse, error) {
	s.log.WithField("vm_id", req.VmId).Info("Deleting VM")

	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	err := s.fcManager.DeleteVM(ctx, req.VmId)
	if err != nil {
		s.log.WithError(err).Error("Failed to delete VM")

		return &pb.DeleteVMResponse{
			VmId:         req.VmId,
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Broadcast deleted event
	s.eventStream.Broadcast(&pb.VMEvent{
		VmId:      req.VmId,
		State:     pb.VMState_VM_STATE_DELETING,
		Message:   "VM deleted",
		Timestamp: time.Now().Unix(),
		Type:      pb.EventType_EVENT_TYPE_DELETED,
	})

	return &pb.DeleteVMResponse{
		VmId:    req.VmId,
		Success: true,
	}, nil
}

// GetVM retrieves VM information
func (s *Server) GetVM(ctx context.Context, req *pb.GetVMRequest) (*pb.GetVMResponse, error) {
	s.log.WithField("vm_id", req.VmId).Debug("Getting VM info")

	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	vm, err := s.fcManager.GetVM(req.VmId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}

	return &pb.GetVMResponse{
		Vm: vm,
	}, nil
}

// ListVMs lists all VMs
func (s *Server) ListVMs(ctx context.Context, req *pb.ListVMsRequest) (*pb.ListVMsResponse, error) {
	s.log.Debug("Listing VMs")

	vms := s.fcManager.ListVMs()

	return &pb.ListVMsResponse{
		Vms:        vms,
		TotalCount: int32(len(vms)),
	}, nil
}

// WatchVMEvents streams VM events
func (s *Server) WatchVMEvents(req *pb.WatchVMEventsRequest, stream pb.FirecrackerAgent_WatchVMEventsServer) error {
	s.log.WithField("vm_id", req.VmId).Info("Client watching VM events")

	// Subscribe to events
	subscriberID := fmt.Sprintf("watch-%d", time.Now().UnixNano())
	eventChan := s.eventStream.Subscribe(subscriberID)
	defer s.eventStream.Unsubscribe(subscriberID)

	// Stream events
	for {
		select {
		case event := <-eventChan:
			// Filter by VM ID if specified
			if req.VmId != "" && event.VmId != req.VmId {
				continue
			}

			if err := stream.Send(event); err != nil {
				s.log.WithError(err).Error("Failed to send event")
				return err
			}

		case <-stream.Context().Done():
			s.log.Info("Client disconnected from event stream")
			return nil
		}
	}
}

// GetHostInfo returns host system information
func (s *Server) GetHostInfo(ctx context.Context, req *pb.GetHostInfoRequest) (*pb.GetHostInfoResponse, error) {
	s.log.Debug("Getting host info")

	hostname, _ := os.Hostname()

	// Get CPU info
	cpuCount := runtime.NumCPU()
	cpuPercent, _ := cpu.Percent(time.Second, false)

	// Get memory info
	vmem, _ := mem.VirtualMemory()

	totalMemMB := int64(vmem.Total / 1024 / 1024)
	availMemMB := int64(vmem.Available / 1024 / 1024)

	// Get running VMs count
	vms := s.fcManager.ListVMs()

	return &pb.GetHostInfoResponse{
		Hostname:          hostname,
		TotalCpus:         int32(cpuCount),
		TotalMemoryMb:     totalMemMB,
		AvailableMemoryMb: availMemMB,
		RunningVms:        int32(len(vms)),
		CpuUsage:          float32(cpuPercent[0]),
		Version:           "0.1.0",
	}, nil
}

// HealthCheck returns agent health status
func (s *Server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	uptime := int64(time.Since(s.startTime).Seconds())

	return &pb.HealthCheckResponse{
		Healthy:       true,
		Version:       "0.1.0",
		UptimeSeconds: uptime,
	}, nil
}
