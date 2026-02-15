package app

func registerHandlers(fns ...func() error) error {
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
