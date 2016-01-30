package simulator

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

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
Usage: nomad simulator -nodeList <path> -jobList <path> -outFile <path>

  -nodeList <path>
    A path to a file containing a list of node specification file paths, one per line.

  -jobList <path>
    A path to a file containing a list of job specification file paths, one per line.
    Jobs will be iteratively registered in the order they're listed.

  -outFile <path>
    A path where the output JSON file will be created into. If not specified, default
    filename is simulator_output.json, at the location where binary was run.

    Example:
      $ nomad simulator -nodeList nodes.txt -jobList jobs.txt
`
	return strings.TrimSpace(helpText)
}

func (c *SimulatorCommand) Synopsis() string {
	return "Run a simulator to get metrics about the placement of jobs"
}

func (c *SimulatorCommand) Run(args []string) int {

	// A flag which will contain as a single string the path to a file containing nodes' configuration file paths.
	var nodeListFlag string
	// Nodes' paths parsed from nodes' flag string.
	var nodesPaths []string
	// A flag which will contain as a single string the path to a file containing jobs' configuration file paths.
	var jobListFlag string
	// Jobs' paths parsed from jobs' flag string.
	var jobsPaths []string
	// A flag which will contain the path for the result file.
	var outFileFlag string
	// Path for the output file.
	var outFilePath string

	// All the given jobs will be attempted to be placed on the nodes iteratively, until
	// it finishes and it will output a JSON file with the 'logs' describing the cluster state
	// at each iteration, which can be used later for analysis.
	flags := flag.NewFlagSet("simulator", flag.ContinueOnError)
	flags.Usage = func() { c.Ui.Error(c.Help()) }

	flags.StringVar(&nodeListFlag, "nodeList", "", "nodeList")
	flags.StringVar(&jobListFlag, "jobList", "", "jobList")
	flags.StringVar(&outFileFlag, "outFile", "", "outFile")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	// Retrieve the nodes' config file paths from the file given as argument.
	if nodeListFlag != "" {
		var err error
		nodesPaths, err = readPaths(nodeListFlag)
		noErr(err)
	} else {
		return 1
	}

	// Retrieve the jobs' config file paths from the file given as argument.
	if jobListFlag != "" {
		var err error
		jobsPaths, err = readPaths(jobListFlag)
		noErr(err)
	} else {
		return 1
	}

	if outFileFlag != "" {
		outFilePath = outFileFlag
	} else {
		outFilePath = "simulator_output.json"
	}

	// Stores the Nodes' pointers.
	var nodes []*structs.Node
	// Stores the the Jobs' pointers.
	var jobs []*structs.Job

	// Stores the node IDs, useful for retrieving Nodes from the harness internal state.
	var nodeIDs []string

	// Load the Nodes' configuration files.
	for _, nodePath := range nodesPaths {
		current, err := LoadNodeFile(nodePath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error loading configuration from %s: %s", nodePath, err))
			return 1
		}

		nodes = append(nodes, current)
		nodeIDs = append(nodeIDs, current.ID)
	}

	// Load the Jobs' configuration files.
	for _, jobPath := range jobsPaths {
		current, err := jobspec.ParseFile(jobPath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error parsing job file %s: %s", jobPath, err))
			return 1
		}

		// The usual rules apply for allocation:
		// Jobs must belong to same region of nodes, otherwise won't allocate.
		// Jobs must have a datacenter related to the one in the nodes, otherwise won't allocate.
		// Jobs' task drivers must be related to the one in nodes, otherwise won't allocate.
		// ...
		// For specifically evaluating the bin packing, its better if all jobs and nodes belong
		// to a single datacenter, in the same region, with the same task drivers, so all nodes
		// are eligible for scheduling, and the bin packing can be seen more evidently.

		jobs = append(jobs, current)
	}

	// Create a new SimulatorHarness, in the same fashion that the Harness is
	// used for Unit Testing, since that is some sort of a simulation for
	// specific features, in our case, scheduling and placement simulation,
	// but with user inputted configurations instead of mocked values.
	h := NewSimulatorHarness()

	// Upsert to the Harness Nodes parsed from configuration files.
	for _, node := range nodes {
		noErr(h.State.UpsertNode(h.NextIndex(), node))
	}

	// Jobs will be processed iteratively in the order they are present at
	// input. After each Job is processed, metrics will be extracted from
	// the present cluster state. This extraction of metrics is done in a
	// concurrent fashion while other jobs are processing.
	// ...
	// This is a channel where a snapshot of the simulator will be sent for
	// metrics extraction after each job has processed.
	snapshotChan := make(chan *SimulatorSnapshot)

	// For each job...
	for _, job := range jobs {
		go func(job *structs.Job) {

			// Upsert to the Harness Jobs parsed from configuration files.
			noErr(h.State.UpsertJob(h.NextIndex(), job))

			// Create mock Evaluations to register the Jobs.
			eval := &structs.Evaluation{
				ID:          structs.GenerateUUID(),
				Priority:    job.Priority,
				TriggeredBy: structs.EvalTriggerJobRegister,
				JobID:       job.ID,
			}

			startTimestamp := int64(time.Now().UnixNano())

			// Process the evaluation, depending on the type of scheduler
			// designated for the job. Modify this if a custom scheduler is
			// to be used.
			switch job.Type {
			case structs.JobTypeSystem:
				noErr(h.Process(scheduler.NewSystemScheduler, eval))
			case structs.JobTypeBatch:
				noErr(h.Process(scheduler.NewBatchScheduler, eval))
			case structs.JobTypeService:
				noErr(h.Process(scheduler.NewServiceScheduler, eval))
			}

			// A snapshot of the simulator after each Job has been processed contains the
			// ID to its Nodes, the plans, and the internal state.
			simulatorSnapshot := &SimulatorSnapshot{
				Eval:    eval,
				Plans:   h.Plans,
				State:   h.State,
				NodeIDs: nodeIDs,
				Time:    startTimestamp,
			}

			// Send the snapshot to a metrics evaluation loop so that metrics can be
			// extracted concurrently.
			snapshotChan <- simulatorSnapshot
		}(job)
	}

	var outputNodes []*Node
	for _, nodeID := range nodeIDs {
		node, err := h.State.NodeByID(nodeID)
		noErr(err)
		outputNodes = append(outputNodes, parseNode(node))
	}

	var jobEvaluations []*JobEvaluationMetrics

	// This is the metrics evaluation loop.
	for iteration := 0; iteration < len(jobs); iteration++ {
		// Receive the simulated cluster snapshot for metrics extraction.
		snapshot := <-snapshotChan

		jobEvaluationMetrics := getMetrics(snapshot)

		jobEvaluations = append(jobEvaluations, jobEvaluationMetrics)
	}

	// Since Jobs are processed in separate goroutines, we first insert each of them
	simulatorOutput := &SimulatorOutput{
		Nodes:          outputNodes,
		JobEvaluations: jobEvaluations,
	}

	marshald, _ := json.MarshalIndent(simulatorOutput, "", "\t")
	err := ioutil.WriteFile(outFilePath, marshald, 0755)
	noErr(err)

	return 0
}

// readPaths reads a whole file containing paths to files as lines into memory
// and returns a slice of its lines, separaing the paths.
func readPaths(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	// Read the file, line by line, until EOF.
	for scanner.Scan() {
		path := scanner.Text()
		// Remove leading or trailing spaces or tabs from paths read.
		path = strings.Trim(path, " \t")
		// Ignore blank lines. If there were lines only containing spaces
		// or tab characters, those were trimmed in the last step.
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	return paths, scanner.Err()
}
