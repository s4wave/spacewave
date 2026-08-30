package s4wave_canvas

// MarshalBlock marshals the logical Canvas migration payload to binary.
func (s *CanvasState) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// Clone clones the canvas state.
func (s *CanvasState) Clone() *CanvasState {
	if s == nil {
		return nil
	}
	return s.CloneVT()
}
