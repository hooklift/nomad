package command

import (
	"fmt"
	"os"
	"strings"
)

type FSCatCommand struct {
	Meta
}

func (f *FSCatCommand) Help() string {
	helpText := `
	Usage: nomad fs-cat [alloc-id] [path]

	Dispays a file in an allocation directory at the given path.
	The path is relative to the allocation directory
	`
	return strings.TrimSpace(helpText)
}

func (f *FSCatCommand) Synopsis() string {
	return "displays a file at a given location"
}

func (f *FSCatCommand) Run(args []string) int {
	flags := f.Meta.FlagSet("fs-list", FlagSetClient)
	flags.Usage = func() { f.Ui.Output(f.Help()) }

	if err := flags.Parse(args); err != nil {
		return 1
	}
	args = flags.Args()

	if len(args) < 1 {
		f.Ui.Error("a valid alloc id is essential")
		return 1
	}

	allocID := args[0]
	path := "/"
	if len(args) == 2 {
		path = args[1]
	}

	client, err := f.Meta.Client()
	if err != nil {
		f.Ui.Error(fmt.Sprintf("Error inititalizing client: %v", err))
		return 1
	}

	// Query the allocation info
	alloc, _, err := client.Allocations().Info(allocID, nil)
	if err != nil {
		if len(allocID) == 1 {
			f.Ui.Error(fmt.Sprintf("Identifier must contain at least two characters."))
			return 1
		}
		if len(allocID)%2 == 1 {
			// Identifiers must be of even length, so we strip off the last byte
			// to provide a consistent user experience.
			allocID = allocID[:len(allocID)-1]
		}

		allocs, _, err := client.Allocations().PrefixList(allocID)
		if err != nil {
			f.Ui.Error(fmt.Sprintf("Error querying allocation: %v", err))
			return 1
		}
		if len(allocs) == 0 {
			f.Ui.Error(fmt.Sprintf("No allocation(s) with prefix or id %q found", allocID))
			return 1
		}
		if len(allocs) > 1 {
			// Format the allocs
			out := make([]string, len(allocs)+1)
			out[0] = "ID|Eval ID|Job ID|Task Group|Desired Status|Client Status"
			for i, alloc := range allocs {
				out[i+1] = fmt.Sprintf("%s|%s|%s|%s|%s|%s",
					alloc.ID,
					alloc.EvalID,
					alloc.JobID,
					alloc.TaskGroup,
					alloc.DesiredStatus,
					alloc.ClientStatus,
				)
			}
			f.Ui.Output(fmt.Sprintf("Prefix matched multiple allocations\n\n%s", formatList(out)))
			return 0
		}
		// Prefix lookup matched a single allocation
		alloc, _, err = client.Allocations().Info(allocs[0].ID, nil)
		if err != nil {
			f.Ui.Error(fmt.Sprintf("Error querying allocation: %s", err))
			return 1
		}
	}

	// Stat the file to find it's size
	file, _, err := client.AllocFS().Stat(alloc, path, nil)
	if err != nil {
		f.Ui.Error(fmt.Sprintf("Error stating file: %v:", err))
		return 1
	}
	if file.IsDir {
		f.Ui.Error("The file %q is a directory")
		return 1
	}

	// Get the contents of the file
	offset := 0
	limit := file.Size
	if _, err := client.AllocFS().ReadAt(alloc, path, int64(offset), limit, os.Stdout, nil); err != nil {
		f.Ui.Error(fmt.Sprintf("Error reading file: %v", err))
		return 1
	}
	return 0
}
