// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/1.25-upgrade/juju2/agent"
	apiagent "github.com/juju/1.25-upgrade/juju2/api/agent"
	"github.com/juju/1.25-upgrade/juju2/api/agenttools"
	"github.com/juju/1.25-upgrade/juju2/api/base"
	"github.com/juju/1.25-upgrade/juju2/cmd/jujud/agent/engine"
	"github.com/juju/1.25-upgrade/juju2/state/multiwatcher"
	"github.com/juju/1.25-upgrade/juju2/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig engine.AgentAPIManifoldConfig

// Manifold returns a dependency manifold that runs a toolsversionchecker worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig(config)
	return engine.AgentAPIManifold(typedConfig, newWorker)
}

func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	st := apiagent.NewState(apiCaller)
	isMM, err := isModelManager(a, st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isMM {
		return nil, dependency.ErrMissing
	}

	// 4 times a day seems a decent enough amount of checks.
	checkerParams := VersionCheckerParams{
		CheckInterval: time.Hour * 6,
	}
	return New(agenttools.NewFacade(apiCaller), &checkerParams), nil
}

func isModelManager(a agent.Agent, st *apiagent.State) (bool, error) {
	cfg := a.CurrentConfig()

	// Grab the tag and ensure that it's for a machine.
	tag, ok := cfg.Tag().(names.MachineTag)
	if !ok {
		return false, errors.New("this manifold may only be used inside a machine agent")
	}

	entity, err := st.Entity(tag)
	if err != nil {
		return false, err
	}

	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageModel {
			return true, nil
		}
	}

	return false, nil
}
