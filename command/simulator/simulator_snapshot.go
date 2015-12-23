package simulator

import (
	"fmt"

	"github.com/hashicorp/nomad/nomad/structs"
)

// This struct will hold the present state and metrics of all the nodes, jobs and allocations
// per iteration (job evaluation), which we'll use to analyse the schedulers. We'll later do
// a graphic web representation of the simulated cluster using information from this struct
// as temporal snapshots from the simulated cluster.
type SimulatorSnapshot struct {
	harness *SimulatorHarness
	nodeIDs []string
	jobIDs  []string
}

func getMetrics(simulatorSnapshot *SimulatorSnapshot) {

	h := simulatorSnapshot.harness
	nodeIDs := simulatorSnapshot.nodeIDs
	//jobIDs := simulatorSnapshot.jobIDs

	// DEBUG: This allows us to know the number of plans.
	fmt.Println("Plans: ", len(h.Plans))

	// To store the failed allocations.
	var failedAllocs []*structs.Allocation
	// To store the planned allocations.
	var plannedAllocs []*structs.Allocation

	for _, plan := range h.Plans {
		// Store the failed allocs aside for later analysis.
		for _, failed := range plan.FailedAllocs {
			failedAllocs = append(failedAllocs, failed)
		}

		// Store the planned allocs aside for later analysis.
		for _, allocList := range plan.NodeAllocation {
			plannedAllocs = append(plannedAllocs, allocList...)
		}

		// This is the information of the planned Allocations.
		for _, allocation := range plannedAllocs {
			fmt.Println("-------- INFO --------")
			fmt.Println("JobID: ", allocation.JobID, "NodeID: ", allocation.NodeID)
			fmt.Println("------ RESOURCES -----")
			resources := allocation.Resources
			fmt.Printf("CPU: %v, MEM: %v, STO: %v\n", resources.CPU, resources.MemoryMB, resources.DiskMB)
			fmt.Println("------- METRICS ------")
			metrics := allocation.Metrics
			fmt.Printf("Time: %v\n", metrics.AllocationTime)
			fmt.Println("----- TASK GROUP -----")
			fmt.Println(allocation.TaskGroup)
			fmt.Println("----- TASK STATES ----")
			taskStates := allocation.TaskStates
			for key, value := range taskStates {
				fmt.Println(key, " ", value)
			}
		}

		// This is the information of the failed Allocations.
		for _, allocation := range failedAllocs {
			fmt.Println("-------- INFO --------")
			fmt.Println("JobID: ", allocation.JobID, "NodeID: ", allocation.NodeID)
			fmt.Println("------ RESOURCES -----")
			resources := allocation.Resources
			fmt.Printf("CPU: %v, MEM: %v, STO: %v\n", resources.CPU, resources.MemoryMB, resources.DiskMB)
			fmt.Println("------- METRICS ------")
			metrics := allocation.Metrics
			fmt.Printf("Time: %v\n", metrics.AllocationTime)
			fmt.Println("----- TASK GROUP -----")
			fmt.Println(allocation.TaskGroup)
			fmt.Println("----- TASK STATES ----")
			taskStates := allocation.TaskStates
			for key, value := range taskStates {
				fmt.Println(key, " ", value)
			}
		}

		// This is the information of the Nodes.
		for _, nodeID := range nodeIDs {
			// Get the node by its ID
			node, _ := h.State.NodeByID(nodeID)
			// Get the resources of the node. These resources do not reflect the resources consumed
			// by the current allocation.
			resources := node.Resources
			cpu := resources.CPU
			mem := resources.MemoryMB
			sto := resources.DiskMB
			// Get the allocations present on the current node...
			allocations, _ := h.State.AllocsByNode(nodeID)
			for _, alloc := range allocations {
				// ... and then update the values to reflect consumption.
				cpu = cpu - alloc.Resources.CPU
				mem = mem - alloc.Resources.MemoryMB
				sto = sto - alloc.Resources.DiskMB
			}
			fmt.Println("----- Node Information -----")
			fmt.Println("Node ID: ", node.ID)
			fmt.Printf("CPU: %v/%v, MEM: %v/%v, STO: %v/%v\n", cpu, resources.CPU, mem, resources.MemoryMB, sto, resources.DiskMB)
		}
	}
}
