// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"path/filepath"

	"github.com/juju/utils/os"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/agent/tools"
)

// Paths represents the set of filesystem paths a uniter worker has reason to
// care about.
type Paths struct {

	// ToolsDir is the directory containing the jujud executable running this
	// process; and also containing jujuc tool symlinks to that executable. It's
	// the only path in this struct that is not typically pointing inside the
	// directory reserved for the exclusive use of this worker (typically
	// /var/lib/juju/agents/$UNIT_TAG/ )
	ToolsDir string

	// Runtime represents the set of paths that are relevant at runtime.
	Runtime RuntimePaths

	// State represents the set of paths that hold persistent local state for
	// the uniter.
	State StatePaths
}

// GetToolsDir exists to satisfy the context.Paths interface.
func (paths Paths) GetToolsDir() string {
	return paths.ToolsDir
}

// GetCharmDir exists to satisfy the context.Paths interface.
func (paths Paths) GetCharmDir() string {
	return paths.State.CharmDir
}

// GetJujucSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucSocket() string {
	return paths.Runtime.JujucServerSocket
}

// GetMetricsSpoolDir exists to satisfy the runner.Paths interface.
func (paths Paths) GetMetricsSpoolDir() string {
	return paths.State.MetricsSpoolDir
}

// ComponentDir returns the filesystem path to the directory
// containing all data files for a component.
func (paths Paths) ComponentDir(name string) string {
	return filepath.Join(paths.State.BaseDir, name)
}

// RuntimePaths represents the set of paths that are relevant at runtime.
type RuntimePaths struct {

	// JujuRunSocket listens for juju-run invocations, and is always
	// active.
	JujuRunSocket string

	// JujucServerSocket listens for jujuc invocations, and is only
	// active when supporting a jujuc execution context.
	JujucServerSocket string
}

// StatePaths represents the set of paths that hold persistent local state for
// the uniter.
type StatePaths struct {

	// BaseDir is the unit agent's base directory.
	BaseDir string

	// CharmDir is the directory to which the charm the uniter runs is deployed.
	CharmDir string

	// OperationsFile holds information about what the uniter is doing
	// and/or has done.
	OperationsFile string

	// RelationsDir holds relation-specific information about what the
	// uniter is doing and/or has done.
	RelationsDir string

	// BundlesDir holds downloaded charms.
	BundlesDir string

	// DeployerDir holds metadata about charms that are installing or have
	// been installed.
	DeployerDir string

	// StorageDir holds storage-specific information about what the
	// uniter is doing and/or has done.
	StorageDir string

	// MetricsSpoolDir acts as temporary storage for metrics being sent from
	// the uniter to state.
	MetricsSpoolDir string
}

// NewPaths returns the set of filesystem paths that the supplied unit should
// use, given the supplied root juju data directory path.
func NewPaths(dataDir string, unitTag names.UnitTag) Paths {
	return NewWorkerPaths(dataDir, unitTag, "")
}

// NewWorkerPaths returns the set of filesystem paths that the supplied unit worker should
// use, given the supplied root juju data directory path and worker identifier.
// Distinct worker identifiers ensure that runtime paths of different worker do not interfere.
func NewWorkerPaths(dataDir string, unitTag names.UnitTag, worker string) Paths {
	join := filepath.Join
	baseDir := join(dataDir, "agents", unitTag.String())
	stateDir := join(baseDir, "state")

	socket := func(name string, abstract bool) string {
		if os.HostOS() == os.Windows {
			base := fmt.Sprintf("%s", unitTag)
			if worker != "" {
				base = fmt.Sprintf("%s-%s", unitTag, worker)
			}
			return fmt.Sprintf(`\\.\pipe\%s-%s`, base, name)
		}
		path := join(baseDir, name+".socket")
		if worker != "" {
			path = join(baseDir, fmt.Sprintf("%s-%s.socket", worker, name))
		}
		if abstract {
			path = "@" + path
		}
		return path
	}

	toolsDir := tools.ToolsDir(dataDir, unitTag.String())
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		Runtime: RuntimePaths{
			JujuRunSocket:     socket("run", false),
			JujucServerSocket: socket("agent", true),
		},
		State: StatePaths{
			BaseDir:         baseDir,
			CharmDir:        join(baseDir, "charm"),
			OperationsFile:  join(stateDir, "uniter"),
			RelationsDir:    join(stateDir, "relations"),
			BundlesDir:      join(stateDir, "bundles"),
			DeployerDir:     join(stateDir, "deployer"),
			StorageDir:      join(stateDir, "storage"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}
