package forge_lib_docker

import "github.com/pkg/errors"

// ApplyWorkdirBind adds the single POSIX bind mount for a supervised Workdir.
// A Docker bind mount bypasses the FSHandle writer, so its caller must flush
// and prove quiescence before collecting diff evidence.
func ApplyWorkdirBind(conf *Config, hostPath, containerPath string) error {
	switch {
	case conf == nil:
		return errors.New("docker config not set")
	case hostPath == "":
		return errors.New("host path must be set")
	case containerPath == "":
		return errors.New("container path must be set")
	}
	conf.Mounts = append(conf.Mounts, &Mount{HostPath: hostPath, ContainerPath: containerPath})
	return nil
}
