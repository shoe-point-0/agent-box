package spike

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const (
	GitPathProbe    = "git-path"
	gitPathTreeName = "git-path-fixtures"
	gitTreeMarker   = ".abox-spike-tree-owner"
	gitProbeImage   = "golang:1.26"
)

type GitPathReport struct {
	SchemaVersion  string                 `json:"schemaVersion"`
	OS             string                 `json:"os"`
	Architecture   string                 `json:"architecture"`
	MachineGit     string                 `json:"machineGit"`
	ContainerGit   string                 `json:"containerGit"`
	ContainerImage string                 `json:"containerImage"`
	Fixtures       []GitPathFixtureResult `json:"fixtures"`
	RelativeMode   GitRelativeModeResult  `json:"relativeMode"`
}

type GitPathFixtureResult struct {
	Name                 string `json:"name"`
	SourcePath           string `json:"sourcePath"`
	WorktreePath         string `json:"worktreePath"`
	SourceViaSymlink     bool   `json:"sourceViaSymlink"`
	MachineHead          string `json:"machineHead"`
	ContainerHead        string `json:"containerHead"`
	SourceUnchanged      bool   `json:"sourceUnchanged"`
	NoAlternates         bool   `json:"noAlternates"`
	NoSharedObjects      bool   `json:"noSharedObjects"`
	WorktreeLocked       bool   `json:"worktreeLocked"`
	MachineStatusClean   bool   `json:"machineStatusClean"`
	ContainerStatusClean bool   `json:"containerStatusClean"`
}

type GitRelativeModeResult struct {
	MachineAdvertised   bool   `json:"machineAdvertised"`
	ContainerAdvertised bool   `json:"containerAdvertised"`
	Exercised           bool   `json:"exercised"`
	Reason              string `json:"reason,omitempty"`
}

type gitPathFixture struct {
	name          string
	directory     string
	symlinkSource bool
	relativeMode  bool
}

func RegisterGitPathProbe(registry *Registry) error {
	return registry.RegisterScenario(GitPathProbe, Scenario{
		Run:     runGitPathProbe,
		Cleanup: cleanupGitPathProbe,
	})
}

func runGitPathProbe(ctx context.Context, env *Environment) error {
	tree, err := createOwnedTree(env, gitPathTreeName)
	if err != nil {
		return err
	}
	machineVersion, err := runText(ctx, env, "machine-git-version", []string{"git", "--version"})
	if err != nil {
		return err
	}
	imageID, err := runText(ctx, env, "container-image-id", []string{"docker", "image", "inspect", "--format", "{{.Id}}", gitProbeImage})
	if err != nil {
		return fmt.Errorf("inspect locally cached Git probe image %s: %w", gitProbeImage, err)
	}
	imageID = strings.TrimSpace(imageID)
	containerVersion, err := runContainerGit(ctx, env, "container-git-version", imageID, tree, tree, []string{"--version"})
	if err != nil {
		return err
	}

	fixtures := []gitPathFixture{
		{name: "plain", directory: "plain"},
		{name: "spaces", directory: "space path"},
		{name: "unicode", directory: "unicodé-路径"},
		{name: "long-component", directory: strings.Repeat("long-path-", 20)},
		{name: "symlink-source", directory: "symlink-source", symlinkSource: true},
	}
	report := GitPathReport{
		SchemaVersion:  "abox.spike.git-path/v1",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		MachineGit:     strings.TrimSpace(machineVersion),
		ContainerGit:   strings.TrimSpace(containerVersion),
		ContainerImage: strings.TrimSpace(imageID),
		Fixtures:       make([]GitPathFixtureResult, 0, len(fixtures)+1),
	}
	for index, fixture := range fixtures {
		result, err := runGitFixture(ctx, env, tree, imageID, index, fixture)
		if err != nil {
			return fmt.Errorf("fixture %s: %w", fixture.name, err)
		}
		report.Fixtures = append(report.Fixtures, result)
	}

	machineConfig, err := runText(ctx, env, "machine-git-config-help", []string{"git", "help", "--config"})
	if err != nil {
		return err
	}
	containerConfig, err := runContainerGit(ctx, env, "container-git-config-help", imageID, tree, tree, []string{"help", "--config"})
	if err != nil {
		return err
	}
	report.RelativeMode.MachineAdvertised = strings.Contains(machineConfig, "worktree.useRelativePaths")
	report.RelativeMode.ContainerAdvertised = strings.Contains(containerConfig, "worktree.useRelativePaths")
	if report.RelativeMode.MachineAdvertised && report.RelativeMode.ContainerAdvertised {
		result, err := runGitFixture(ctx, env, tree, imageID, len(fixtures), gitPathFixture{
			name:         "relative-worktree",
			directory:    "relative-worktree",
			relativeMode: true,
		})
		if err != nil {
			return fmt.Errorf("relative-worktree fixture: %w", err)
		}
		report.RelativeMode.Exercised = true
		report.Fixtures = append(report.Fixtures, result)
	} else {
		report.RelativeMode.Reason = "worktree.useRelativePaths is not advertised by every Git client; manual pointer rewriting is forbidden"
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Git path report: %w", err)
	}
	data = append(data, '\n')
	if _, err := env.workspace.WriteDiagnostic("git-path-report.json", data); err != nil {
		return err
	}
	return nil
}

