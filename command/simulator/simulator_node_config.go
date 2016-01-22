package simulator

import (
	"io/ioutil"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/nomad/nomad/structs"
)

// NodeConfig is the struct to parse a node configuration data into.
// We could do without defining this if we put annotations on the original
// Node struct in 'structs', but we don't know if its a good idea to make
// this kind of changes outside of the simulator package (it should adapt
// to Nomad's logic, not the other way around), since in production nodes'
// configurations aren't retrieved from a config file but from the system
// itself via fingerprinting.
type NodeConfig struct {
	// The ID of this Node
	ID string `hcl:"id"`

	// Datacenter for this Node
	Datacenter string `hcl:"datacenter"`

	// Node name
	Name string `hcl:"name"`

	// Attributes is an arbitrary set of key/value data that can be used for
	// defining constraints or available task drivers. Examples
	// include "kernel.name=linux", "arch=386", "driver.docker=1",
	// "driver.exec=1", "docker.runtime=1.8.3"
	Attributes map[string]string `hcl:"attributes"`

	// Resources is the available resources on the client.
	// For example "cpu=2700" "memory=8192"
	Resources *SimulatorResources `hcl:"resources"`

	// Reserved is the set of Resources that are reserved, and should be
	// subtracted from the total Resources for the purposes of scheduling.
	// This may be provide certain high-watermark tolerances or because
	// of external schedulers consuming resources.
	Reserved *SimulatorResources `hcl:"reserved"`

	// Links are used to 'link' this client to external systems.
	// For example "consul=foo.dc1" "aws=i-83212" "ami=ami-123"
	Links map[string]string `hcl:"links"`

	// Meta is used to associate arbitrary metadata with this client.
	// This is opaque to Nomad.
	Meta map[string]string `hcl:"meta"`

	// NodeClass is an opaque identifier used to group nodes together
	// for the purpose of determining scheduling pressure.
	NodeClass string `hcl:"node_class"`

	// Drain is controlled by the servers, and not the client. If true, no jobs
	// will be scheduled to this node, and existing allocations will be drained.
	Drain bool `hcl:"drain"`

	// Status of this Node
	// For example: "initializing", "ready", "down"
	Status string `hcl:"status"`

	// StatusDescription is meant to provide more human useful information
	StatusDescription string `hcl:"status_description"`

	// Raft Indexes
	CreateIndex uint64 `hcl:"create_index"`
	ModifyIndex uint64 `hcl:"modify_index"`
}

// Get the regular Node struct from the NodeConfig struct (intermediary for
// HCL/JSON parsing, to avoid putting such parsing tags on the real struct).
func ParseNode(n *NodeConfig) *structs.Node {
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

// LoadNodeString is used to load a Node from a config string.
func LoadNodeString(s string) (*structs.Node, error) {
	// Parse!
	// TODO: add JSON parsing. And figure out how to decide whether to
	// apply HCL or JSON parsing to the file string.
	obj, err := hcl.Parse(s)
	if err != nil {
		return nil, err
	}

	// Start building the result, for now its our intermediate struct
	var result NodeConfig
	if err := hcl.DecodeObject(&result, obj); err != nil {
		return nil, err
	}

	return ParseNode(&result), nil
}

// LoadNodeConfigFile loads the node configuration from the given file path.
func LoadNodeFile(path string) (*structs.Node, error) {
	// Read the file
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Up to this point, the file is read as a string, we need to parse
	// it and return the resulting struct.
	return LoadNodeString(string(c))
}
