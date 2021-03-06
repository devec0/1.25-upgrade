// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/1.25-upgrade/juju1/environs"
)

var (
	ProviderInstance = manualProvider{}
	NewSSHStorage    = &newSSHStorage
	InitUbuntuUser   = &initUbuntuUser
)

func EnvironUseSSHStorage(env environs.Environ) bool {
	e, ok := env.(*manualEnviron)
	if !ok {
		return false
	}
	return e.cfg.useSSHStorage()
}
