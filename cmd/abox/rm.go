package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

type removeOptions struct {
	taskKey                  string
	manifestPath             string
	json                     bool
	yes                      bool
	discardWorktreeChanges   bool
	discardUnpublishedCommit bool
	deleteVolumes            bool
	help                     bool
}

func runRemove(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	options, parseFailure := parseRemove(args)
	if parseFailure != nil {
		return reportFailure(stdout, stderr, options.json, parseFailure)
	}
	if options.help {
		return writeRemoveHelp(stdout)
	}
	cwd, err := deps.getwd()
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve current directory"))
	}
	manifestPath, manifestDigest, manifestFailure := resolveManifest(cwd, options.manifestPath)
	if manifestFailure != nil {
		return reportFailure(stdout, stderr, options.json, manifestFailure)
	}
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeManifestInvalid, "manifest path is not canonical"))
	}
	boxID, err := identity.Box(workspaceID, options.taskKey)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeCLIUsage, "task key is invalid"))
	}
	userPaths, err := deps.userPaths()
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve user-scoped daemon paths"))
	}
	client, connectFailure := connect(ctx, userPaths, deps)
	if connectFailure != nil {
		return reportFailure(stdout, stderr, options.json, connectFailure)
	}
	defer client.Close()

	request := daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: options.taskKey,
		ManifestPath: manifestPath, ManifestDigest: manifestDigest,
	}
	preview, operationFailure, transportErr := requestRemovalPreview(ctx, client, request, stdout, stderr, options.json, deps)
	if transportErr != nil {
		return reportFailure(stdout, stderr, options.json, classifyTransport(transportErr))
	}
	if operationFailure == nil || operationFailure.Code != contract.CodeCleanupRefused {
		if operationFailure != nil {
			return reportFailure(stdout, stderr, options.json, operationFailure)
		}
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "removal preview completed without a refusal"))
	}
	if !options.yes {
		return reportFailure(stdout, stderr, options.json, operationFailure)
	}

	input, err := json.Marshal(removal.OperationInput{
		PreviewDigest: preview.Digest, Yes: true,
		DiscardWorktreeChanges:    options.discardWorktreeChanges,
		DiscardUnpublishedCommits: options.discardUnpublishedCommit,
		DeleteVolumes:             options.deleteVolumes,
	})
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "cannot encode removal authorization"))
	}
	request.Input = input
	key, err := newIdempotencyKey(deps.random)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "cannot create request identity"))
	}
	consume := removeEventConsumer(stdout, stderr, options.json, boxID.String(), nil)
	operationFailure, transportErr = client.Remove(ctx, request, key, consume)
	if transportErr != nil {
		return reportFailure(stdout, stderr, options.json, classifyTransport(transportErr))
	}
	if operationFailure != nil {
		return reportFailure(stdout, stderr, options.json, operationFailure)
	}
	if !options.json {
		if _, err := fmt.Fprintln(stderr, "abox: removal complete"); err != nil {
			return contract.ExitInternal
		}
	}
	return contract.ExitOK
}

func requestRemovalPreview(ctx context.Context, client daemonAPI, request daemon.MutationRequest, stdout, stderr io.Writer, jsonMode bool, deps dependencies) (removal.Preview, *contract.Failure, error) {
	key, err := newIdempotencyKey(deps.random)
	if err != nil {
		return removal.Preview{}, failure(contract.CodeInternalInvariant, "cannot create request identity"), nil
	}
	var preview removal.Preview
	consume := removeEventConsumer(stdout, stderr, jsonMode, request.BoxID, &preview)
	operationFailure, transportErr := client.Remove(ctx, request, key, consume)
	if transportErr != nil || operationFailure == nil || operationFailure.Code != contract.CodeCleanupRefused {
		return preview, operationFailure, transportErr
	}
	if err := validateRemovalPreview(preview, request.BoxID); err != nil {
		return removal.Preview{}, nil, fmt.Errorf("%w: invalid removal preview", daemonclient.ErrProtocol)
	}
	return preview, operationFailure, nil
}

func removeEventConsumer(stdout, stderr io.Writer, jsonMode bool, boxID string, captured *removal.Preview) func(daemonclient.Event) error {
	encoder := json.NewEncoder(stdout)
	return func(event daemonclient.Event) error {
		var preview removal.Preview
		if event.Kind == "removal.preview" {
			var err error
			preview, err = decodeRemovalPreview(event.Details)
			if err != nil || validateRemovalPreview(preview, boxID) != nil {
				return fmt.Errorf("%w: invalid removal preview", daemonclient.ErrProtocol)
			}
			if captured != nil {
				*captured = preview
			}
		}
		if jsonMode {
			if err := encoder.Encode(contract.Event{
				SchemaVersion: contract.EventSchemaVersion, OperationID: event.OperationID,
				Sequence: event.Sequence, Kind: event.Kind, Details: event.Details, OccurredAt: event.OccurredAt,
			}); err != nil {
				return err
			}
		}
		if event.Kind == "removal.preview" {
			if !jsonMode {
				return renderRemovalPreview(stdout, preview)
			}
			return nil
		}
		if !jsonMode && event.Kind != "operation.finished" {
			_, err := fmt.Fprintf(stderr, "abox: %s\n", event.Kind)
			return err
		}
		return nil
	}
}

