// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net"

	"github.com/juju/1.25-upgrade/juju2/rpc"
	"github.com/juju/1.25-upgrade/juju2/rpc/jsoncodec"
)

// FakeAPIServer returns a net.Conn implementation
// that serves the RPC server defined by the given
// root object (see rpc.Conn.Serve).
func FakeAPIServer(root interface{}) net.Conn {
	c0, c1 := net.Pipe()
	serverCodec := jsoncodec.NewNet(c1)
	serverRPC := rpc.NewConn(serverCodec, nil)
	serverRPC.Serve(root, nil)
	serverRPC.Start()
	go func() {
		<-serverRPC.Dead()
		serverRPC.Close()
	}()
	return c0
}
