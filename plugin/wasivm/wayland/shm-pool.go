package wayland

import (
	"github.com/s4wave/spacewave/plugin/wasivm/kernel"
)

// shmPool is a wl_shm_pool: a client's shared memory file, from which its
// buffers take their pixels.
type shmPool struct {
	// c is the client.
	c *client
	// id is the pool's object id.
	id uint32
	// file is the shared memory file.
	file *kernel.File
	// size is the pool's size in bytes, which only grows.
	size int32
	// refs counts the pool object and its buffers. The file closes when refs
	// reaches zero.
	refs int
}

// request handles a wl_shm_pool request.
func (p *shmPool) request(op uint16, m *message) error {
	switch op {
	case shmPoolCreateBuffer:
		id, offset, width, height, stride, format := m.uint(), m.int(), m.int(), m.int(), m.int(), m.uint()
		if m.err != nil {
			return nil
		}
		return p.createBuffer(id, offset, width, height, stride, format)
	case shmPoolDestroy:
		p.c.destroy(p.id)
		p.unref()
	case shmPoolResize:
		size := m.int()
		if m.err != nil {
			return nil
		}
		if size < p.size {
			return p.c.fail(p.id, shmErrInvalidFD, "pool shrank")
		}
		p.size = size
	}
	return nil
}

// createBuffer creates a buffer of width by height pixels at offset, after
// checking that it fits the pool.
func (p *shmPool) createBuffer(id uint32, offset, width, height, stride int32, format uint32) error {
	// Check the format and the layout.
	if format != shmFormatARGB8888 && format != shmFormatXRGB8888 {
		return p.c.fail(p.id, shmErrInvalidFormat, "unsupported format")
	}
	end := int64(offset) + int64(height-1)*int64(stride) + 4*int64(width)
	if offset < 0 || width <= 0 || height <= 0 || stride < 4*width || end > int64(p.size) {
		return p.c.fail(p.id, shmErrInvalidStride, "buffer outside its pool")
	}

	// Add the buffer, which holds the pool open.
	b := &shmBuffer{
		pool:   p,
		id:     id,
		offset: int64(offset),
		width:  int(width),
		height: int(height),
		stride: int(stride),
		opaque: format == shmFormatXRGB8888,
	}
	if err := p.c.add(id, b); err != nil {
		return err
	}
	p.refs++
	return nil
}

// unref drops a reference and closes the file after the last reference.
func (p *shmPool) unref() {
	p.refs--
	if p.refs == 0 {
		_ = p.file.Close()
	}
}

// _ is a type assertion.
var _ object = (*shmPool)(nil)
