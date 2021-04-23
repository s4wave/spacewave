package ival

// Increment attepts to increment the numeric interface value.
func Increment(v interface{}) interface{} {
	switch val := v.(type) {
	case int:
		return val + 1
	case uint:
		return val + 1
	case int8:
		return val + 1
	case int16:
		return val + 1
	case int32:
		return val + 1
	case int64:
		return val + 1
	case uint8:
		return val + 1
	case uint16:
		return val + 1
	case uint32:
		return val + 1
	case uint64:
		return val + 1
	case float32:
		return val + 1
	case float64:
		return val + 1
	}
	return v
}
