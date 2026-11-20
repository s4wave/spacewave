//go:build !unix

package forge_lib_docker

// workerUser returns nil: without Unix ownership the container keeps the
// image's user.
func workerUser([]string) (*ContainerUser, error) {
	return nil, nil
}
