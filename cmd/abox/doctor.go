package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/doctor"
	"github.com/shoe-point-0/agent-box/internal/identity"
)

type doctorOptions struct {
	mode    doctor.Mode
	taskKey string
	json    bool
	help    bool
}

func parseDoctor(args []string) (doctorOptions, *contract.Failure) {
	options := doctorOptions{mode: doctor.ModeMachine}
	positionals := make([]string, 0, 2)
	for _, argument := range args {
		switch {
		case argument == "--json":
			if options.json {
				return options, failure(contract.CodeCLIUsage, "--json may be specified only once")
			}
			options.json = true
		case argument == "--help" || argument == "-h":
			if options.help {
				return options, failure(contract.CodeCLIUsage, "--help may be specified only once")
			}
			options.help = true
		case strings.HasPrefix(argument, "-"):
			return options, failure(contract.CodeCLIUsage, "unknown doctor option")
		default:
			positionals = append(positionals, argument)
		}
	}
	if options.help {
		if options.json || len(positionals) != 0 {
			return options, failure(contract.CodeCLIUsage, "--help cannot be combined with doctor arguments")
		}
		return options, nil
	}
	switch len(positionals) {
	case 0:
		return options, nil
	case 1:
		if positionals[0] == "fs" {
			return options, failure(contract.CodeCLIUsage, "doctor fs requires one task key")
		}
		options.mode, options.taskKey = doctor.ModeBox, positionals[0]
	case 2:
		if positionals[0] != "fs" {
			return options, failure(contract.CodeCLIUsage, "doctor accepts at most one task key")
		}
		options.mode, options.taskKey = doctor.ModeFilesystem, positionals[1]
	default:
		return options, failure(contract.CodeCLIUsage, "doctor accepts at most one mode and task key")
	}
	if err := identity.ValidateTaskKey(options.taskKey); err != nil {
		return options, failure(contract.CodeCLIUsage, "doctor requires a valid task key")
	}
	return options, nil
}

func renderDoctor(writer io.Writer, report doctor.Report) error {
	heading := "Machine prerequisites"
	switch report.Mode {
	case doctor.ModeBox:
		heading = "Box prerequisites: " + report.TaskKey
	case doctor.ModeFilesystem:
		heading = "Box filesystem prerequisites: " + report.TaskKey
	}
	if _, err := fmt.Fprintln(writer, heading); err != nil {
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "RESULT\tEVIDENCE\tSTATUS\tOBSERVED\tREQUIRED\tREMEDIATION"); err != nil {
		return err
	}
	for _, check := range report.Checks {
		remediation := check.Remediation
		if remediation == "" {
			remediation = "-"
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", check.Result, check.Evidence, check.Status, check.Observed, check.Required, remediation); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeDoctorHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage:
  abox doctor [--json]
  abox doctor [--json] TASK
  abox doctor fs [--json] TASK

Modes:
  machine      inspect Machine prerequisites (default)
  box          inspect Machine and Box-specific evidence
  filesystem   inspect retained Box filesystem and watcher evidence

Doctor observes existing state only. It never starts aboxd, binds a port,
provisions or repairs resources, or creates filesystem probe files.
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
