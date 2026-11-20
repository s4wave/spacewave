package forge_lib_docker

import (
	"slices"
	"strconv"
	"strings"
)

// BuildDockerEnv renders the complete configured CLI environment as KEY=value arguments.
func BuildDockerEnv(conf *Config) []string {
	vals := conf.GetDockerEnv()
	env := make([]string, 0, len(vals))
	for _, key := range sortedMapKeys(vals) {
		env = append(env, key+"="+vals[key])
	}
	return env
}

// buildCreateArgs renders the docker create invocation for the config.
// mounts follow the config's own bind mounts.
func buildCreateArgs(conf *Config, runtimeName string, mounts []*Mount) []string {
	// Select the container creation command and reserved runtime name.
	args := []string{"create"}
	if runtimeName != "" {
		args = append(args, "--name", runtimeName)
	}

	// Enforce the same CPU and memory request that admission debits.
	cpu := strconv.FormatUint(conf.GetMilliCpu()/1000, 10)
	if fraction := conf.GetMilliCpu() % 1000; fraction != 0 {
		cpu += "." + strings.TrimRight(strconv.FormatUint(1000+fraction, 10)[1:], "0")
	}
	args = append(args, "--cpus", cpu, "--memory", strconv.FormatUint(conf.GetMemoryBytes(), 10))

	// Apply the explicitly requested network and host devices.
	if mode := conf.GetNetworkMode(); mode != "" {
		args = append(args, "--network", mode)
	}
	for _, device := range conf.GetDevices() {
		args = append(args, "--device", device)
	}

	// Apply the container working directory, environment, and bind mounts.
	if workdir := conf.GetWorkdir(); workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	for _, key := range sortedMapKeys(conf.GetEnv()) {
		args = append(args, "--env", key+"="+conf.GetEnv()[key])
	}
	for _, mount := range slices.Concat(conf.GetMounts(), mounts) {
		args = append(args, "--mount", buildMountArg(mount))
	}

	// Select the entrypoint, image and command after the runtime options.
	entrypoint := conf.GetEntrypoint()
	if len(entrypoint) != 0 {
		args = append(args, "--entrypoint", entrypoint[0])
	}
	args = append(args, conf.GetImage())
	if len(entrypoint) > 1 {
		args = append(args, entrypoint[1:]...)
	}
	args = append(args, conf.GetCommand()...)
	return args
}

// buildMountArg renders one bind mount in docker --mount syntax.
func buildMountArg(mount *Mount) string {
	arg := "type=bind,source=" + mount.GetHostPath() + ",target=" + mount.GetContainerPath()
	if mount.GetReadOnly() {
		arg += ",readonly"
	}
	return arg
}
