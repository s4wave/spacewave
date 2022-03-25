package forge_lib_podman_pod

import (
	k8s_v1 "k8s.io/api/core/v1"
	k8s_yaml "sigs.k8s.io/yaml"
)

// ParsePodSpec parses a pod spec from yaml or json.
func ParsePodSpec(body []byte, spec *k8s_v1.PodSpec, opts ...k8s_yaml.JSONOpt) error {
	return k8s_yaml.Unmarshal(body, spec, opts...)
}
