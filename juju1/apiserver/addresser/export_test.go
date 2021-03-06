// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/1.25-upgrade/juju1/state"
)

var NetEnvReleaseAddress = &netEnvReleaseAddress

type Patcher interface {
	PatchValue(ptr, value interface{})
}

func PatchState(p Patcher, st StateInterface) {
	p.PatchValue(&getState, func(*state.State) StateInterface {
		return st
	})
}
