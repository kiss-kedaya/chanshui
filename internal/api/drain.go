package api

// BeginDrain rejects new jobs while allowing queued and running jobs to finish.
func (s *Server) BeginDrain() { s.draining.Store(true) }
