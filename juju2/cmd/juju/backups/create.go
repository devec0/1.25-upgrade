// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/1.25-upgrade/juju2/cmd/modelcmd"
	"github.com/juju/1.25-upgrade/juju2/state/backups"
)

const (
	notset          = backups.FilenamePrefix + "<date>-<time>.tar.gz"
	downloadWarning = "WARNING: downloading backup archives is recommended; " +
		"backups stored remotely are not guaranteed to be available"
)

const createDoc = `
create-backup requests that juju create a backup of its state and print the
backup's unique ID.  You may provide a note to associate with the backup.

The backup archive and associated metadata are stored remotely by juju.

The --download option may be used without the --filename option.  In
that case, the backup archive will be stored in the current working
directory with a name matching juju-backup-<date>-<time>.tar.gz.

WARNING: Remotely stored backups will be lost when the model is
destroyed.  Furthermore, the remotely backup is not guaranteed to be
available.

Therefore, you should use the --download or --filename options, or use:

    juju download-backups

to get a local copy of the backup archive.
This local copy can then be used to restore an model even if that
model was already destroyed or is otherwise unavailable.
`

// NewCreateCommand returns a command used to create backups.
func NewCreateCommand() cmd.Command {
	return modelcmd.Wrap(&createCommand{})
}

// createCommand is the sub-command for creating a new backup.
type createCommand struct {
	CommandBase
	// NoDownload means the backups archive should not be downloaded.
	NoDownload bool
	// Filename is where the backup should be downloaded.
	Filename string
	// Notes is the custom message to associated with the new backup.
	Notes string
}

// Info implements Command.Info.
func (c *createCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-backup",
		Args:    "[<notes>]",
		Purpose: "Create a backup.",
		Doc:     createDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.NoDownload, "no-download", false, "Do not download the archive")
	f.StringVar(&c.Filename, "filename", notset, "Download to this file")
}

// Init implements Command.Init.
func (c *createCommand) Init(args []string) error {
	notes, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	c.Notes = notes

	if c.Filename != notset && c.NoDownload {
		return errors.Errorf("cannot mix --no-download and --filename")
	}
	if c.Filename == "" {
		return errors.Errorf("missing filename")
	}

	return nil
}

// Run implements Command.Run.
func (c *createCommand) Run(ctx *cmd.Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	result, err := client.Create(c.Notes)
	if err != nil {
		return errors.Trace(err)
	}

	if c.Log != nil && !c.Log.Quiet {
		if c.NoDownload {
			fmt.Fprintln(ctx.Stderr, downloadWarning)
		}
		c.dumpMetadata(ctx, result)
	}

	fmt.Fprintln(ctx.Stdout, result.ID)

	// Handle download.
	filename := c.decideFilename(ctx, c.Filename, result.Started)
	if filename != "" {
		if err := c.download(ctx, result.ID, filename); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *createCommand) decideFilename(ctx *cmd.Context, filename string, timestamp time.Time) string {
	if filename != notset {
		return filename
	}
	if c.NoDownload {
		return ""
	}

	// Downloading but no filename given, so generate one.
	return timestamp.Format(backups.FilenameTemplate)
}

func (c *createCommand) download(ctx *cmd.Context, id string, filename string) error {
	fmt.Fprintln(ctx.Stdout, "downloading to "+filename)

	// TODO(ericsnow) lp-1399722 This needs further investigation:
	// There is at least anecdotal evidence that we cannot use an API
	// client for more than a single request. So we use a new client
	// for download.
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	archive, err := client.Download(id)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.Close()

	outfile, err := os.Create(filename)
	if err != nil {
		return errors.Trace(err)
	}
	defer outfile.Close()

	_, err = io.Copy(outfile, archive)
	return errors.Trace(err)
}
