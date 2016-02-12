package simulator

import (
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

// This struct will hold the present state of all the nodes, jobs and allocations in a single
// iteration (each Job Evaluation), which we'll use to analyse the Schedulers.
type SimSnapshot struct {
	// The Evaluation associated to the Job processed during a given iteration.
	Eval *structs.Evaluation
	// The Plans that have been created.
	Plans []*structs.Plan
	// The internal State of the cluster after a given iteration.
	State *state.StateStore
	// The list of Node IDs inside of the cluster at a given iteration.
	NodeIDs []string
	// The UNIX timestamp at the moment of processing the Job. This together with the allocation
	// time is helpful for determining the real order of Job processings given that each processing
	// is done in a separate goroutine.
	Time int64
}

// This struct will hold the metrics of a single Node, which will be later serialized
// and outputted by the program. These are the initial Node values without usage.
type SimNode struct {
	ID         string            `json:"id"`
	Datacenter string            `json:"datacenter"`
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
	Resources  *SimResources     `json:"resources"`
	Reserved   *SimResources     `json:"reserved"`
}

// This struct will hold the updates of usage of a single Node. These values change
// after Allocations, so associate these to Job evaluations.
type NodeUsageChange struct {
	ID        string        `json:"id"`
	Available *SimResources `json:"available"`
}

// SimResources is used to briefly define the resources of a Node, whether total,
// reserved, or the ones left available when new Allocations happen.
type SimResources struct {
	CPU      int `hcl:"cpu"       json:"cpu"`
	MemoryMB int `hcl:"memory_mb" json:"memory_mb"`
	DiskMB   int `hcl:"disk_mb"   json:"disk_mb"`
	IOPS     int `hcl:"iops"      json:"iops"`
}

// A certain relevant metrics (as of now) for a given Job.
type JobMetrics struct {
	ID             string   `json:"job_id"`
	Datacenters    []string `json:"datacenters"`
	Region         string   `json:"region"`
	Name           string   `json:"job_name"`
	Type           string   `json:"job_type"`
	TaskGroups     []string `json:"task_groups"`
	StartTimestamp int64    `json:"start_time_stamp"`
	FinalTimestamp int64    `json:"final_time_stamp"`
}

// A certain relevant metrics (as of now) for a given Allocation.
type AllocMetrics struct {
	NodeID             string        `json:"node_id"`
	JobID              string        `json:"job_id"`
	TaskGroup          string        `json:"task_group"`
	AllocationTime     int64         `json:"allocation_time"`
	DesiredDescription string        `json:"desired_description"`
	DesiredStatus      string        `json:"desired_status"`
	Resources          *SimResources `json:"resources"`
}

// Get the only most relevant Node information from a Node struct.
// Serializing the whole Node could be expensive, for now lets just
// focus on what we deem relevant for evaluating placements and the
// bin packing of the simulator.
func parseNode(node *structs.Node) *SimNode {
	return &SimNode{
		ID:         node.ID,
		Name:       node.Name,
		Datacenter: node.Datacenter,
		Attributes: node.Attributes,
		Resources: &SimResources{
			CPU:      node.Resources.CPU,
			MemoryMB: node.Resources.MemoryMB,
			DiskMB:   node.Resources.DiskMB,
			IOPS:     node.Resources.IOPS,
		},
		Reserved: &SimResources{
			CPU:      node.Reserved.CPU,
			MemoryMB: node.Reserved.MemoryMB,
			DiskMB:   node.Reserved.DiskMB,
			IOPS:     node.Reserved.IOPS,
		},
	}
}

// For a given Node, lets calculate the resources it has left available, after
// substracting from its original resources the consumption done by Allocations
// associated to such Node.
func getAvailable(node *structs.Node, state *state.StateStore) *SimResources {

	// The original resources available for Allocations are the base resources
	// of the Node, minus the resources that are reserved.
	available := &SimResources{
		CPU:      node.Resources.CPU - node.Reserved.CPU,
		MemoryMB: node.Resources.MemoryMB - node.Reserved.MemoryMB,
		DiskMB:   node.Resources.DiskMB - node.Reserved.DiskMB,
		IOPS:     node.Resources.IOPS - node.Reserved.IOPS,
	}

	// Get the allocations associated with the current node
	allocations, _ := state.AllocsByNode(node.ID)
	for _, allocation := range allocations {
		// And then substract the resources consumed by each Allocation, to
		// reflect them on the current node resource consumption.
		resources := allocation.Resources
		available.CPU = available.CPU - resources.CPU
		available.MemoryMB = available.MemoryMB - resources.MemoryMB
		available.DiskMB = available.DiskMB - resources.DiskMB
		available.IOPS = available.IOPS - resources.IOPS
	}
	return available
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
		Resources: &SimResources{
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

// The metrics of the cluster for a given iteration ~ Job processing.
type JobEvaluationMetrics struct {
	JobMetrics              *JobMetrics        `json:"evaluated_job"`
	SuccessfulAllocsMetrics []*AllocMetrics    `json:"successful_allocs_metrics"`
	FailedAllocsMetrics     []*AllocMetrics    `json:"failed_allocs_metrics"`
	NodeUsageChanges        []*NodeUsageChange `json:"node_usage_changes"`
}

// The output of the simulator after processing all Jobs.
type SimOutput struct {
	Nodes          []*SimNode              `json:"nodes"`
	JobEvaluations []*JobEvaluationMetrics `json:"job_evaluations"`
}
