package bucket_json

import (
	"context"
	"math"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// Config implements the bucket configuration JSON marshalling logic.
type Config struct {
	// Id is the bucket identifier.
	Id string `json:"id"`
	// Rev is the configuration version.
	Rev uint32 `json:"version"`
	// PutOpts contains the put options.
	PutOpts *block.PutOpts `json:"putOpts,omitempty"`
	// Lookup controls the lookup confiuration.
	Lookup *LookupConfig `json:"lookup,omitempty"`
}

// ParseConfig parses the bucket config JSON bytes.
func ParseConfig(dat []byte) (*Config, error) {
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return nil, err
	}
	return parseConfigValue(v)
}

// NewConfig builds a new controller config.
func NewConfig(ctx context.Context, b bus.Bus, c *bucket.Config) (*Config, error) {
	if c == nil {
		return nil, nil
	}

	n := &Config{
		Id:  c.GetId(),
		Rev: c.GetRev(),
	}
	return n, nil
}

// MarshalJSON marshals the config to JSON.
func (c *Config) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}

	var a fastjson.Arena
	obj := a.NewObject()
	obj.Set("id", a.NewString(c.Id))
	obj.Set("version", a.NewNumberString(strconv.FormatUint(uint64(c.Rev), 10)))

	if c.PutOpts != nil {
		dat, err := c.PutOpts.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal put options")
		}
		putOpts, err := marshalJSONBytesValue(&a, dat)
		if err != nil {
			return nil, errors.Wrap(err, "parse put options")
		}
		obj.Set("putOpts", putOpts)
	}
	if c.Lookup != nil {
		lookup, err := c.Lookup.marshalJSONValue(&a)
		if err != nil {
			return nil, errors.Wrap(err, "marshal lookup config")
		}
		obj.Set("lookup", lookup)
	}
	return obj.MarshalTo(nil), nil
}

// GetRev returns the version.
func (c *Config) GetRev() uint32 {
	return c.Rev
}

// ResolveToProto resolves the config to a proto object.
func (c *Config) ResolveToProto(ctx context.Context, b bus.Bus) (*bucket.Config, error) {
	bc := &bucket.Config{
		Id:      c.Id,
		Rev:     c.Rev,
		PutOpts: c.PutOpts,
	}
	if c.Lookup != nil {
		bc.Lookup = &bucket.LookupConfig{
			Disable: c.Lookup.GetDisable(),
		}
		if c.Lookup.Controller != nil {
			lookupConf, err := c.Lookup.Controller.Resolve(ctx, b)
			if err != nil {
				return nil, errors.Wrap(err, "lookup controller resolve")
			}
			/*
				lc, ok := lookupConf.GetConfig().(lookup.Config)
				if !ok {
					confID := lookupConf.GetConfig().GetConfigID()
					return nil, errors.Errorf("config does not implement lookup config: %s", confID)
				}
			*/
			bc.Lookup.Controller, err = configset_proto.NewControllerConfig(lookupConf, false)
			if err != nil {
				return nil, errors.Wrap(err, "lookup controller resolve")
			}
		}
	}
	return bc, nil
}

func parseConfigValue(v *fastjson.Value) (*Config, error) {
	if v == nil || v.Type() != fastjson.TypeObject {
		return nil, errors.New("bucket config must be object")
	}
	rev := v.GetUint("version")
	if rev > math.MaxUint32 {
		return nil, errors.New("bucket config version exceeds uint32")
	}

	c := &Config{
		Id:  string(v.GetStringBytes("id")),
		Rev: uint32(rev),
	}
	if putOpts := v.Get("putOpts"); putOpts != nil && putOpts.Type() != fastjson.TypeNull {
		po := &block.PutOpts{}
		if err := po.UnmarshalJSON(putOpts.MarshalTo(nil)); err != nil {
			return nil, errors.Wrap(err, "unmarshal put options")
		}
		c.PutOpts = po
	}
	lookup, err := parseLookupConfigValue(v.Get("lookup"))
	if err != nil {
		return nil, errors.Wrap(err, "parse lookup config")
	}
	c.Lookup = lookup
	return c, nil
}
