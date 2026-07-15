package transport_quic

import (
	"strconv"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
	"github.com/sirupsen/logrus"
)

type logQlogTrace struct {
	le *logrus.Entry
}

func newLogQlogTrace(
	le *logrus.Entry,
	isClient bool,
	connID quic.ConnectionID,
) qlogwriter.Trace {
	role := "server"
	if isClient {
		role = "client"
	}
	return &logQlogTrace{le: le.WithFields(logrus.Fields{
		"quic-connection-id": connID.String(),
		"quic-role":          role,
	})}
}

func (t *logQlogTrace) AddProducer() qlogwriter.Recorder {
	return &logQlogRecorder{le: t.le}
}

func (*logQlogTrace) SupportsSchemas(schema string) bool {
	return schema == qlog.EventSchema
}

type logQlogRecorder struct {
	le *logrus.Entry
}

func (r *logQlogRecorder) RecordEvent(event qlogwriter.Event) {
	switch e := event.(type) {
	case qlog.PacketSent:
		r.logPacket("sent", e.Header, e.Raw, e.Frames)
	case qlog.PacketReceived:
		r.logPacket("received", e.Header, e.Raw, e.Frames)
	case qlog.PacketBuffered:
		r.le.WithFields(packetFields("buffered", e.Header, e.Raw)).Debug("quic trace")
	case qlog.PacketDropped:
		fields := packetFields("dropped", e.Header, e.Raw)
		fields["trigger"] = string(e.Trigger)
		r.le.WithFields(fields).Debug("quic trace")
	case qlog.PacketLost:
		fields := packetFields("lost", e.Header, qlog.RawInfo{})
		fields["trigger"] = string(e.Trigger)
		r.le.WithFields(fields).Debug("quic trace")
	case qlog.ConnectionClosed:
		fields := logrus.Fields{
			"initiator": string(e.Initiator),
			"reason":    e.Reason,
			"trigger":   string(e.Trigger),
		}
		if e.ConnectionError != nil {
			fields["connection-error"] = uint64(*e.ConnectionError)
		}
		if e.ApplicationError != nil {
			fields["application-error"] = uint64(*e.ApplicationError)
		}
		r.le.WithFields(fields).Debug("quic trace: connection closed")
	default:
		r.le.WithField("quic-event", event.Name()).Debug("quic trace")
	}
}

func (r *logQlogRecorder) logPacket(
	direction string,
	header qlog.PacketHeader,
	raw qlog.RawInfo,
	frames []qlog.Frame,
) {
	fields := packetFields(direction, header, raw)
	fields["frames"] = qlogFrameNames(frames)
	r.le.WithFields(fields).Debug("quic trace")
}

func (*logQlogRecorder) Close() error {
	return nil
}

func packetFields(
	direction string,
	header qlog.PacketHeader,
	raw qlog.RawInfo,
) logrus.Fields {
	return logrus.Fields{
		"direction":     direction,
		"packet-type":   string(header.PacketType),
		"packet-number": int64(header.PacketNumber),
		"packet-length": raw.Length,
	}
}

func qlogFrameNames(frames []qlog.Frame) []string {
	names := make([]string, 0, len(frames))
	for _, frame := range frames {
		switch f := frame.Frame.(type) {
		case *qlog.AckFrame:
			names = append(names, "ack")
		case *qlog.CryptoFrame:
			names = append(names,
				"crypto(offset="+strconv.FormatInt(f.Offset, 10)+
					",length="+strconv.FormatInt(f.Length, 10)+")")
		case *qlog.HandshakeDoneFrame:
			names = append(names, "handshake_done")
		case *qlog.PingFrame:
			names = append(names, "ping")
		case *qlog.ConnectionCloseFrame:
			names = append(names, "connection_close")
		default:
			names = append(names, "other")
		}
	}
	return names
}

// _ is a type assertion
var (
	_ qlogwriter.Trace    = (*logQlogTrace)(nil)
	_ qlogwriter.Recorder = (*logQlogRecorder)(nil)
)
