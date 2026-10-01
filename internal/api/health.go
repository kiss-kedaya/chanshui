package api

func (s *Server) runtimeCounts() (queued, running int) {
	s.mu.Lock()
	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	s.mu.Unlock()
	for _, task := range tasks {
		task.mu.Lock()
		switch task.Status {
		case "queued":
			queued++
		case "running":
			running++
		}
		task.mu.Unlock()
	}
	return queued, running
}
