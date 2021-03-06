// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/apiserver/applicationoffers"
	jujucrossmodel "github.com/juju/1.25-upgrade/juju2/core/crossmodel"
	"github.com/juju/1.25-upgrade/juju2/permission"
	"github.com/juju/1.25-upgrade/juju2/testing"
)

const (
	offerCall       = "offerCall"
	addOfferCall    = "addOffersCall"
	listOffersCall  = "listOffersCall"
	updateOfferCall = "updateOfferCall"
	removeOfferCall = "removeOfferCall"
)

type mockApplicationOffers struct {
	jtesting.Stub
	jujucrossmodel.ApplicationOffers

	addOffer   func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error)
	listOffers func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error)
}

func (m *mockApplicationOffers) AddOffer(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(addOfferCall)
	return m.addOffer(offer)
}

func (m *mockApplicationOffers) ListOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

func (m *mockApplicationOffers) UpdateOffer(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(updateOfferCall)
	panic("not implemented")
}

func (m *mockApplicationOffers) Remove(url string) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

func (m *mockApplicationOffers) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCall)
	panic("not implemented")
}

type mockModel struct {
	uuid  string
	name  string
	owner string
}

func (m *mockModel) UUID() string {
	return m.uuid
}

func (m *mockModel) Name() string {
	return m.name
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag(m.owner)
}

type mockCharm struct {
	meta *charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return m.meta
}

type mockApplication struct {
	name  string
	charm *mockCharm
	curl  *charm.URL
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) Charm() (applicationoffers.Charm, bool, error) {
	return m.charm, true, nil
}

func (m *mockApplication) CharmURL() (curl *charm.URL, force bool) {
	return m.curl, true
}

type mockConnectionStatus struct {
	count int
}

func (m *mockConnectionStatus) ConnectionCount() int {
	return m.count
}

type offerAccess struct {
	user  names.UserTag
	offer names.ApplicationOfferTag
}

type mockState struct {
	modelUUID         string
	model             applicationoffers.Model
	allmodels         []applicationoffers.Model
	users             set.Strings
	applications      map[string]applicationoffers.Application
	applicationOffers map[string]jujucrossmodel.ApplicationOffer
	connStatus        applicationoffers.RemoteConnectionStatus
	accessPerms       map[offerAccess]permission.Access
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
}

func (m *mockState) Application(name string) (applicationoffers.Application, error) {
	app, ok := m.applications[name]
	if !ok {
		return nil, errors.NotFoundf("application %q", name)
	}
	return app, nil
}

func (m *mockState) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	offer, ok := m.applicationOffers[name]
	if !ok {
		return nil, errors.NotFoundf("application offer %q", name)
	}
	return &offer, nil
}

func (m *mockState) Model() (applicationoffers.Model, error) {
	return m.model, nil
}

func (m *mockState) ModelUUID() string {
	return m.modelUUID
}

func (m *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag(m.modelUUID)
}

func (m *mockState) AllModels() ([]applicationoffers.Model, error) {
	if len(m.allmodels) > 0 {
		return m.allmodels, nil
	}
	return []applicationoffers.Model{m.model}, nil
}

func (m *mockState) RemoteConnectionStatus(offerName string) (applicationoffers.RemoteConnectionStatus, error) {
	return m.connStatus, nil
}

func (m *mockState) GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error) {
	access, ok := m.accessPerms[offerAccess{user: user, offer: offer}]
	if !ok {
		return "", errors.NotFoundf("offer access for %v", user)
	}
	return access, nil
}

func (m *mockState) CreateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if !m.users.Contains(user.Name()) {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offer: offer}]; ok {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offer: offer}] = access
	return nil
}

func (m *mockState) UpdateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if !m.users.Contains(user.Name()) {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offer: offer}]; !ok {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offer: offer}] = access
	return nil
}

func (m *mockState) RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error {
	if !m.users.Contains(user.Name()) {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %q does not exist", user.Name()))
	}
	delete(m.accessPerms, offerAccess{user: user, offer: offer})
	return nil
}

type mockStatePool struct {
	st map[string]applicationoffers.Backend
}

func (st *mockStatePool) Get(modelUUID string) (applicationoffers.Backend, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	return backend, func() {}, nil
}
