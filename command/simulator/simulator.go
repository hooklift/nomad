package simulator

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/command"
	"github.com/hashicorp/nomad/jobspec"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/scheduler"
)

type SimulatorCommand struct {
	command.Meta
}

func (c *SimulatorCommand) Help() string {
	helpText := `
Usage: nomad simulator -clients <paths> -jobs <paths>

  -clients <paths>
    A list of paths to client specification files separated by commas.

  -jobs <paths>
    A list of paths to job specification files separated by commas.
  

    Example:
      $ nomad simulator -clients=client1.hcl,client2.chl -jobs=job1.nomad
`
	return strings.TrimSpace(helpText)
}

func (c *SimulatorCommand) Synopsis() string {
	return "Run a simulator to evaluate scheduling and placements"
}

func (c *SimulatorCommand) Run(args []string) int {

	snapshotChan := make(chan *SimulatorSnapshot)
	go func() {
		for {
			snapshot := <-snapshotChan
			fmt.Println("Hola", snapshot)
			// getMetrics(snapshot)
		}
	}()

	// Clients' paths passed as arguments in a list. Maybe we can pass a file
	// that contains the paths to all the config files for the clients, since
	// we may want to generate them programatically and set thousands. We can't
	// pass a folder and let Nomad handle that because when a folder of config
	// files is loaded, it merges all of them sequentially into a single node.
	var clientsPaths []string
	// A string which will contain as a single value all clients of the flag.
	var clientsFlag string
	// Jobs' paths passed as arguments in a list.
	var jobsPaths []string
	// A string which will contain as a single value all jobs of the flag.
	var jobsFlag string

	flags := flag.NewFlagSet("simulator", flag.ContinueOnError)
	flags.Usage = func() { c.Ui.Error(c.Help()) }

	flags.StringVar(&clientsFlag, "clients", "", "clients")
	flags.StringVar(&jobsFlag, "jobs", "", "jobs")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	// Split the clients from the single argument string. For now lets keep
	// the ',' separator.
	// TODO: make the argument a folder name, and get all files in such folder,
	// or a text file with the paths to all the desired files.
	if clientsFlag != "" {
		clientsPaths = strings.Split(clientsFlag, ",")
	} else {
		return 0
	}

	// DEBUG, print the clients arguments
	fmt.Printf("%v\n", clientsPaths)

	// Split the jobs from the single argument string. For now lets keep
	// the ',' separator.
	// TODO: make the argument a folder name, and get all files in such folder,
	// or a text file with the paths to all the desired files.
	if jobsFlag != "" {
		jobsPaths = strings.Split(jobsFlag, ",")
	} else {
		return 0
	}

	// DEBUG, print the jobs arguments
	fmt.Printf("%v\n", jobsPaths)

	// To store the nodes.
	var nodes []*structs.Node
	// To store the jobs.
	var jobs []*structs.Job

	// To store the node IDs, useful for retrieving from the harness state.
	var nodeIDs []string
	// To store the job IDs, useful for retrieving from the harness state.
	var jobIDs []string

	// Load the node configuration files.
	// TODO: be able to create a new Node via API.
	for _, clientPath := range clientsPaths {
		current, err := LoadNodeFile(clientPath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf(
				"Error loading configuration from %s: %s", clientPath, err))
			return 1
		}

		nodes = append(nodes, current)
		nodeIDs = append(nodeIDs, current.ID)
	}

	// Load the job spec files.
	// TODO: be able to create a new Job via API.
	for _, jobPath := range jobsPaths {
		current, err := jobspec.ParseFile(jobPath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error parsing job file %s: %s", jobPath, err))
			return 1
		}

		jobs = append(jobs, current)
		jobIDs = append(jobIDs, current.ID)
	}

	// Create a new SimulatorHarness, in the same fashion that the Harness is
	// used for Unit Testing, since that is some sort of a simulation for
	// specific features, in our case, simulation scheduling and placements,
	// but not with fixed mocked values but with our own input.
	h := NewSimulatorHarness()

	// Insert to the Harness client nodes from configuration files.
	for _, node := range nodes {
		noErr(h.State.UpsertNode(h.NextIndex(), node))
	}

	// Insert to the SimulatorHarness jobs from the job spec files.
	for _, job := range jobs {
		noErr(h.State.UpsertJob(h.NextIndex(), job))
	}

	// For each job...
	for _, job := range jobs {
		// Create mock evaluations to register the jobs.
		eval := &structs.Evaluation{
			ID:          structs.GenerateUUID(),
			Priority:    job.Priority,
			TriggeredBy: structs.EvalTriggerJobRegister,
			JobID:       job.ID,
		}

		// Process the evaluation
		// TODO: use the other kind of schedulers depending on type of job
		switch job.Type {
		case structs.JobTypeSystem:
			noErr(h.Process(scheduler.NewSystemScheduler, eval))
		case structs.JobTypeBatch:
			noErr(h.Process(scheduler.NewBatchScheduler, eval))
		case structs.JobTypeService:
			noErr(h.Process(scheduler.NewServiceScheduler, eval))
		}

		// Step by step, process by process, state of node (resources, allocations) at each
		// iteration. Send this information via different channels depending on the type of scheduler.
		simulatorSnapshot := &SimulatorSnapshot{
			harness: h,
			nodeIDs: nodeIDs,
			jobIDs:  jobIDs,
		}

		getMetrics(simulatorSnapshot)
		// snapshotChan <- simulatorSnapshot
	}

	return 0
}
