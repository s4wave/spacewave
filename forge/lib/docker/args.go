package forge_lib_docker

import (
	"strconv"
)

func buildDockerEnv(conf *Config) []string {
	vals := conf.GetDockerEnv()
	env := make([]string, 0, len(vals))
	for _, key := range sortedMapKeys(vals) {
		env = append(env, key+"="+vals[key])
	}
	return env
}

func buildCreateArgs(conf *Config) []string {
	args := []string{"create"}
	if workdir := conf.GetWorkdir(); workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	for _, key := range sortedMapKeys(conf.GetEnv()) {
		args = append(args, "--env", key+"="+conf.GetEnv()[key])
	}
	for _, mount := range conf.GetMounts() {
		args = append(args, "--mount", buildMountArg(mount))
	}
	args = append(args, conf.GetImage())
	args = append(args, conf.GetCommand()...)
	return args
}

func buildStopArgs(conf *Config, containerID string) []string {
	args := []string{"stop"}
	if timeout := conf.GetStopTimeoutSeconds(); timeout != 0 {
		args = append(args, "--time", strconv.FormatUint(uint64(timeout), 10))
	}
	return append(args, containerID)
}

func buildMountArg(mount *Mount) string {
	arg := "type=bind,source=" + mount.GetHostPath() + ",target=" + mount.GetContainerPath()
	if mount.GetReadOnly() {
		arg += ",readonly"
	}
	return arg
}
