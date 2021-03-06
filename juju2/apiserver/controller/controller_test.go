// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/1.25-upgrade/juju2/apiserver"
	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/apiserver/controller"
	"github.com/juju/1.25-upgrade/juju2/apiserver/facade/facadetest"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	apiservertesting "github.com/juju/1.25-upgrade/juju2/apiserver/testing"
	"github.com/juju/1.25-upgrade/juju2/cloud"
	"github.com/juju/1.25-upgrade/juju2/environs"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/permission"
	"github.com/juju/1.25-upgrade/juju2/state"
	"github.com/juju/1.25-upgrade/juju2/state/multiwatcher"
	statetesting "github.com/juju/1.25-upgrade/juju2/state/testing"
	"github.com/juju/1.25-upgrade/juju2/testing"
	"github.com/juju/1.25-upgrade/juju2/testing/factory"
)

type controllerSuite struct {
	statetesting.StateSuite

	statePool  *state.StatePool
	controller *controller.ControllerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})

	s.StateSuite.SetUpTest(c)

	s.statePool = state.NewStatePool(s.State)
	s.AddCleanup(func(c *gc.C) {
		err := s.statePool.Close()
		c.Assert(err, jc.ErrorIsNil)
	})

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	controller, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			StatePool_: s.statePool,
			Resources_: s.resources,
			Auth_:      s.authorizer,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *controllerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
		})
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) checkEnvironmentMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	admin := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "owned", Owner: admin.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "user", Owner: remoteUserTag})
	defer st.Close()
	st.AddModelUser(st.ModelUUID(),
		state.UserAccessSpec{
			User:        admin.UserTag(),
			CreatedBy:   remoteUserTag,
			DisplayName: "Foo Bar",
			Access:      permission.ReadAccess})

	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "no-access", Owner: remoteUserTag}).Close()

	response, err := s.controller.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	// The results are sorted.
	expected := []string{"controller", "no-access", "owned", "user"}
	var obtained []string
	for _, env := range response.UserModels {
		obtained = append(obtained, env.Name)
		stateEnv, err := s.State.GetModel(names.NewModelTag(env.UUID))
		c.Assert(err, jc.ErrorIsNil)
		s.checkEnvironmentMatches(c, env.Model, stateEnv)
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *controllerSuite) TestHostedModelConfigs_OnlyHostedModelsReturned(c *gc.C) {
	owner := s.Factory.MakeUser(c, nil)
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: remoteUserTag}).Close()

	results, err := s.controller.HostedModelConfigs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Models), gc.Equals, 2)

	one := results.Models[0]
	two := results.Models[1]

	c.Assert(one.Name, gc.Equals, "first")
	c.Assert(one.OwnerTag, gc.Equals, owner.UserTag().String())
	c.Assert(two.Name, gc.Equals, "second")
	c.Assert(two.OwnerTag, gc.Equals, remoteUserTag.String())
}

func (s *controllerSuite) makeCloudSpec(c *gc.C, pSpec *params.CloudSpec) environs.CloudSpec {
	c.Assert(pSpec, gc.NotNil)
	var credential *cloud.Credential
	if pSpec.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(pSpec.Credential.AuthType),
			pSpec.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := environs.CloudSpec{
		Type:             pSpec.Type,
		Name:             pSpec.Name,
		Region:           pSpec.Region,
		Endpoint:         pSpec.Endpoint,
		IdentityEndpoint: pSpec.IdentityEndpoint,
		StorageEndpoint:  pSpec.StorageEndpoint,
		Credential:       credential,
	}
	c.Assert(spec.Validate(), jc.ErrorIsNil)
	return spec
}