func decodeRemovalPreview(data []byte) (removal.Preview, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var preview removal.Preview
	if err := decoder.Decode(&preview); err != nil {
		return removal.Preview{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return removal.Preview{}, errors.New("removal preview contains trailing data")
	}
	return preview, nil
}

func validateRemovalPreview(preview removal.Preview, boxID string) error {
	if preview.BoxID != boxID || len(preview.Digest) != len("sha256:")+64 || !strings.HasPrefix(preview.Digest, "sha256:") ||
		strings.Trim(preview.Digest[len("sha256:"):], "0123456789abcdef") != "" || preview.Resources == nil || preview.Losses == nil {
		return errors.New("removal preview envelope is invalid")
	}
	for _, loss := range preview.Losses {
		if loss.Value == "" || (loss.Kind != removal.LossWorktreeChange && loss.Kind != removal.LossUnpublishedCommit && loss.Kind != removal.LossVolume) {
			return errors.New("removal preview loss is invalid")
		}
	}
	return nil
}

func renderRemovalPreview(writer io.Writer, preview removal.Preview) error {
	if _, err := fmt.Fprintf(writer, "Box %s removal preview %s\n", preview.BoxID, preview.Digest); err != nil {
		return err
	}
	if len(preview.Losses) == 0 {
		_, err := fmt.Fprintln(writer, "No data losses.")
		return err
	}
	for _, loss := range preview.Losses {
		if _, err := fmt.Fprintf(writer, "LOSS	%s	%s\n", loss.Kind, strconv.Quote(loss.Value)); err != nil {
			return err
		}
	}
	return nil
}

func parseRemove(args []string) (removeOptions, *contract.Failure) {
	var options removeOptions
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			if options.json {
				return options, failure(contract.CodeCLIUsage, "--json may be specified only once")
			}
			options.json = true
		case argument == "--yes":
			if options.yes {
				return options, failure(contract.CodeCLIUsage, "--yes may be specified only once")
			}
			options.yes = true
		case argument == "--discard-worktree-changes":
			if options.discardWorktreeChanges {
				return options, failure(contract.CodeCLIUsage, "--discard-worktree-changes may be specified only once")
			}
			options.discardWorktreeChanges = true
		case argument == "--discard-unpublished-commits":
			if options.discardUnpublishedCommit {
				return options, failure(contract.CodeCLIUsage, "--discard-unpublished-commits may be specified only once")
			}
			options.discardUnpublishedCommit = true
		case argument == "--delete-volumes":
			if options.deleteVolumes {
				return options, failure(contract.CodeCLIUsage, "--delete-volumes may be specified only once")
			}
			options.deleteVolumes = true
		case argument == "--help" || argument == "-h":
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
			return options, failure(contract.CodeCLIUsage, "unknown rm option")
		case options.taskKey == "":
			options.taskKey = argument
		default:
			return options, failure(contract.CodeCLIUsage, "rm accepts exactly one task key")
		}
	}
	if options.help {
		if options.taskKey != "" || options.manifestPath != "" || options.json || options.yes || options.discardWorktreeChanges || options.discardUnpublishedCommit || options.deleteVolumes {
			return options, failure(contract.CodeCLIUsage, "--help cannot be combined with rm arguments")
		}
		return options, nil
	}
	if err := identity.ValidateTaskKey(options.taskKey); err != nil {
		return options, failure(contract.CodeCLIUsage, "rm requires one valid task key")
	}
	return options, nil
}

func writeRemoveHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox rm [--json] [--manifest PATH] [--yes] [DISCARD FLAGS] TASK

Removal always runs and prints a read-only loss preview first. Mutation requires
--yes plus the separate flag for every loss class present in that exact preview.

Options:
  --json                         stream versioned NDJSON events on stdout
  --manifest PATH                use this manifest instead of discovering .abox/box.yaml
  --yes                          confirm the exact preview digest
  --discard-worktree-changes     allow loss of listed dirty, untracked, and ignored files
  --discard-unpublished-commits  allow loss of listed unpublished commits
  --delete-volumes               allow deletion of listed Box-owned volumes
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
