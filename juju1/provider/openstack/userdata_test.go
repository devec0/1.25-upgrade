// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju1/cloudconfig/providerinit/renderers"
	"github.com/juju/1.25-upgrade/juju1/provider/openstack"
	"github.com/juju/1.25-upgrade/juju1/testing"
	"github.com/juju/1.25-upgrade/juju1/version"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestOpenstackUnix(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, version.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(data))

	data = []byte("test")
	result, err = renderer.EncodeUserdata(data, version.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(data))
}

func (s *UserdataSuite) TestOpenstackWindows(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, version.Windows)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, renderers.WinEmbedInScript(data))
}

func (s *UserdataSuite) TestOpenstackUnknownOS(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	result, err := renderer.EncodeUserdata(nil, version.Arch)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Arch")
}
