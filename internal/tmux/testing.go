package tmux

func NewSubscriptionForTest() *Subscription {
	return &Subscription{
		chunks: make(chan OutputChunk, 8),
		errs:   make(chan error, 1),
		close:  func() error { return nil },
	}
}

func (s *Subscription) PushChunk(chunk OutputChunk) {
	s.chunks <- chunk
}

func (s *Subscription) PushError(err error) {
	s.errs <- err
}

func (s *Subscription) CloseChannels() {
	close(s.chunks)
	close(s.errs)
}
