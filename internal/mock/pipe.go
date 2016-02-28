package mock

import (
	"errors"
	"net"
)

var (
	errClosed = errors.New("listener closed")
)

// listener returns a net.Listener that does not need permission
// to bind to a port or create a socket file. Useful for testing in
// heavily sandboxed environments or intra-process communication.
func listener() *pipeListener {
	return &pipeListener{
		incoming: make(chan net.Conn),
		shutdown: make(chan struct{}),
	}
}

type pipeListener struct {
	incoming chan net.Conn
	shutdown chan struct{}
}

// Accept accepts a new connection on a PipeListener.
// Accept blocks until a new connection is made or the
// PipeListener is closed.
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.incoming:
		return c, nil
	case <-l.shutdown:
		return nil, errClosed
	}
}

func (l *pipeListener) Dial() (net.Conn, error) {
	x, y := net.Pipe()
	select {
	case <-l.shutdown:
		x.Close()
		y.Close()
		return nil, errClosed
	case l.incoming <- x:
		return y, nil
	}
	panic("not reached")
}

// Close closes a PipeListener. The returned error will always
// be nil.
func (l *pipeListener) Close() error {
	select {
	case <-l.shutdown:
		// avoiding a panic on double close here
	default:
		close(l.shutdown)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return dummyAddress{}
}

type dummyAddress struct{}

func (dummyAddress) String() string  { return "" }
func (dummyAddress) Network() string { return "" }
