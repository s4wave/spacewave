//go:build !js

package goscriptbench

import (
	"context"
	"math"
	"slices"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

func (p *ProjectedImage) captureBrowserCPUProfile(
	ctx context.Context,
	work func(context.Context) (Sample, error),
) (sample Sample, data []byte, retErr error) {
	if !p.config.BrowserCPUProfile || p.config.Engine != "chromium" {
		sample, retErr = work(ctx)
		return sample, nil, retErr
	}

	// Bracket the browser action with the Chromium profiler.
	cdp, err := p.session.BrowserContext().NewCDPSession(p.session.Page())
	if err != nil {
		return Sample{}, nil, errors.Wrap(err, "create projected-image CDP session")
	}
	defer func() {
		if err := cdp.Detach(); retErr == nil && err != nil {
			retErr = errors.Wrap(err, "detach projected-image CDP session")
		}
	}()
	if _, err := cdp.Send("Profiler.enable", nil); err != nil {
		return Sample{}, nil, errors.Wrap(err, "enable projected-image CPU profiler")
	}
	if _, err := cdp.Send("Profiler.start", nil); err != nil {
		return Sample{}, nil, errors.Wrap(err, "start projected-image CPU profiler")
	}

	// Stop the profiler even when the measured action fails.
	sample, workErr := work(ctx)
	response, stopErr := cdp.Send("Profiler.stop", nil)
	if workErr != nil {
		return Sample{}, nil, workErr
	}
	if stopErr != nil {
		return Sample{}, nil, errors.Wrap(stopErr, "stop projected-image CPU profiler")
	}
	if err := ctx.Err(); err != nil {
		return Sample{}, nil, err
	}

	// Encode the returned profile without reflection.
	profile := response
	if object, ok := response.(map[string]any); ok {
		if value, exists := object["profile"]; exists {
			profile = value
		}
	}
	data, err = marshalProjectedImageCPUProfile(profile)
	if err != nil {
		return Sample{}, nil, err
	}
	return sample, data, nil
}

func marshalProjectedImageCPUProfile(profile any) ([]byte, error) {
	var arena fastjson.Arena
	value, err := marshalProjectedImageCDPValue(&arena, profile)
	if err != nil {
		return nil, err
	}
	return append(value.MarshalTo(nil), '\n'), nil
}

func marshalProjectedImageCDPValue(arena *fastjson.Arena, value any) (*fastjson.Value, error) {
	switch typed := value.(type) {
	case nil:
		return arena.NewNull(), nil
	case bool:
		if typed {
			return arena.NewTrue(), nil
		}
		return arena.NewFalse(), nil
	case string:
		return arena.NewString(typed), nil
	case int:
		return arena.NewNumberInt(typed), nil
	case int64:
		return arena.NewNumberString(strconv.FormatInt(typed, 10)), nil
	case uint64:
		return arena.NewNumberString(strconv.FormatUint(typed, 10)), nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, errors.New("projected-image CPU profile contains a non-finite number")
		}
		return arena.NewNumberString(strconv.FormatFloat(typed, 'f', -1, 64)), nil
	case []any:
		array := arena.NewArray()
		for idx, item := range typed {
			encoded, err := marshalProjectedImageCDPValue(arena, item)
			if err != nil {
				return nil, err
			}
			array.SetArrayItem(idx, encoded)
		}
		return array, nil
	case map[string]any:
		object := arena.NewObject()
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			encoded, err := marshalProjectedImageCDPValue(arena, typed[key])
			if err != nil {
				return nil, err
			}
			object.Set(key, encoded)
		}
		return object, nil
	default:
		return nil, errors.Errorf("projected-image CPU profile contains unsupported %T", value)
	}
}
