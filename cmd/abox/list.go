package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

type listOptions struct {
	json bool
	help bool
}

func parseList(args []string) (listOptions, *contract.Failure) {
	var options listOptions
	for _, argument := range args {
		switch argument {
		case "--json":
			if options.json {
				return options, failure(contract.CodeCLIUsage, "--json may be specified only once")
			}
			options.json = true
		case "--help", "-h":
			if options.help {
				return options, failure(contract.CodeCLIUsage, "--help may be specified only once")
			}
			options.help = true
		default:
			return options, failure(contract.CodeCLIUsage, "unknown list argument")
		}
	}
	if options.help && options.json {
		return options, failure(contract.CodeCLIUsage, "--help cannot be combined with list arguments")
	}
	return options, nil
}

func renderList(writer io.Writer, output contract.ListOutput) error {
	if len(output.Boxes) == 0 {
		_, err := fmt.Fprintln(writer, "No Boxes.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "BOX\tSTATE\tTASK\tWORKSPACE\tDRIFT"); err != nil {
		return err
	}
	for _, box := range output.Boxes {
		drift := "-"
		if len(box.Drift) > 0 {
			drift = strings.Join(box.Drift, "; ")
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", box.ID, box.State, box.TaskKey, box.WorkspaceID, drift); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeListHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox list [--json]

Options:
  --json  print the versioned Box inventory on stdout
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
