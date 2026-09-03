package http

func (s *Service) nextBackend() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Backends) == 0 {
		return ""
	}

	backend := s.Backends[s.next%len(s.Backends)]
	s.next++

	return backend
}
