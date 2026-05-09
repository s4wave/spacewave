package plugin_host_logs

import (
	"strconv"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

const hostLogPluginID = "plugin-host"

var hostLogrusHooks struct {
	sync.Mutex
	byBus map[bus.Bus]*hostLogrusHookAttachment
}

type hostLogrusHookAttachment struct {
	logger *logrus.Logger
	hook   *hostLogrusHook
	refs   uint64
}

type logrusFieldStringer interface {
	String() string
}

// AttachHostLogrusHook attaches the host logrus hook once for the bus.
func AttachHostLogrusHook(
	b bus.Bus,
	logger *logrus.Logger,
	hub *Hub,
) func() {
	hostLogrusHooks.Lock()
	defer hostLogrusHooks.Unlock()

	if hostLogrusHooks.byBus == nil {
		hostLogrusHooks.byBus = make(map[bus.Bus]*hostLogrusHookAttachment)
	}
	att := hostLogrusHooks.byBus[b]
	if att == nil {
		hook := newHostLogrusHook()
		logger.AddHook(hook)
		att = &hostLogrusHookAttachment{
			logger: logger,
			hook:   hook,
		}
		hostLogrusHooks.byBus[b] = att
	}
	att.refs++
	att.hook.addHub(hub)

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			releaseHostLogrusHook(b, hub)
		})
	}
}

func releaseHostLogrusHook(b bus.Bus, hub *Hub) {
	hostLogrusHooks.Lock()
	defer hostLogrusHooks.Unlock()

	att := hostLogrusHooks.byBus[b]
	if att == nil {
		return
	}
	att.hook.removeHub(hub)
	att.refs--
	if att.refs != 0 {
		return
	}
	delete(hostLogrusHooks.byBus, b)
	removeLogrusHook(att.logger, att.hook)
}

func removeLogrusHook(logger *logrus.Logger, target logrus.Hook) {
	oldHooks := logger.ReplaceHooks(logrus.LevelHooks{})
	nextHooks := make(logrus.LevelHooks, len(oldHooks))
	for level, hooks := range oldHooks {
		for _, hook := range hooks {
			if hook == target {
				continue
			}
			nextHooks[level] = append(nextHooks[level], hook)
		}
	}
	logger.ReplaceHooks(nextHooks)
}

type hostLogrusHook struct {
	mu   sync.RWMutex
	hubs map[*Hub]uint64
}

func newHostLogrusHook() *hostLogrusHook {
	return &hostLogrusHook{
		hubs: make(map[*Hub]uint64),
	}
}

func (h *hostLogrusHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *hostLogrusHook) Fire(entry *logrus.Entry) error {
	event := structuredLogEventFromLogrusEntry(entry)

	h.mu.RLock()
	hubs := make([]*Hub, 0, len(h.hubs))
	for hub, refs := range h.hubs {
		if refs != 0 {
			hubs = append(hubs, hub)
		}
	}
	h.mu.RUnlock()

	for _, hub := range hubs {
		if _, err := hub.Emit(event); err != nil {
			return err
		}
	}
	return nil
}

func (h *hostLogrusHook) addHub(hub *Hub) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hubs[hub]++
}

func (h *hostLogrusHook) removeHub(hub *Hub) {
	h.mu.Lock()
	defer h.mu.Unlock()

	refs := h.hubs[hub]
	if refs <= 1 {
		delete(h.hubs, hub)
		return
	}
	h.hubs[hub] = refs - 1
}

func structuredLogEventFromLogrusEntry(entry *logrus.Entry) *StructuredLogEvent {
	fields := make(map[string]string, len(entry.Data))
	pluginID := hostLogPluginID
	var instanceKey string
	for key, value := range entry.Data {
		fieldValue := logrusFieldString(value)
		fields[key] = fieldValue
		switch key {
		case "plugin-id", "plugin_id":
			if fieldValue != "" {
				pluginID = fieldValue
			}
		case "instance-key", "instance_key":
			instanceKey = fieldValue
		}
	}
	if entry.Caller != nil {
		fields["caller-file"] = entry.Caller.File
		fields["caller-function"] = entry.Caller.Function
		fields["caller-line"] = strconv.Itoa(entry.Caller.Line)
	}

	return &StructuredLogEvent{
		PluginId:    pluginID,
		InstanceKey: instanceKey,
		Stream:      StructuredLogStream_STRUCTURED_LOG_STREAM_LOGGER,
		Level:       structuredLogLevelFromLogrus(entry.Level),
		Message:     entry.Message,
		Fields:      fields,
	}
}

func structuredLogLevelFromLogrus(level logrus.Level) StructuredLogLevel {
	switch level {
	case logrus.TraceLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_TRACE
	case logrus.DebugLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_DEBUG
	case logrus.InfoLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_INFO
	case logrus.WarnLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN
	case logrus.ErrorLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_ERROR
	case logrus.FatalLevel, logrus.PanicLevel:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_FATAL
	default:
		return StructuredLogLevel_STRUCTURED_LOG_LEVEL_UNSPECIFIED
	}
}

func logrusFieldString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case logrusFieldStringer:
		return v.String()
	case error:
		return v.Error()
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return ""
	}
}
