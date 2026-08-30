package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
)

type planOptions struct {
	taskKey      string
	manifestPath string
	json         bool
	help         bool
}

func parsePlan(args []string) (planOptions, *contract.Failure) {
	var options planOptions
	for index := 0; index < len(args); index++ {
		argument := args[index]
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
		case argument == "--manifest":
			if options.manifestPath != "" || index+1 == len(args) {
				return options, failure(contract.CodeCLIUsage, "--manifest requires one path")
			}
			index++
			options.manifestPath = args[index]
		case strings.HasPrefix(argument, "--manifest=") && len(argument) > len("--manifest="):
			if options.manifestPath != "" {
				return options, failure(contract.CodeCLIUsage, "--manifest may be specified only once")
			}
			options.manifestPath = strings.TrimPrefix(argument, "--manifest=")
		case strings.HasPrefix(argument, "-"):
			return options, failure(contract.CodeCLIUsage, "unknown plan option")
		case options.taskKey == "":
			options.taskKey = argument
		default:
			return options, failure(contract.CodeCLIUsage, "plan accepts exactly one task key")
		}
	}
	if options.help {
		if options.taskKey != "" || options.manifestPath != "" || options.json {
			return options, failure(contract.CodeCLIUsage, "--help cannot be combined with plan arguments")
		}
		return options, nil
	}
	if err := identity.ValidateTaskKey(options.taskKey); err != nil {
		return options, failure(contract.CodeCLIUsage, "plan requires one valid task key")
	}
	return options, nil
}

func renderPlan(writer io.Writer, output contract.PlanOutput) error {
	if _, err := fmt.Fprintf(writer, "Box %s: %s (%s)\n", output.Plan.BoxID, output.Classification, output.Reason); err != nil {
		return err
	}
	if len(output.Operations) == 0 {
		_, err := fmt.Fprintln(writer, "No changes.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "ACTION\tRESOURCE\tIMPACT\tREASON"); err != nil {
		return err
	}
	for _, operation := range output.Operations {
		implications := make([]string, 0, 3)
		if operation.Restart {
			implications = append(implications, "restart")
		}
		if operation.Destructive {
			implications = append(implications, "destructive")
		}
		if operation.Conflict {
			implications = append(implications, "conflict")
		}
		impact := "none"
		if len(implications) != 0 {
			impact = strings.Join(implications, ",")
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", operation.Action, operation.Resource, impact, operation.Reason); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writePlanHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox plan [--json] [--manifest PATH] TASK

Options:
  --json           print the versioned proposed operations on stdout
  --manifest PATH  use this manifest instead of discovering .abox/box.yaml
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
