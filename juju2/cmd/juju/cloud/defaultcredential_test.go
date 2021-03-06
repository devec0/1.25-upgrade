// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/1.25-upgrade/juju2/cloud"
	"github.com/juju/1.25-upgrade/juju2/cmd/juju/cloud"
	"github.com/juju/1.25-upgrade/juju2/jujuclient"
	"github.com/juju/1.25-upgrade/juju2/testing"
)

type defaultCredentialSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&defaultCredentialSuite{})

func (s *defaultCredentialSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju set-default-credential <cloud-name> <credential-name>")
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultCredentialSuite) TestBadCredential(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `credential "foo" for cloud aws not valid`)
}

func (s *defaultCredentialSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultCredentialSuite) assertSetDefaultCredential(c *gc.C, cloudName string) {
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"my-sekrets": {},
		},
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName, "my-sekrets")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default credential for %s set to "my-sekrets".`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, gc.Equals, "my-sekrets")
}

func (s *defaultCredentialSuite) TestSetDefaultCredential(c *gc.C) {
	s.assertSetDefaultCredential(c, "aws")
}

func (s *defaultCredentialSuite) TestSetDefaultCredentialBuiltIn(c *gc.C) {
	s.assertSetDefaultCredential(c, "localhost")
}
