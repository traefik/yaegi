// Package msgstaticplug names (*net.UnixConn).WriteMsgUnix, whose oob
// argument can hand the descriptors of the host to the peer process.
package msgstaticplug

import "net"

func Send(c *net.UnixConn, oob []byte) error {
	_, _, err := c.WriteMsgUnix([]byte("x"), oob, nil)
	return err
}