func (s *controllerSuite) TestHostedModelConfigs_CanOpenEnviron(c *gc.C) {
	owner := s.Factory.MakeUser(c, nil)
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: remoteUserTag}).Close()

	results, err := s.controller.HostedModelConfigs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Models), gc.Equals, 2)

	for _, model := range results.Models {
		c.Assert(model.Error, gc.IsNil)

		cfg, err := config.New(config.NoDefaults, model.Config)
		c.Assert(err, jc.ErrorIsNil)
		spec := s.makeCloudSpec(c, model.CloudSpec)
		_, err = environs.New(environs.OpenParams{
			Cloud:  spec,
			Config: cfg,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *controllerSuite) TestListBlockedModels(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(list.Models, jc.DeepEquals, []params.ModelBlockInfo{
		params.ModelBlockInfo{
			Name:     "controller",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.Owner.String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
		params.ModelBlockInfo{
			Name:     "test",
			UUID:     st.ModelUUID(),
			OwnerTag: s.Owner.String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	})

}

func (s *controllerSuite) TestListBlockedModelsNoBlocks(c *gc.C) {
	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.Models, gc.HasLen, 0)
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	env, err := s.controller.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], jc.DeepEquals, params.ConfigValue{Value: "controller"})
}

func (s *controllerSuite) TestModelConfigFromNonController(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
	controller, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     st,
			Resources_: common.NewResources(),
			Auth_:      authorizer,
		})

	c.Assert(err, jc.ErrorIsNil)
	cfg, err := controller.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["name"], jc.DeepEquals, params.ConfigValue{Value: "controller"})
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.controller.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	cfgFromDB, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestControllerConfigFromNonController(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.Owner}
	controller, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     st,
			Resources_: common.NewResources(),
			Auth_:      authorizer,
		})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := controller.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	cfgFromDB, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestRemoveBlocks(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{All: true})
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *controllerSuite) TestRemoveBlocksNotAll(c *gc.C) {
	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{})
	c.Assert(err, gc.ErrorMatches, "not supported")
}

func (s *controllerSuite) TestWatchAllModels(c *gc.C) {
	watcherId, err := s.controller.WatchAllModels()
	c.Assert(err, jc.ErrorIsNil)

	var disposed bool
	watcherAPI_, err := apiserver.NewAllWatcher(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
		ID_:        watcherId.AllWatcherId,
		Dispose_:   func() { disposed = true },
	})
	c.Assert(err, jc.ErrorIsNil)
	watcherAPI := watcherAPI_.(*apiserver.SrvAllWatcher)
	defer func() {
		err := watcherAPI.Stop()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(disposed, jc.IsTrue)
	}()

	resultC := make(chan params.AllWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next()
		c.Assert(err, jc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		deltas := result.Deltas
		c.Assert(deltas, gc.HasLen, 1)
		envInfo := deltas[0].Entity.(*multiwatcher.ModelInfo)
		c.Assert(envInfo.ModelUUID, gc.Equals, s.State.ModelUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestInitiateMigration(c *gc.C) {
	// Create two hosted models to migrate.
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	mac, err := macaroon.New([]byte("secret"), "id", "location")
	c.Assert(err, jc.ErrorIsNil)
	macsJSON, err := json.Marshal([]macaroon.Slice{{mac}})
	c.Assert(err, jc.ErrorIsNil)

	controller.SetPrecheckResult(s, nil)

	// Kick off migrations
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: st1.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert1",
					AuthTag:       names.NewUserTag("admin1").String(),
					Password:      "secret1",
				},
			}, {
				ModelTag: st2.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"3.3.3.3:3333"},
					CACert:        "cert2",
					AuthTag:       names.NewUserTag("admin2").String(),
					Macaroons:     string(macsJSON),
					Password:      "secret2",
				},
			},
		},
	}
	out, err := s.controller.InitiateMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	states := []*state.State{st1, st2}
	for i, spec := range args.Specs {
		c.Log(i)
		st := states[i]
		result := out.Results[i]

		c.Assert(result.Error, gc.IsNil)
		c.Check(result.ModelTag, gc.Equals, spec.ModelTag)
		expectedId := st.ModelUUID() + ":0"
		c.Check(result.MigrationId, gc.Equals, expectedId)

		// Ensure the migration made it into the DB correctly.
		mig, err := st.LatestMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(mig.Id(), gc.Equals, expectedId)
		c.Check(mig.ModelUUID(), gc.Equals, st.ModelUUID())
		c.Check(mig.InitiatedBy(), gc.Equals, s.Owner.Id())

		targetInfo, err := mig.TargetInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(targetInfo.ControllerTag.String(), gc.Equals, spec.TargetInfo.ControllerTag)
		c.Check(targetInfo.Addrs, jc.SameContents, spec.TargetInfo.Addrs)
		c.Check(targetInfo.CACert, gc.Equals, spec.TargetInfo.CACert)
		c.Check(targetInfo.AuthTag.String(), gc.Equals, spec.TargetInfo.AuthTag)
		c.Check(targetInfo.Password, gc.Equals, spec.TargetInfo.Password)

		if spec.TargetInfo.Macaroons != "" {
			macJSONdb, err := json.Marshal(targetInfo.Macaroons)
			c.Assert(err, jc.ErrorIsNil)
			c.Check(string(macJSONdb), gc.Equals, spec.TargetInfo.Macaroons)
		}
	}
}

