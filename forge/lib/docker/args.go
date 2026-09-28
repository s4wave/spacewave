package forge_lib_docker

import (
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
func buildCreateArgs(conf *Config, runtimeName string) []string {
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

// buildMountArg renders one bind mount in docker --mount syntax.
func buildMountArg(mount *Mount) string {
	arg := "type=bind,source=" + mount.GetHostPath() + ",target=" + mount.GetContainerPath()
	if mount.GetReadOnly() {
		arg += ",readonly"
	}
	return arg
}
