package tunnel

import (
	"errors"
	"io"
	"net"
	"sync"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	glog "github.com/sirupsen/logrus"
)

type TunnelServer struct {
	id     string
	stream cakeagent.CakeAgentService_TunnelServer
	conn   net.Conn
}

func NewTunnelServer(stream cakeagent.CakeAgentService_TunnelServer) (tunnel *TunnelServer, err error) {
	var input *cakeagent.CakeAgent_TunnelMessage
	var conn net.Conn
	protocols := []string{"tcp", "udp"}

	if input, err = stream.Recv(); err == nil {
		if connect := input.GetConnect(); connect == nil {
			err = errors.New("invalid message")
		} else if conn, err = net.Dial(protocols[connect.Protocol], connect.GuestAddress); err == nil {
			tunnel = &TunnelServer{
				id:     connect.Id,
				stream: stream,
				conn:   conn,
			}
		}
	} else if errors.Is(err, io.EOF) {
		err = nil
	}

	return
}

func (s *TunnelServer) Stream(quit <-chan struct{}) (err error) {
	var wg sync.WaitGroup

	broker := func(to, from io.ReadWriter) {

		if _, err := io.Copy(to, from); err != nil {
			glog.WithError(err).Debug("failed to call io.Copy")
		}

		wg.Done()
	}

	wg.Add(2)

	go broker(s, s.conn)
	go broker(s.conn, s)

	finish := make(chan struct{})

	go func() {
		wg.Wait()
		close(finish)
	}()

	select {
	case <-quit:
	case <-finish:
	}

	if err = s.conn.Close(); err != nil {
		glog.WithError(err).Debug("failed close guest connection")
	}

	if err = s.Close(); err != nil {
		glog.WithError(err).Debug("failed to close stream")
	}

	<-finish

	return
}

func (s *TunnelServer) Close() error {
	message := &cakeagent.CakeAgent_TunnelMessage{
		Message: &cakeagent.CakeAgent_TunnelMessage_Eof{
			Eof: true,
		},
	}

	return s.stream.Send(message)
}

func (s *TunnelServer) Write(p []byte) (n int, err error) {
	message := &cakeagent.CakeAgent_TunnelMessage{
		Message: &cakeagent.CakeAgent_TunnelMessage_Datas{
			Datas: p,
		},
	}

	err = s.stream.Send(message)

	return len(p), err
}

func (s *TunnelServer) Read(p []byte) (n int, err error) {
	if in, err := s.stream.Recv(); err != nil {
		return 0, err
	} else if data := in.GetDatas(); data != nil {
		copy(p, data)

		return len(data), nil
	} else if eof := in.GetEof(); eof {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Trace("Received EOF")
		}
		return 0, io.EOF
	} else if err := in.GetError(); err != "" {
		if glog.GetLevel() >= glog.TraceLevel {
			glog.Tracef("Received error: %s", err)
		}
		return 0, errors.New(err)
	} else {
		return 0, errors.New("invalid message")
	}
}