func (s *controllerSuite) TestInitiateMigrationSpecError(c *gc.C) {
	// Create a hosted model to migrate.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	// Kick off the migration with missing details.
	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: st.ModelTag().String(),
			// TargetInfo missing
		}},
	}
	out, err := s.controller.InitiateMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, gc.Equals, args.Specs[0].ModelTag)
	c.Check(result.MigrationId, gc.Equals, "")
	c.Check(result.Error, gc.ErrorMatches, "controller tag: .+ is not a valid tag")
}

func (s *controllerSuite) TestInitiateMigrationPartialFailure(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	controller.SetPrecheckResult(s, nil)

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: st.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
				},
			}, {
				ModelTag: randomModelTag(), // Doesn't exist.
			},
		},
	}
	out, err := s.controller.InitiateMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	c.Check(out.Results[0].ModelTag, gc.Equals, st.ModelTag().String())
	c.Check(out.Results[0].Error, gc.IsNil)

	c.Check(out.Results[1].ModelTag, gc.Equals, args.Specs[1].ModelTag)
	c.Check(out.Results[1].Error, gc.ErrorMatches, "unable to read model: .+")
}

func (s *controllerSuite) TestInitiateMigrationInvalidMacaroons(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{
			{
				ModelTag: st.ModelTag().String(),
				TargetInfo: params.MigrationTargetInfo{
					ControllerTag: randomControllerTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Macaroons:     "BLAH",
				},
			},
		},
	}
	out, err := s.controller.InitiateMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, gc.Equals, args.Specs[0].ModelTag)
	c.Check(result.Error, gc.ErrorMatches, "invalid macaroons: .+")
}

func (s *controllerSuite) TestInitiateMigrationPrecheckFail(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	controller.SetPrecheckResult(s, errors.New("boom"))

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: st.ModelTag().String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: randomControllerTag(),
				Addrs:         []string{"1.1.1.1:1111"},
				CACert:        "cert1",
				AuthTag:       names.NewUserTag("admin1").String(),
				Password:      "secret1",
			},
		}},
	}
	out, err := s.controller.InitiateMigration(args)
	c.Assert(out.Results, gc.HasLen, 1)
	c.Check(out.Results[0].Error, gc.ErrorMatches, "boom")

	active, err := st.IsMigrationActive()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(active, jc.IsFalse)
}

func randomControllerTag() string {
	uuid := utils.MustNewUUID().String()
	return names.NewControllerTag(uuid).String()
}

func randomModelTag() string {
	uuid := utils.MustNewUUID().String()
	return names.NewModelTag(uuid).String()
}

func (s *controllerSuite) modifyControllerAccess(c *gc.C, user names.UserTag, action params.ControllerAction, access string) error {
	args := params.ModifyControllerAccessRequest{
		Changes: []params.ModifyControllerAccess{{
			UserTag: user.String(),
			Action:  action,
			Access:  access,
		}}}
	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	return result.OneError()
}

func (s *controllerSuite) controllerGrant(c *gc.C, user names.UserTag, access string) error {
	return s.modifyControllerAccess(c, user, params.GrantControllerAccess, access)
}

func (s *controllerSuite) controllerRevoke(c *gc.C, user names.UserTag, access string) error {
	return s.modifyControllerAccess(c, user, params.RevokeControllerAccess, access)
}

