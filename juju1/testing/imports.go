// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

const jujuPkgPrefix = "github.com/juju/1.25-upgrade/juju1/"

// FindJujuCoreImports returns a sorted list of juju-core packages that are
// imported by the packageName parameter.  The resulting list removes the
// common prefix "github.com/juju/1.25-upgrade/juju1/" leaving just the short names.
func FindJujuCoreImports(c *gc.C, packageName string) []string {
	imps, err := testing.FindImports(packageName, jujuPkgPrefix)
	c.Assert(err, jc.ErrorIsNil)
	return imps
}
