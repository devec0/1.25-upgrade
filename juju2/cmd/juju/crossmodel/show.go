// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/cmd/modelcmd"
	"github.com/juju/1.25-upgrade/juju2/core/crossmodel"
)

const showCommandDoc = `
Show extended information about an offered application.

This command is aimed for a user who wants to see more detail about what’s offered behind a particular URL.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (tabular|json|yaml)

Examples:
   $ juju show-endpoints fred/prod.db2

See also:
   find-endpoints
`

type showCommand struct {
	RemoteEndpointsCommandBase

	url        string
	out        cmd.Output
	newAPIFunc func() (ShowAPI, error)
}

// NewShowOfferedEndpointCommand constructs command that
// allows to show details of offered application's endpoint.
func NewShowOfferedEndpointCommand() cmd.Command {
	showCmd := &showCommand{}
	showCmd.newAPIFunc = func() (ShowAPI, error) {
		return showCmd.NewRemoteEndpointsAPI()
	}
	return modelcmd.WrapController(showCmd)
}

// Init implements Command.Init.
func (c *showCommand) Init(args []string) (err error) {
	if len(args) != 1 {
		return errors.New("must specify endpoint URL")
	}
	c.url = args[0]
	return nil
}

// Info implements Command.Info.
func (c *showCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-endpoints",
		Purpose: "Shows offered applications' endpoints details",
		Doc:     showCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showCommand) SetFlags(f *gnuflag.FlagSet) {
	c.RemoteEndpointsCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatShowTabular,
	})
}

// Run implements Command.Run.
func (c *showCommand) Run(ctx *cmd.Context) (err error) {
	if err := c.validateSource(); err != nil {
		return errors.Trace(err)
	}
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	found, err := api.ApplicationOffer(c.url)
	if err != nil {
		return err
	}

	output, err := convertOffers(found)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, output)
}

// validateSource checks that the offer is from the
// current controller.
func (c *showCommand) validateSource() error {
	url, err := crossmodel.ParseApplicationURL(c.url)
	if err != nil {
		return errors.Trace(err)
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	// For now we only support offers on the current controller.
	if url.Source != "" && url.Source != controllerName {
		return errors.NotSupportedf("showing endpoints from another controller %q", url.Source)
	}
	return nil
}

// ShowAPI defines the API methods that cross model show command uses.
type ShowAPI interface {
	Close() error
	ApplicationOffer(url string) (params.ApplicationOffer, error)
}

// ShowOfferedApplication defines the serialization behaviour of an application offer.
// This is used in map-style yaml output where remote application name is the key.
type ShowOfferedApplication struct {
	// Access is the level of access the user has on the offer.
	Access string `yaml:"access" json:"access"`

	// Endpoints list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Description is the user entered description.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// convertOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted offers.
func convertOffers(offers ...params.ApplicationOffer) (map[string]ShowOfferedApplication, error) {
	if len(offers) == 0 {
		return nil, nil
	}
	output := make(map[string]ShowOfferedApplication, len(offers))
	for _, one := range offers {
		app := ShowOfferedApplication{
			Access:    one.Access,
			Endpoints: convertRemoteEndpoints(one.Endpoints...),
		}
		if one.ApplicationDescription != "" {
			app.Description = one.ApplicationDescription
		}
		output[one.OfferURL] = app
	}
	return output, nil
}
