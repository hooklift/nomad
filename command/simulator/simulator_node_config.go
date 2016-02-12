package simulator

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/nomad/nomad/structs"
)

// NodeConfig is the struct to parse a node configuration data into.
// We could do without defining this if we put annotations on the original
// Node struct in 'structs', but we don't know if its a good idea to make
// this kind of changes outside of the simulator package (it should adapt
// to Nomad's logic, not the other way around), since in production nodes'
// structs aren't retrieved from a config file but from the system itself
// via fingerprinting. It will be read from either HCL or JSON files.
type NodeConfig struct {
	// The ID of this Node
	ID string `hcl:"id" json:"id"`

	// Datacenter for this Node
	Datacenter string `hcl:"datacenter" json:"datacenter"`

	// Node name
	Name string `hcl:"name" json:"name"`

	// Attributes is an arbitrary set of key/value data that can be used for
	// defining constraints or available task drivers. Examples
	// include "kernel.name=linux", "arch=386", "driver.docker=1",
	// "driver.exec=1", "docker.runtime=1.8.3"
	Attributes map[string]string `hcl:"attributes" json:"attributes"`

	// Resources is the available resources on the client.
	// For example "cpu=2700" "memory=8192"
	Resources *SimulatorResources `hcl:"resources" json:"resources"`

	// Reserved is the set of Resources that are reserved, and should be
	// subtracted from the total Resources for the purposes of scheduling.
	// This may be provide certain high-watermark tolerances or because
	// of external schedulers consuming resources.
	Reserved *SimulatorResources `hcl:"reserved" json:"reserved"`

	// Links are used to 'link' this client to external systems.
	// For example "consul=foo.dc1" "aws=i-83212" "ami=ami-123"
	Links map[string]string `hcl:"links" json:"links"`

	// Meta is used to associate arbitrary metadata with this client.
	// This is opaque to Nomad.
	Meta map[string]string `hcl:"meta" json:"meta"`

	// NodeClass is an opaque identifier used to group nodes together
	// for the purpose of determining scheduling pressure.
	NodeClass string `hcl:"node_class" json:"node_class"`

	// Drain is controlled by the servers, and not the client. If true, no jobs
	// will be scheduled to this node, and existing allocations will be drained.
	Drain bool `hcl:"drain" json:"drain"`

	// Status of this Node
	// For example: "initializing", "ready", "down"
	Status string `hcl:"status" json:"status"`

	// StatusDescription is meant to provide more human useful information
	StatusDescription string `hcl:"status_description" json:"status_description"`

	// Raft Indexes
	CreateIndex uint64 `hcl:"create_index" json:"create_index"`
	ModifyIndex uint64 `hcl:"modify_index" json:"modify_index"`
}

// Get the regular Node struct from the NodeConfig struct (intermediary for
// HCL/JSON parsing, to avoid putting such parsing tags on the real struct).
func ConvertNode(n *NodeConfig) *structs.Node {
	return &structs.Node{
		ID:         structs.GenerateUUID(),
		Datacenter: n.Datacenter,
		Name:       n.Name,
		Attributes: n.Attributes,
		Resources: &structs.Resources{
			CPU:      n.Resources.CPU,
			MemoryMB: n.Resources.MemoryMB,
			DiskMB:   n.Resources.DiskMB,
			IOPS:     n.Resources.IOPS,
			// TODO: read the network resources from the config file, or maybe not, since
			// networks are irrelevant for in-memory placement simulation. Ask about this.
			// In Unit Tests, all mocked nodes have the same network resource configuration,
			// which is the following for the regular resources.
			Networks: []*structs.NetworkResource{
				&structs.NetworkResource{
					Device: "eth0",
					CIDR:   "192.168.0.100/32",
					MBits:  2147483647,
				},
			},
		},
		Reserved: &structs.Resources{
			CPU:      n.Reserved.CPU,
			MemoryMB: n.Reserved.MemoryMB,
			DiskMB:   n.Reserved.DiskMB,
			IOPS:     n.Reserved.IOPS,
			// TODO: read the network resources from config file, or maybe not, since
			// networks are irrelevant for in-memory placement simulation. Ask about this.
			// In Unit Tests, all mocked nodes have the same network resource configuration,
			// which is the following for the reserved resources.
			Networks: []*structs.NetworkResource{
				&structs.NetworkResource{
					Device:        "eth0",
					IP:            "192.168.0.100",
					ReservedPorts: []structs.Port{{Label: "main", Value: 22}},
					MBits:         1,
				},
			},
		},
		Links:             n.Links,
		Meta:              n.Meta,
		NodeClass:         n.NodeClass,
		Drain:             n.Drain,
		Status:            n.Status,
		StatusDescription: n.StatusDescription,
		CreateIndex:       n.CreateIndex,
		ModifyIndex:       n.ModifyIndex,
	}
}

// Parse a NodeConfig struct from a HCL file.
func parseHCL(c []byte) (*NodeConfig, error) {
	// Up to this point, the file is read as a string, we need to parse
	// it and return the resulting struct.
	s := string(c)
	// Parse!
	obj, err := hcl.Parse(s)
	if err != nil {
		return nil, err
	}

	// Start building the result, for now its our intermediate struct
	var result NodeConfig
	if err := hcl.DecodeObject(&result, obj); err != nil {
		return nil, err
	}

	return &result, nil
}

// Parse a NodeConfig struct from a JSON file.
func parseJSON(c []byte) (*NodeConfig, error) {
	// Start building the result, for now its our intermediate struct.
	var result NodeConfig
	if err := json.Unmarshal(c, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// LoadNodeConfigFile loads the Node struct from the given file path.
func LoadNodeFile(path string) (*structs.Node, error) {
	// Start building the result, for now its our intermediate struct.
	var result *NodeConfig

	// Read the file
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	length := len(path)
	// Parse HCL if the extension is ".hcl", or JSON if extension is ".json"...
	// or return an error if its neither of those extensions.
	if length > 4 && path[length-4:] == ".hcl" {
		result, err = parseHCL(c)
		if err != nil {
			return nil, err
		}
	} else if length > 5 && path[length-5:] == ".json" {
		result, err = parseJSON(c)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unrecognized extension: %v", path)
	}

	return ConvertNode(result), nil
}
