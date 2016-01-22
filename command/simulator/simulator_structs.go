package simulator

import (
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

// This struct will hold the present state of all the nodes, jobs and allocations in a single
// iteration (each Job Evaluation), which we'll use to analyse the Schedulers.
type SimulatorSnapshot struct {
	// The Evaluation associated to the Job processed during a given iteration.
	Eval *structs.Evaluation
	// The Plans that have been created.
	Plans []*structs.Plan
	// The internal State of the cluster after a given iteration.
	State *state.StateStore
	// The list of Node IDs inside of the cluster at a given iteration.
	NodeIDs []string
}

// This struct will hold the metrics of a single iteration, which will be later serialized
// and outputted by the program.
type NodeMetrics struct {
	// The ID of this Node
	ID string `json:"id"`
	// Datacenter for this Node
	Datacenter string `json:"datacenter"`
	// Node Name
	Name string `json:"name"`
	// Node Attributes
	Attributes map[string]string `json:"attributes"`

	// The resources that the Node has by default.
	Resources *SimulatorResources `json:"resources"`
	// The currently available resources in a Node (this if after substracting the
	// resources consumed by current Allocations on the given Node).
	Available *SimulatorResources `json:"available"`
	// The reserved resources of the Node.
	Reserved *SimulatorResources `json:"reserved"`
}

// SimulatorResources is used to briefly define the resources available on a client.
type SimulatorResources struct {
	CPU      int `hcl:"cpu"       json:"cpu"`
	MemoryMB int `hcl:"memory_mb" json:"memory_mb"`
	DiskMB   int `hcl:"disk_mb"   json:"disk_mb"`
	IOPS     int `hcl:"iops"      json:"iops"`
}

// A certain relevant metrics (for now) for a given Job.
type JobMetrics struct {
	ID          string   `json:"job_id"`
	Datacenters []string `json:"datacenters"`
	Region      string   `json:"region"`
	Name        string   `json:"job_name"`
	Type        string   `json:"job_type"`
	TaskGroups  []string `json:"task_groups"`
}

// A certain relevant metrics (for now) for a given Allocation.
type AllocMetrics struct {
	NodeID             string              `json:"node_id"`
	JobID              string              `json:"job_id"`
	TaskGroup          string              `json:"task_group"`
	AllocationTime     int64               `json:"allocation_time"`
	DesiredDescription string              `json:"desired_description"`
	DesiredStatus      string              `json:"desired_status"`
	Resources          *SimulatorResources `json:"resources"`
}

// Get the most relevant Node information from a Node struct.
// Relevant as in helpful for evaluating placement and bin packing.
func ParseNodeMetrics(node *structs.Node) *NodeMetrics {
	return &NodeMetrics{
		ID:         node.ID,
		Name:       node.Name,
		Datacenter: node.Datacenter,
		Attributes: node.Attributes,
		Resources: &SimulatorResources{
			CPU:      node.Resources.CPU,
			MemoryMB: node.Resources.MemoryMB,
			DiskMB:   node.Resources.DiskMB,
			IOPS:     node.Resources.IOPS,
		},
		Available: &SimulatorResources{
			CPU:      node.Resources.CPU,
			MemoryMB: node.Resources.MemoryMB,
			DiskMB:   node.Resources.DiskMB,
			IOPS:     node.Resources.IOPS,
		},
		Reserved: &SimulatorResources{
			CPU:      node.Reserved.CPU,
			MemoryMB: node.Reserved.MemoryMB,
			DiskMB:   node.Reserved.DiskMB,
			IOPS:     node.Reserved.IOPS,
		},
	}
}

// Get the most relevant Job information from a Job struct.
// Relevant as in helpful for evaluating placement and bin packing.
func ParseJobMetrics(job *structs.Job) *JobMetrics {
	var taskGroups []string
	for _, taskGroup := range job.TaskGroups {
		taskGroups = append(taskGroups, taskGroup.Name)
	}

	return &JobMetrics{
		ID:          job.ID,
		Region:      job.Region,
		Datacenters: job.Datacenters,
		Name:        job.Name,
		Type:        job.Type,
		TaskGroups:  taskGroups,
	}
}

// Get the most relevant Allocation information from an Allocation struct.
// Relevant as in helpful for evaluating placement and bin packing.
func ParseAllocMetrics(alloc *structs.Allocation) *AllocMetrics {
	return &AllocMetrics{
		NodeID: alloc.NodeID,
		JobID:  alloc.JobID,
		Resources: &SimulatorResources{
			CPU:      alloc.Resources.CPU,
			MemoryMB: alloc.Resources.MemoryMB,
			DiskMB:   alloc.Resources.DiskMB,
			IOPS:     alloc.Resources.IOPS,
		},
		TaskGroup:          alloc.TaskGroup,
		AllocationTime:     int64(alloc.Metrics.AllocationTime),
		DesiredDescription: alloc.DesiredDescription,
		DesiredStatus:      alloc.DesiredStatus,
	}
}

// The metrics of the cluster for a given iteration ~ job processing.
type IterationMetrics struct {
	JobMetrics              *JobMetrics     `json:"evaluated_job"`
	NodesMetrics            []*NodeMetrics  `json:"nodes_metrics"`
	SuccessfulAllocsMetrics []*AllocMetrics `json:"successful_allocs_metrics"`
	FailedAllocsMetrics     []*AllocMetrics `json:"failed_allocs_metrics"`
}