func runGitFixture(ctx context.Context, env *Environment, tree, imageID string, index int, fixture gitPathFixture) (GitPathFixtureResult, error) {
	base := filepath.Join(tree, fixture.directory)
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0o700); err != nil {
		return GitPathFixtureResult{}, fmt.Errorf("create source path: %w", err)
	}
	sourceArgument := source
	if fixture.symlinkSource {
		sourceArgument = filepath.Join(base, "source-link")
		if err := os.Symlink(source, sourceArgument); err != nil {
			return GitPathFixtureResult{}, fmt.Errorf("create source symlink: %w", err)
		}
	}
	prefix := fmt.Sprintf("fixture-%02d", index)
	if _, err := runText(ctx, env, prefix+"-init", []string{"git", "init", "--initial-branch=main", source}); err != nil {
		return GitPathFixtureResult{}, err
	}
	for configIndex, pair := range [][2]string{{"user.name", "Agent Box Probe"}, {"user.email", "probe@abox.invalid"}} {
		if _, err := runText(ctx, env, fmt.Sprintf("%s-config-%d", prefix, configIndex), []string{"git", "-C", sourceArgument, "config", pair[0], pair[1]}); err != nil {
			return GitPathFixtureResult{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte(fixture.name+"\n"), 0o600); err != nil {
		return GitPathFixtureResult{}, fmt.Errorf("write source fixture: %w", err)
	}
	if _, err := runText(ctx, env, prefix+"-source-add", []string{"git", "-C", sourceArgument, "add", "--", "README.md"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if _, err := runText(ctx, env, prefix+"-source-commit", []string{"git", "-C", sourceArgument, "commit", "-m", "seed fixture"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	before, err := snapshotTree(filepath.Join(source, ".git"))
	if err != nil {
		return GitPathFixtureResult{}, err
	}

	store := filepath.Join(base, "box.git")
	worktree := filepath.Join(base, "worktree")
	if _, err := runText(ctx, env, prefix+"-clone", []string{"git", "clone", "--bare", "--no-hardlinks", sourceArgument, store}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if _, err := runText(ctx, env, prefix+"-reflogs", []string{"git", "-C", store, "config", "core.logAllRefUpdates", "true"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if fixture.relativeMode {
		if _, err := runText(ctx, env, prefix+"-relative", []string{"git", "-C", store, "config", "worktree.useRelativePaths", "true"}); err != nil {
			return GitPathFixtureResult{}, err
		}
	}
	if _, err := runText(ctx, env, prefix+"-worktree-add", []string{"git", "-C", store, "worktree", "add", "-b", "abox/probe", worktree, "HEAD"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if _, err := runText(ctx, env, prefix+"-worktree-lock", []string{"git", "-C", store, "worktree", "lock", "--reason", "owned by Agent Box Git path probe", worktree}); err != nil {
		return GitPathFixtureResult{}, err
	}
	alternates := filepath.Join(store, "objects", "info", "alternates")
	noAlternates := false
	if _, err := os.Lstat(alternates); errors.Is(err, os.ErrNotExist) {
		noAlternates = true
	} else if err != nil {
		return GitPathFixtureResult{}, fmt.Errorf("inspect alternates: %w", err)
	}
	noSharedObjects, err := noSharedGitObjects(filepath.Join(source, ".git", "objects"), filepath.Join(store, "objects"))
	if err != nil {
		return GitPathFixtureResult{}, err
	}

	if err := os.WriteFile(filepath.Join(worktree, "machine.txt"), []byte("machine\n"), 0o600); err != nil {
		return GitPathFixtureResult{}, fmt.Errorf("write Machine fixture: %w", err)
	}
	if _, err := runText(ctx, env, prefix+"-machine-add", []string{"git", "-C", worktree, "add", "--", "machine.txt"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if _, err := runText(ctx, env, prefix+"-machine-commit", []string{"git", "-C", worktree, "-c", "user.name=Agent Box Probe", "-c", "user.email=probe@abox.invalid", "commit", "-m", "Machine commit"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if err := os.WriteFile(filepath.Join(worktree, "container.txt"), []byte("container\n"), 0o600); err != nil {
		return GitPathFixtureResult{}, fmt.Errorf("write container fixture: %w", err)
	}
	if _, err := runContainerGit(ctx, env, prefix+"-container-add", imageID, env.Root(), worktree, []string{"-c", "safe.directory=" + worktree, "add", "--", "container.txt"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	if _, err := runContainerGit(ctx, env, prefix+"-container-commit", imageID, env.Root(), worktree, []string{"-c", "safe.directory=" + worktree, "-c", "user.name=Agent Box Probe", "-c", "user.email=probe@abox.invalid", "commit", "-m", "container commit"}); err != nil {
		return GitPathFixtureResult{}, err
	}
	machineStatus, err := runText(ctx, env, prefix+"-machine-status", []string{"git", "-C", worktree, "status", "--porcelain=v2", "-z", "--untracked-files=all"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	containerStatus, err := runContainerGit(ctx, env, prefix+"-container-status", imageID, env.Root(), worktree, []string{"-c", "safe.directory=" + worktree, "status", "--porcelain=v2", "-z", "--untracked-files=all"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	machineHead, err := runText(ctx, env, prefix+"-machine-head", []string{"git", "-C", worktree, "rev-parse", "HEAD"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	containerHead, err := runContainerGit(ctx, env, prefix+"-container-head", imageID, env.Root(), worktree, []string{"-c", "safe.directory=" + worktree, "rev-parse", "HEAD"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	machineLog, err := runText(ctx, env, prefix+"-machine-log", []string{"git", "-C", worktree, "log", "-1", "--format=%H"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	containerLog, err := runContainerGit(ctx, env, prefix+"-container-log", imageID, env.Root(), worktree, []string{"-c", "safe.directory=" + worktree, "log", "-1", "--format=%H"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	listing, err := runText(ctx, env, prefix+"-worktree-list", []string{"git", "-C", store, "worktree", "list", "--porcelain", "-z"})
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	after, err := snapshotTree(filepath.Join(source, ".git"))
	if err != nil {
		return GitPathFixtureResult{}, err
	}
	machineHead = strings.TrimSpace(machineHead)
	containerHead = strings.TrimSpace(containerHead)
	machineLog = strings.TrimSpace(machineLog)
	containerLog = strings.TrimSpace(containerLog)
	result := GitPathFixtureResult{
		Name:                 fixture.name,
		SourcePath:           sourceArgument,
		WorktreePath:         worktree,
		SourceViaSymlink:     fixture.symlinkSource,
		MachineHead:          machineHead,
		ContainerHead:        containerHead,
		SourceUnchanged:      snapshotsEqual(before, after),
		NoAlternates:         noAlternates,
		NoSharedObjects:      noSharedObjects,
		WorktreeLocked:       strings.Contains(listing, worktree) && strings.Contains(listing, "locked"),
		MachineStatusClean:   machineStatus == "",
		ContainerStatusClean: containerStatus == "",
	}
	if machineHead == "" || machineHead != containerHead || machineLog != machineHead || containerLog != machineHead || !result.SourceUnchanged || !result.NoAlternates || !result.NoSharedObjects || !result.WorktreeLocked || !result.MachineStatusClean || !result.ContainerStatusClean {
		return result, fmt.Errorf("cross-boundary invariant failed: %+v", result)
	}
	if fixture.relativeMode {
		pointer, err := os.ReadFile(filepath.Join(worktree, ".git"))
		if err != nil {
			return result, fmt.Errorf("read relative worktree pointer: %w", err)
		}
		if filepath.IsAbs(strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))) {
			return result, errors.New("relative worktree mode produced an absolute .git pointer")
		}
	}
	return result, nil
}

func createOwnedTree(env *Environment, name string) (string, error) {
	path, err := env.workspace.resolveRelative(name)
	if err != nil {
		return "", err
	}
	resource := Resource{
		Kind:    "path-tree",
		ID:      path,
		OwnerID: env.workspace.metadata.ID,
		Attributes: map[string]string{
			"ownerMarker": gitTreeMarker,
		},
	}
	if _, err := env.workspace.inventory.Append("planned", resource); err != nil {
		return "", err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return "", fmt.Errorf("create Git fixture tree: %w", err)
	}
	if err := writeSyncedFile(filepath.Join(path, gitTreeMarker), []byte(env.workspace.metadata.ID+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write Git fixture ownership marker: %w", err)
	}
	if _, err := env.workspace.inventory.Append("created", resource); err != nil {
		return "", err
	}
	return path, nil
}

func cleanupGitPathProbe(_ context.Context, workspace *Workspace, resources []Resource) error {
	for _, resource := range resources {
		if resource.Kind != "path-tree" {
			continue
		}
		if !isWithin(workspace.Root(), resource.ID) || filepath.Base(resource.ID) != gitPathTreeName {
			return fmt.Errorf("%w: Git fixture tree path mismatch: %s", ErrCleanupRefused, resource.ID)
		}
		info, err := os.Lstat(resource.ID)
		if errors.Is(err, os.ErrNotExist) {
			if err := workspace.MarkRemoved(resource); err != nil {
				return err
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: Git fixture tree identity mismatch", ErrCleanupRefused)
		}
		marker, err := os.ReadFile(filepath.Join(resource.ID, gitTreeMarker))
		if err != nil || strings.TrimSpace(string(marker)) != workspace.metadata.ID {
			return fmt.Errorf("%w: Git fixture ownership marker mismatch", ErrCleanupRefused)
		}
		if err := preflightRemovableTree(resource.ID); err != nil {
			return err
		}
		if err := os.RemoveAll(resource.ID); err != nil {
			return fmt.Errorf("remove owned Git fixture tree: %w", err)
		}
		if err := workspace.MarkRemoved(resource); err != nil {
			return err
		}
	}
	return nil
}

func runText(ctx context.Context, env *Environment, label string, argv []string) (string, error) {
	result, err := env.RunCommand(ctx, label, Command{Argv: argv, Env: os.Environ()})
	if err != nil {
		return string(result.Stdout), err
	}
	return string(result.Stdout), nil
}

func runContainerGit(ctx context.Context, env *Environment, label, imageID, mountRoot, workdir string, gitArgs []string) (string, error) {
	argv := []string{
		"docker", "run", "--rm", "--network", "none",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "HOME=/tmp",
		"--mount", "type=bind,src=" + mountRoot + ",dst=" + mountRoot,
		"--workdir", workdir,
		imageID, "git",
	}
	argv = append(argv, gitArgs...)
	return runText(ctx, env, label, argv)
}

func preflightRemovableTree(root string) error {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("%w: inspect Git fixture root: %v", ErrCleanupRefused, err)
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: unsupported filesystem identity for Git fixture root", ErrCleanupRefused)
	}
	groups, err := os.Getgroups()
	if err != nil {
		return fmt.Errorf("%w: inspect Machine groups: %v", ErrCleanupRefused, err)
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%w: inspect %s: %v", ErrCleanupRefused, path, walkErr)
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("%w: inspect directory %s: %v", ErrCleanupRefused, path, err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Dev != rootStat.Dev {
			return fmt.Errorf("%w: directory crosses filesystem identity: %s", ErrCleanupRefused, path)
		}
		if !directoryWritableByCurrentUser(info.Mode().Perm(), int(stat.Uid), int(stat.Gid), groups) {
			return fmt.Errorf("%w: directory is not removable by uid %d: %s", ErrCleanupRefused, os.Getuid(), path)
		}
		return nil
	})
}

func directoryWritableByCurrentUser(mode fs.FileMode, owner, group int, groups []int) bool {
	if os.Geteuid() == 0 {
		return true
	}
	if owner == os.Geteuid() {
		return mode&0o300 == 0o300
	}
	if group == os.Getegid() || containsGroup(groups, group) {
		return mode&0o030 == 0o030
	}
	return mode&0o003 == 0o003
}

func containsGroup(groups []int, target int) bool {
	for _, group := range groups {
		if group == target {
			return true
		}
	}
	return false
}

type fileSnapshot struct {
	Mode fs.FileMode
	Hash string
}

func snapshotTree(root string) (map[string]fileSnapshot, error) {
	snapshot := make(map[string]fileSnapshot)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		snapshot[relative] = fileSnapshot{Mode: info.Mode(), Hash: hex.EncodeToString(digest[:])}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", root, err)
	}
	return snapshot, nil
}

func snapshotsEqual(left, right map[string]fileSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for path, value := range left {
		if right[path] != value {
			return false
		}
	}
	return true
}

func noSharedGitObjects(sourceObjects, boxObjects string) (bool, error) {
	sourceFiles, err := regularFileInfos(sourceObjects)
	if err != nil {
		return false, fmt.Errorf("inspect source Git objects: %w", err)
	}
	boxFiles, err := regularFileInfos(boxObjects)
	if err != nil {
		return false, fmt.Errorf("inspect Box Git objects: %w", err)
	}
	for _, sourceInfo := range sourceFiles {
		for _, boxInfo := range boxFiles {
			if os.SameFile(sourceInfo, boxInfo) {
				return false, nil
			}
		}
	}
	return true, nil
}

func regularFileInfos(root string) ([]fs.FileInfo, error) {
	files := make([]fs.FileInfo, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		files = append(files, info)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
