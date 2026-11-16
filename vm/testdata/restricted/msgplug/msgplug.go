// Package msgplug sends a socket control message on a unix socket that it
// dials itself without naming the method (an interface of the plugin, its
// reflect; the named call is refused at load time, msgstaticplug): the
// WriteMsg methods must be refused, and the ReadMsg methods must deliver
// no control message.
package msgplug

import (
	"net"
	"reflect"
)

type writer interface {
	WriteMsgUnix(b, oob []byte, addr *net.UnixAddr) (int, int, error)
}

// Write sends a non-empty oob to the unix socket at path, by the way named
// by how: "interface" or "reflect". It returns the error text.
func Write(path, how string) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = r.(error).Error()
		}
	}()
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err.Error()
	}
	defer c.Close()
	oob := make([]byte, 16)
	switch how {
	case "interface":
		_, _, err = writer(c).WriteMsgUnix([]byte("x"), oob, nil)
	case "reflect":
		out := reflect.ValueOf(c).MethodByName("WriteMsgUnix").Call([]reflect.Value{
			reflect.ValueOf([]byte("x")), reflect.ValueOf(oob), reflect.Zero(reflect.TypeOf((*net.UnixAddr)(nil))),
		})
		err, _ = out[2].Interface().(error)
	}
	if err != nil {
		return err.Error()
	}
	return "sent"
}

// Read receives one message from the unix socket at path with an oob
// buffer, and returns the number of oob bytes delivered.
func Read(path string) (int, error) {
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return 0, err
	}
	defer c.Close()
	if _, err := c.Write([]byte("hello")); err != nil {
		return 0, err
	}
	b, oob := make([]byte, 16), make([]byte, 64)
	_, oobn, _, _, err := c.ReadMsgUnix(b, oob)
	return oobn, err
}
