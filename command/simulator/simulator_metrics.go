package simulator

import "github.com/hashicorp/nomad/nomad/structs"

// Get the Metrics of cluster allocations (successful or failed) after a given iteration,
// which is the processing of one Job. Also retrieve the current resource consumption of
// all the Nodes present in the cluster.
func getMetrics(simulatorSnapshot *SimulatorSnapshot) *IterationMetrics {

	state := simulatorSnapshot.State
	nodeIDs := simulatorSnapshot.NodeIDs
	eval := simulatorSnapshot.Eval

	// A list of Node Metrics.
	var nodesMetrics []*NodeMetrics

	// A list with the failed Allocations' Metrics.
	var failedAllocsMetrics []*AllocMetrics

	// A list with the successful Allocations' Metrics.
	var successfulAllocsMetrics []*AllocMetrics

	// TODO: try to resume theses following long comments a bit.
	// ...
	// We first need to get all the failed Allocations to report all of them.
	// This is not as simple as just going through the FailedAllocs list in
	// the current Plan.
	// ...
	// Allocations are mappings between a TaskGroup in a Job, and a Node.
	// If there is a failed Allocation, it means that not all Tasks in a
	// given TaskGroup were able to allocate, meaning the entire TaskGroup
	// could not allocate, since its all its Tasks must run together.
	// ...
	// A Job can have multiple TaskGroups, but simultaneuously each TaskGroup
	// can have a 'count' value, meaning that such TaskGroup will be attempted
	// to be duplicated that much times. Also, TaskGroups are allocated in a
	// random fashion (not the order given in the Job specification).
	// ...
	// A 'count' of 5 in a TaskGroup means such TaskGroup will be attempted
	// to be allocated 5 separate times, but not necessarily on the same Node,
	// and also not all of those 5 may allocate, but there's not a strict need
	// for all those to be allocated or not (the strict requirement is only for
	// Tasks within a same TaskGroup) so 3 out of 5 duplicates of the TaskGroup
	// may allocate and 2 out of 5 may fail to do so.
	// ...
	// But when one or many of the 'duplicates' of a multi-count TaskGroup fails
	// to allocate, only one will be reported in the FailedAllocs list of the Plan.
	// We want to know exactly how many of these 'duplicates' failed. Maybe an
	// improvement on Nomad itself outside of the Simulator package would be to
	// include all the failing duplicate TaskGroups Allocations in this list, not
	// just a single one.
	// ...
	// So lets find out which were exactly the TaskGroups that failed to
	// allocate, and in the case of a multi-count TaskGroup, how many of
	// its copies failed to allocate.

	for _, plan := range simulatorSnapshot.Plans {
		// Look for the Plan with associated with the current Eval (which was
		// created after processing the Job during the simulator iteration).
		if plan.EvalID != eval.ID {
			continue
		}

		for _, allocsByNode := range plan.NodeAllocation {
			for _, alloc := range allocsByNode {
				successfulAllocsMetrics = append(successfulAllocsMetrics, ParseAllocMetrics(alloc))
			}
		}

		// Iterate over the failed Allocations in the Plan associated with the Job
		// that was processed during a simulator iteration.
		for _, failedAlloc := range plan.FailedAllocs {
			// The only thing the failed Allocation contains about its TaskGroup
			// is the Name.
			failedTaskGroupName := failedAlloc.TaskGroup

			// The Job to which the failed TaskGroup belongs to.
			job := failedAlloc.Job

			// To get the TaskGroup struct itself, we first retrieved the TaskGroup name
			// from the failed Allocation, then the Job to which the failed Allocation is
			// associated...
			for _, taskGroup := range job.TaskGroups {
				// ... Then, with the name, we can find the failing TaskGroup among the
				// TaskGroups of the Job, because TaskGroups names are unique inside Jobs.
				if taskGroup.Name == failedTaskGroupName {
					totalCount := taskGroup.Count
					// For System Scheduling, the total count of instances of a TaskGroup
					// is multiplied by the total amount of Nodes.
					if job.Type == structs.JobTypeSystem {
						totalCount *= len(nodeIDs)
					}
					if totalCount == 1 {
						// Check if the failing TaskGroup had a singular count, if it
						// does, it means it was just a single TaskGroup which failed.
						failedAllocsMetrics = append(failedAllocsMetrics, ParseAllocMetrics(failedAlloc))
					} else {
						// If it didn't have a singular count, there's the possibility
						// that some duplicates of the TaskGroup did allocate, but we
						// do not know exactly how many to report as failing, since only
						// one of those failing Allocations will be stored in the failed
						// list. The reason for this is unknown, may be its the desired
						// Nomad behavior, or maybe its something they could improve.
						successCount := 0
						// Lets count the number of successful Allocations for duplicates
						// of the failing TaskGroup.
						for _, allocsByNode := range plan.NodeAllocation {
							for _, alloc := range allocsByNode {
								// Substract from the total count amount the number of
								// successful allocations for that multi-count TaskGroup.
								if alloc.TaskGroup == failedAlloc.TaskGroup {
									successCount++
								}
							}
						}
						failedCount := totalCount - successCount
						for i := 0; i < failedCount; i++ {
							failedAllocsMetrics = append(failedAllocsMetrics, ParseAllocMetrics(failedAlloc))
						}
					}
				}
			}
		}
	}

	for _, nodeID := range nodeIDs {
		// Get the node by its ID
		node, _ := state.NodeByID(nodeID)
		// Set the metrics struct with the base resources of the node. These resources do not
		// reflect the resources consumed by the current allocations.
		nodeMetrics := ParseNodeMetrics(node)

		// Get the allocations associated with the current node
		allocations, _ := state.AllocsByNode(nodeID)
		for _, allocation := range allocations {
			// And then the resources consumed by such association, to reflect them on the current
			// node resource consumption.
			resources := allocation.Resources
			nodeMetrics.Available.CPU = nodeMetrics.Available.CPU - resources.CPU
			nodeMetrics.Available.MemoryMB = nodeMetrics.Available.MemoryMB - resources.MemoryMB
			nodeMetrics.Available.DiskMB = nodeMetrics.Available.DiskMB - resources.DiskMB
			nodeMetrics.Available.IOPS = nodeMetrics.Available.IOPS - resources.IOPS
		}
		nodesMetrics = append(nodesMetrics, nodeMetrics)
	}

	evaluatedJob, err := state.JobByID(eval.JobID)
	noErr(err)
	return &IterationMetrics{
		JobMetrics:              ParseJobMetrics(evaluatedJob),
		NodesMetrics:            nodesMetrics,
		FailedAllocsMetrics:     failedAllocsMetrics,
		SuccessfulAllocsMetrics: successfulAllocsMetrics,
	}
}
