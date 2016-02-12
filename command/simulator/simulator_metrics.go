package simulator

import "github.com/hashicorp/nomad/nomad/structs"

// Get the Metrics of cluster allocations (successful or failed) after a given iteration,
// which is the processing of one Job. Also retrieve the current resource consumption of
// all the Nodes present in the cluster.
func getMetrics(simSnapshot *SimSnapshot) *JobEvaluationMetrics {

	state := simSnapshot.State
	nodeIDs := simSnapshot.NodeIDs
	eval := simSnapshot.Eval
	startTimestamp := simSnapshot.Time

	// A list of NodeUsageChange associated with Nodes involved in new Allocations.
	var nodeUsageChanges []*NodeUsageChange

	// A list with the failed Allocations' Metrics.
	var failedAllocsMetrics []*AllocMetrics

	// A list with the successful Allocations' Metrics.
	var successfulAllocsMetrics []*AllocMetrics

	// TODO: try to reduce these following long comments a bit.
	// ...
	// We first need to get all the failed Allocations to report all of them.
	// This is not as simple as just going through the FailedAllocs list in
	// the current Plan. This is because...
	// ...
	// Allocations are mappings between a TaskGroup in a Job, and a Node.
	// If there is a failed Allocation, it means that not all Tasks in a
	// given TaskGroup were able to allocate, meaning the entire TaskGroup
	// could not allocate, since its all its Tasks must run together.
	// ...
	// A Job can have multiple TaskGroups, but simultaneuously each TaskGroup
	// can have a 'count' value, meaning that such TaskGroup will be attempted
	// to be placed that much times. Also, TaskGroups are allocated in a random
	// fashion (not the order given in the Job specification).
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

	evaluatedJob, err := state.JobByID(eval.JobID)
	noErr(err)
	jobMetrics := ParseJobMetrics(evaluatedJob)

	for _, plan := range simSnapshot.Plans {
		// Look for the Plan with associated with the current Eval (which was
		// created after processing the Job during the simulator iteration).
		if plan.EvalID != eval.ID {
			continue
		}

		// Lets look for the highest Allocation time amongst the Job Allocations.
		maxAllocTime := int64(-1)

		for _, allocsByNode := range plan.NodeAllocation {
			for _, alloc := range allocsByNode {
				parsedAlloc := ParseAllocMetrics(alloc)
				successfulAllocsMetrics = append(successfulAllocsMetrics, parsedAlloc)
				// The final timestamp for the Job will be the biggest AllocationTimestamp of any of its
				// allocations. If there's not a single successful Allocation, then the -1 value will be
				// preserved and the Job will be considered failed.
				if maxAllocTime < parsedAlloc.AllocationTime {
					maxAllocTime = parsedAlloc.AllocationTime
				}
			}
		}

		// The Job starting time is whenever it was started to be processed in a goroutine
		// (added to the State, eval created, scheduling processed). And the Job final time
		// is the time it took to the longest Allocation to place. If no Allocation placed,
		// the finishing time will be -1. All these times are in nanoseconds, since Allocation
		// timestamps are in UNIX nanoseconds.
		jobMetrics.StartTimestamp = startTimestamp
		if maxAllocTime != -1 {
			jobMetrics.FinalTimestamp = startTimestamp + maxAllocTime
		} else {
			jobMetrics.FinalTimestamp = -1
		}

		// Now, to determine the real amount of failing Allocations, lets calculate for a
		// given TaskGroup how many Allocations should've been done, and how many were
		// placed, so the difference is the amount of failing Allocations.

		// Iterate over the failed Allocations in the Plan associated with the Job
		// that was processed during a simulator iteration.
		for _, failedAlloc := range plan.FailedAllocs {
			// The only thing the failed Allocation contains about its associated
			// TaskGroup is the Name.
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
					// that should have placed is multiplied by the total amount of Nodes.
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

	changedNodes := make(map[string]bool)
	for _, alloc := range successfulAllocsMetrics {
		// Store uniquely the Node IDs associated with successful Allocations
		// after the current Job scheduling has been done.
		changedNodes[alloc.NodeID] = true
	}

	for nodeID := range changedNodes {
		// Now, for those Nodes affected during this iteration, calculate their
		// currently available resources. We first find out which were the Nodes
		// affected during this iteration to avoid doing this calculation for all
		// the Nodes in the simulater cluster, which could be expensive.
		node, err := state.NodeByID(nodeID)
		noErr(err)
		// Get the resources currently available to this Node.
		available := getAvailable(node, state)
		nodeUsageChange := &NodeUsageChange{
			ID:        node.ID,
			Available: available,
		}
		// This list will only contain updates to the resource usage of Nodes
		// affected by placements during this iteration.
		nodeUsageChanges = append(nodeUsageChanges, nodeUsageChange)
	}

	return &JobEvaluationMetrics{
		JobMetrics:              jobMetrics,
		NodeUsageChanges:        nodeUsageChanges,
		FailedAllocsMetrics:     failedAllocsMetrics,
		SuccessfulAllocsMetrics: successfulAllocsMetrics,
	}
}
