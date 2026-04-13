package bucket_json

import (
	"strconv"

	configset_json "github.com/aperturerobotics/controllerbus/controller/configset/json"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

func marshalJSONBytesValue(a *fastjson.Arena, dat []byte) (*fastjson.Value, error) {
	var p fastjson.Parser
	v, err := p.ParseBytes(dat)
	if err != nil {
		return nil, err
	}
	return a.DeepCopyValue(v), nil
}

func parseControllerConfigValue(v *fastjson.Value) (*configset_json.ControllerConfig, error) {
	if v == nil || v.Type() == fastjson.TypeNull {
		return nil, nil
	}
	if v.Type() != fastjson.TypeObject {
		return nil, errors.New("controller config must be object")
	}

	c := &configset_json.ControllerConfig{
		Rev: v.GetUint64("rev"),
		Id:  string(v.GetStringBytes("id")),
	}
	if confv := v.Get("config"); confv != nil && confv.Type() != fastjson.TypeNull {
		conf := &configset_json.Config{}
		if err := conf.UnmarshalJSON(confv.MarshalTo(nil)); err != nil {
			return nil, errors.Wrap(err, "unmarshal controller config body")
		}
		c.Config = conf
	}
	return c, nil
}

func marshalControllerConfigValue(
	a *fastjson.Arena,
	c *configset_json.ControllerConfig,
) (*fastjson.Value, error) {
	if c == nil {
		return a.NewNull(), nil
	}

	obj := a.NewObject()
	obj.Set("id", a.NewString(c.Id))
	if c.Rev != 0 {
		obj.Set("rev", a.NewNumberString(strconv.FormatUint(c.Rev, 10)))
	}
	if c.Config != nil {
		dat, err := c.Config.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal controller config body")
		}
		confv, err := marshalJSONBytesValue(a, dat)
		if err != nil {
			return nil, errors.Wrap(err, "parse controller config body")
		}
		obj.Set("config", confv)
	}
	return obj, nil
}
