package simulator

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/scheduler"
)

//
// All these functions and types are from the scheduler package, but are not accesible outside of
// testing since they are in the test files. The 'Harness' and related functions allow to test
// the scheduler by simulating certain conditions under mocked clients/jobs/etc. We want to simulate
// the scheduling process, but using our own inputted Nodes and Jobs instead of mocked values.
//

// RejectPlan is used to always reject the entire plan and force a state refresh
type RejectPlan struct {
	SimHarness *SimHarness
}

func (r *RejectPlan) SubmitPlan(*structs.Plan) (*structs.PlanResult, scheduler.State, error) {
	result := new(structs.PlanResult)
	result.RefreshIndex = r.SimHarness.NextIndex()
	return result, r.SimHarness.State, nil
}

func (r *RejectPlan) UpdateEval(eval *structs.Evaluation) error {
	return nil
}

func (r *RejectPlan) CreateEval(*structs.Evaluation) error {
	return nil
}

// SimHarness is a lightweight testing harness for simulating
// schedulers. It manages a state store copy and provides the planner
// interface. It can be extended for various testing uses, in our case,
// for simulation.
type SimHarness struct {
	State *state.StateStore

	Planner  scheduler.Planner
	planLock sync.Mutex

	Plans       []*structs.Plan
	Evals       []*structs.Evaluation
	CreateEvals []*structs.Evaluation

	nextIndex     uint64
	nextIndexLock sync.Mutex
}

// NewSimHarness is used to make a new simulator harness
func NewSimHarness() *SimHarness {
	state, err := state.NewStateStore(os.Stderr)
	if err != nil {
		// In the unit tests logic this was 't.Fatalf' so the appropiate
		// behaviour outside of testing for a fatal error would be... panic.
		panic(fmt.Sprintf("err: %v", err))
	}

	h := &SimHarness{
		State:     state,
		nextIndex: 1,
	}
	return h
}

// SubmitPlan is used to handle plan submission
func (h *SimHarness) SubmitPlan(plan *structs.Plan) (*structs.PlanResult, scheduler.State, error) {
	// Ensure sequential plan application
	h.planLock.Lock()
	defer h.planLock.Unlock()

	// Store the plan
	h.Plans = append(h.Plans, plan)

	// Check for custom planner
	if h.Planner != nil {
		return h.Planner.SubmitPlan(plan)
	}

	// Get the index
	index := h.NextIndex()

	// Prepare the result
	result := new(structs.PlanResult)
	result.NodeUpdate = plan.NodeUpdate
	result.NodeAllocation = plan.NodeAllocation
	result.AllocIndex = index

	// Flatten evicts and allocs
	var allocs []*structs.Allocation
	for _, updateList := range plan.NodeUpdate {
		allocs = append(allocs, updateList...)
	}
	for _, allocList := range plan.NodeAllocation {
		allocs = append(allocs, allocList...)
	}
	allocs = append(allocs, plan.FailedAllocs...)

	// Apply the full plan
	err := h.State.UpsertAllocs(index, allocs)
	return result, nil, err
}

func (h *SimHarness) UpdateEval(eval *structs.Evaluation) error {
	// Ensure sequential plan application
	h.planLock.Lock()
	defer h.planLock.Unlock()

	// Store the eval
	h.Evals = append(h.Evals, eval)

	// Check for custom planner
	if h.Planner != nil {
		return h.Planner.UpdateEval(eval)
	}
	return nil
}

func (h *SimHarness) CreateEval(eval *structs.Evaluation) error {
	// Ensure sequential plan application
	h.planLock.Lock()
	defer h.planLock.Unlock()

	// Store the eval
	h.CreateEvals = append(h.CreateEvals, eval)

	// Check for custom planner
	if h.Planner != nil {
		return h.Planner.CreateEval(eval)
	}
	return nil
}

// NextIndex returns the next index
func (h *SimHarness) NextIndex() uint64 {
	h.nextIndexLock.Lock()
	defer h.nextIndexLock.Unlock()
	idx := h.nextIndex
	h.nextIndex += 1
	return idx
}

// Snapshot is used to snapshot the current state
func (h *SimHarness) Snapshot() scheduler.State {
	snap, _ := h.State.Snapshot()
	return snap
}

// Scheduler is used to return a new scheduler from
// a snapshot of current state using the harness for planning.
func (h *SimHarness) Scheduler(factory scheduler.Factory) scheduler.Scheduler {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	return factory(logger, h.Snapshot(), h)
}

// Process is used to process an evaluation given a factory
// function to create the scheduler
func (h *SimHarness) Process(factory scheduler.Factory, eval *structs.Evaluation) error {
	sched := h.Scheduler(factory)
	return sched.Process(eval)
}

func (h *SimHarness) AssertEvalStatus(state string) {
	if len(h.Evals) != 1 {
		panic(fmt.Sprintf("bad: %#v", h.Evals))
	}
	update := h.Evals[0]

	if update.Status != state {
		panic(fmt.Sprintf("bad: %#v", update))
	}
}

// noErr is used to assert there are no errors
func noErr(err error) {
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}
}