func (s *controllerSuite) TestGrantMissingUserFails(c *gc.C) {
	user := names.NewLocalUserTag("foobar")
	err := s.controllerGrant(c, user, string(permission.AddModelAccess))
	expectedErr := `could not grant controller access: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestRevokeSuperuserLeavesAddModelAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(permission.SuperuserAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, permission.SuperuserAccess)

	err = s.controllerRevoke(c, user.UserTag(), string(permission.SuperuserAccess))
	c.Assert(err, gc.IsNil)

	controllerUser, err = s.State.UserAccess(user.UserTag(), controllerUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, permission.AddModelAccess)
}

func (s *controllerSuite) TestRevokeAddModelLeavesLoginAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(permission.AddModelAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, permission.AddModelAccess)

	err = s.controllerRevoke(c, user.UserTag(), string(permission.AddModelAccess))
	c.Assert(err, gc.IsNil)

	controllerUser, err = s.State.UserAccess(user.UserTag(), controllerUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, permission.LoginAccess)
}

func (s *controllerSuite) TestRevokeLoginRemovesControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.controllerRevoke(c, user.UserTag(), string(permission.LoginAccess))
	c.Assert(err, gc.IsNil)

	ctag := names.NewControllerTag(s.State.ControllerUUID())
	_, err = s.State.UserAccess(user.UserTag(), ctag)

	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *controllerSuite) TestRevokeControllerMissingUser(c *gc.C) {
	user := names.NewLocalUserTag("foobar")
	err := s.controllerRevoke(c, user, string(permission.AddModelAccess))
	expectedErr := `could not look up controller access for user: user "foobar" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(permission.AddModelAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, permission.AddModelAccess)

	err = s.controllerGrant(c, user.UserTag(), string(permission.AddModelAccess))
	expectedErr := `could not grant controller access: user already has "add-model" access or greater`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestGrantControllerAddRemoteUser(c *gc.C) {
	userTag := names.NewUserTag("foobar@ubuntuone")

	err := s.controllerGrant(c, userTag, string(permission.AddModelAccess))
	c.Assert(err, jc.ErrorIsNil)

	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerUser.Access, gc.Equals, permission.AddModelAccess)
}

func (s *controllerSuite) TestGrantControllerInvalidUserTag(c *gc.C) {
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user@local",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify controller access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyControllerAccessRequest{
			Changes: []params.ModifyControllerAccess{{
				UserTag: testParam.tag,
				Action:  params.GrantControllerAccess,
				Access:  string(permission.SuperuserAccess),
			}}}

		result, err := s.controller.ModifyControllerAccess(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	}
}

func (s *controllerSuite) TestModifyControllerAccessEmptyArgs(c *gc.C) {
	args := params.ModifyControllerAccessRequest{Changes: []params.ModifyControllerAccess{{}}}

	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `"" controller access not valid`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestModifyControllerAccessInvalidAction(c *gc.C) {
	var dance params.ControllerAction = "dance"
	args := params.ModifyControllerAccessRequest{
		Changes: []params.ModifyControllerAccess{{
			UserTag: "user-user@local",
			Action:  dance,
			Access:  string(permission.LoginAccess),
		}}}

	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestGetControllerAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	user2 := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(permission.SuperuserAccess))
	c.Assert(err, gc.IsNil)
	err = s.controllerGrant(c, user2.UserTag(), string(permission.AddModelAccess))
	c.Assert(err, gc.IsNil)
	req := params.Entities{
		Entities: []params.Entity{{Tag: user.Tag().String()}, {Tag: user2.Tag().String()}},
	}
	results, err := s.controller.GetControllerAccess(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.UserAccessResult{{
		Result: &params.UserAccess{
			Access:  "superuser",
			UserTag: user.Tag().String(),
		}}, {
		Result: &params.UserAccess{
			Access:  "add-model",
			UserTag: user2.Tag().String(),
		}}})
}

func (s *controllerSuite) TestGetControllerAccessPermissions(c *gc.C) {
	// Set up the user making the call.
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endpoint, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
		})
	c.Assert(err, jc.ErrorIsNil)
	args := params.ModifyControllerAccessRequest{
		Changes: []params.ModifyControllerAccess{{
			UserTag: user.Tag().String(),
			Action:  params.GrantControllerAccess,
			Access:  "superuser",
		}}}
	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)

	// We ask for permissions for a different user as well as ourselves.
	differentUser := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	req := params.Entities{
		Entities: []params.Entity{{Tag: user.Tag().String()}, {Tag: differentUser.Tag().String()}},
	}
	results, err := endpoint.GetControllerAccess(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(*results.Results[0].Result, jc.DeepEquals, params.UserAccess{
		Access:  "superuser",
		UserTag: user.Tag().String(),
	})
	c.Assert(*results.Results[1].Error, gc.DeepEquals, params.Error{
		Message: "permission denied", Code: "unauthorized access",
	})
}
