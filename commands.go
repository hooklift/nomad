package main

import (
	"os"

	"github.com/hashicorp/nomad/command"
	"github.com/hashicorp/nomad/command/agent"
	"github.com/hashicorp/nomad/command/simulator"
	"github.com/mitchellh/cli"
)

// Commands returns the mapping of CLI commands for Nomad. The meta
// parameter lets you set meta options for all commands.
func Commands(metaPtr *command.Meta) map[string]cli.CommandFactory {
	if metaPtr == nil {
		metaPtr = new(command.Meta)
	}

	meta := *metaPtr
	if meta.Ui == nil {
		meta.Ui = &cli.BasicUi{
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		}
	}

	return map[string]cli.CommandFactory{
		"alloc-status": func() (cli.Command, error) {
			return &command.AllocStatusCommand{
				Meta: meta,
			}, nil
		},

		"agent": func() (cli.Command, error) {
			return &agent.Command{
				Revision:          GitCommit,
				Version:           Version,
				VersionPrerelease: VersionPrerelease,
				Ui:                meta.Ui,
				ShutdownCh:        make(chan struct{}),
			}, nil
		},

		"agent-info": func() (cli.Command, error) {
			return &command.AgentInfoCommand{
				Meta: meta,
			}, nil
		},

		"client-config": func() (cli.Command, error) {
			return &command.ClientConfigCommand{
				Meta: meta,
			}, nil
		},

		"eval-monitor": func() (cli.Command, error) {
			return &command.EvalMonitorCommand{
				Meta: meta,
			}, nil
		},
		"executor": func() (cli.Command, error) {
			return &command.ExecutorPluginCommand{
				Meta: meta,
			}, nil
		},
		"fs": func() (cli.Command, error) {
			return &command.FSCommand{
				Meta: meta,
			}, nil
		},
		"fs ls": func() (cli.Command, error) {
			return &command.FSListCommand{
				Meta: meta,
			}, nil
		},
		"fs stat": func() (cli.Command, error) {
			return &command.FSStatCommand{
				Meta: meta,
			}, nil
		},
		"fs cat": func() (cli.Command, error) {
			return &command.FSCatCommand{
				Meta: meta,
			}, nil
		},
		"init": func() (cli.Command, error) {
			return &command.InitCommand{
				Meta: meta,
			}, nil
		},

		"node-drain": func() (cli.Command, error) {
			return &command.NodeDrainCommand{
				Meta: meta,
			}, nil
		},

		"node-status": func() (cli.Command, error) {
			return &command.NodeStatusCommand{
				Meta: meta,
			}, nil
		},

		"run": func() (cli.Command, error) {
			return &command.RunCommand{
				Meta: meta,
			}, nil
		},
		"syslog": func() (cli.Command, error) {
			return &command.SyslogPluginCommand{
				Meta: meta,
			}, nil
		},
		"server-force-leave": func() (cli.Command, error) {
			return &command.ServerForceLeaveCommand{
				Meta: meta,
			}, nil
		},

		"server-join": func() (cli.Command, error) {
			return &command.ServerJoinCommand{
				Meta: meta,
			}, nil
		},

		"server-members": func() (cli.Command, error) {
			return &command.ServerMembersCommand{
				Meta: meta,
			}, nil
		},

		"simulator": func() (cli.Command, error) {
			return &simulator.SimCommand{
				Meta: meta,
			}, nil
		},

		"status": func() (cli.Command, error) {
			return &command.StatusCommand{
				Meta: meta,
			}, nil
		},

		"stop": func() (cli.Command, error) {
			return &command.StopCommand{
				Meta: meta,
			}, nil
		},

		"validate": func() (cli.Command, error) {
			return &command.ValidateCommand{
				Meta: meta,
			}, nil
		},

		"version": func() (cli.Command, error) {
			ver := Version
			rel := VersionPrerelease
			if GitDescribe != "" {
				ver = GitDescribe
				// Trim off a leading 'v', we append it anyways.
				if ver[0] == 'v' {
					ver = ver[1:]
				}
			}
			if GitDescribe == "" && rel == "" && VersionPrerelease != "" {
				rel = "dev"
			}

			return &command.VersionCommand{
				Revision:          GitCommit,
				Version:           ver,
				VersionPrerelease: rel,
				Ui:                meta.Ui,
			}, nil
		},
	}
}
