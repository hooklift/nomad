package simulator

import (
	"io/ioutil"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/nomad/nomad/structs"
)

// NodeConfig is the struct to parse a node configuration data into.
// We could do without defining this if we put annotations on the original
// Node struct, but we don't know if thats a good idea.
type NodeConfig struct {
	// Datacenter for this node
	Datacenter string `hcl:"datacenter"`

	// Node name
	Name string `hcl:"name"`

	// Attributes is an arbitrary set of key/value
	// data that can be used for constraints. Examples
	// include "kernel.name=linux", "arch=386", "driver.docker=1",
	// "docker.runtime=1.8.3"
	Attributes map[string]string `hcl:"attributes"`

	// Resources is the available resources on the client.
	// For example 'cpu=2' 'memory=2048'
	Resources *NodeResources `hcl:"resources"`

	// Reserved is the set of resources that are reserved,
	// and should be subtracted from the total resources for
	// the purposes of scheduling. This may be provide certain
	// high-watermark tolerances or because of external schedulers
	// consuming resources.
	Reserved *NodeResources `hcl:"reserved"`

	// Links are used to 'link' this client to external
	// systems. For example 'consul=foo.dc1' 'aws=i-83212'
	// 'ami=ami-123'
	Links map[string]string `hcl:"links"`

	// Meta is used to associate arbitrary metadata with this
	// client. This is opaque to Nomad.
	Meta map[string]string `hcl:"meta"`

	// NodeClass is an opaque identifier used to group nodes
	// together for the purpose of determining scheduling pressure.
	NodeClass string `hcl:"node_class"`

	// Drain is controlled by the servers, and not the client.
	// If true, no jobs will be scheduled to this node, and existing
	// allocations will be drained.
	Drain bool `hcl:"drain"`

	// Status of this node
	Status string `hcl:"status"`

	// StatusDescription is meant to provide more human useful information
	StatusDescription string `hcl:"status_description"`

	// Raft Indexes
	CreateIndex uint64 `hcl:"create_index"`
	ModifyIndex uint64 `hcl:"modify_index"`
}

// Resources is used to define the resources available
// on a client
type NodeResources struct {
	CPU      int `hcl:"cpu"`
	MemoryMB int `hcl:"memory_mb"`
	DiskMB   int `hcl:"disk_mb"`
	IOPS     int `hcl:"iops"`
	Networks []*structs.NetworkResource
}

// Get the node struct from the node configuration file.
func ParseNode(n *NodeConfig) *structs.Node {
	return &structs.Node{
		ID:                structs.GenerateUUID(),
		Datacenter:        n.Datacenter,
		Name:              n.Name,
		Attributes:        n.Attributes,
		Resources:         ParseResources(n.Resources),
		Reserved:          ParseReservedResources(n.Reserved),
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

// TODO: read the network resources from config file, or maybe not, since
// networks are irrelevant for in-memory simulation. Ask about this. In Unit Tests,
// all the mocked nodes have the same network resource configuration.
func ParseResources(r *NodeResources) *structs.Resources {
	return &structs.Resources{
		CPU:      r.CPU,
		MemoryMB: r.MemoryMB,
		DiskMB:   r.DiskMB,
		IOPS:     r.IOPS,
		Networks: []*structs.NetworkResource{
			&structs.NetworkResource{
				Device: "eth0",
				CIDR:   "192.168.0.100/32",
				MBits:  1000,
			},
		},
	}
}

// TODO: read the reserved network resources from config file, or maybe not, since
// networks are irrelevant for in-memory simulation. Ask about this. In Unit Tests,
// all the mocked nodes have the same network resource configuration.
func ParseReservedResources(r *NodeResources) *structs.Resources {
	return &structs.Resources{
		CPU:      r.CPU,
		MemoryMB: r.MemoryMB,
		DiskMB:   r.DiskMB,
		IOPS:     r.IOPS,
		Networks: []*structs.NetworkResource{
			&structs.NetworkResource{
				Device:        "eth0",
				IP:            "192.168.0.100",
				ReservedPorts: []structs.Port{{Label: "main", Value: 22}},
				MBits:         1,
			},
		},
	}
}

// LoadNodeString is used to parse a node config string.
func LoadNodeString(s string) (*structs.Node, error) {
	// Parse!
	obj, err := hcl.Parse(s)
	if err != nil {
		return nil, err
	}

	// Start building the result
	var result NodeConfig
	if err := hcl.DecodeObject(&result, obj); err != nil {
		return nil, err
	}

	return ParseNode(&result), nil
}

// LoadNodeConfigFile loads the node configuration from the given file.
func LoadNodeFile(path string) (*structs.Node, error) {
	// Read the file
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadNodeString(string(c))
}
