// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju1/agent"
	"github.com/juju/1.25-upgrade/juju1/worker"
	"github.com/juju/1.25-upgrade/juju1/worker/dependency"
	dt "github.com/juju/1.25-upgrade/juju1/worker/dependency/testing"
	"github.com/juju/1.25-upgrade/juju1/worker/util"
)

type AgentManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&AgentManifoldSuite{})

func (s *AgentManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = util.AgentManifold(util.AgentManifoldConfig{
		AgentName: "agent-name",
	}, s.newWorker)
}

func (s *AgentManifoldSuite) newWorker(a agent.Agent) (worker.Worker, error) {
	s.AddCall("newWorker", a)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *AgentManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name"})
}

func (s *AgentManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *AgentManifoldSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *AgentManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: expectAgent},
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}

func (s *AgentManifoldSuite) TestStartSuccess(c *gc.C) {
	expectAgent := &dummyAgent{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: expectAgent},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAgent},
	}})
}
