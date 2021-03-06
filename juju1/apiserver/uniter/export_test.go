// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import "github.com/juju/1.25-upgrade/juju1/apiserver/common"

var (
	GetZone = &getZone
)

type StorageStateInterface storageStateInterface

func NewStorageAPI(
	st StorageStateInterface,
	resources *common.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(storageStateInterface(st), resources, accessUnit)
}
